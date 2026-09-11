//go:build linux

package enrollmentstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/enrollmentcoord"
	"github.com/braidenm/home-lab-observer/internal/uploadledger"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

type Setup struct {
	mu             sync.Mutex
	path           string
	root           *os.Root
	dir, lease     *os.File
	binding        uploadstate.Binding
	step           int
	failed, closed bool
	hook           func(string) error
}

type Ready struct {
	mu         sync.Mutex
	setup      *Setup
	ledger     *uploadledger.Ledger
	credential enrollmentcoord.Credential
}

var _ enrollmentcoord.AttemptStore = (*Setup)(nil)
var _ enrollmentcoord.CredentialStore = (*Setup)(nil)
var _ enrollmentcoord.LedgerProvisioner = (*Setup)(nil)

func open(ctx context.Context, directory string, fresh bool) (_ *Setup, result error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrRecovery
	}
	path, err := safePath(directory)
	if err != nil {
		return nil, err
	}
	dir, err := openDirectory(path)
	if err != nil {
		return nil, err
	}
	s := &Setup{path: path, dir: dir}
	defer func() {
		if result != nil {
			s.Close()
		}
	}()
	s.root, err = os.OpenRoot(path)
	if err != nil {
		return nil, ErrUnsafe
	}
	// Prove both handles name the same inode before any mutation.
	ri, err := s.root.Stat(".")
	di, statErr := dir.Stat()
	if err != nil || statErr != nil || !os.SameFile(ri, di) {
		return nil, ErrUnsafe
	}
	entries, err := s.entries()
	if err != nil {
		return nil, err
	}
	if fresh && len(entries) != 0 {
		return nil, ErrRecovery
	}
	if !fresh && (!entries[lockName] || entries[stageName]) {
		return nil, ErrRecovery
	}
	s.lease, err = s.file(lockName, fresh, 0)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(s.lease.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrBusy
		}
		return nil, ErrRecovery
	}
	if s.checkRoot() != nil || ctx.Err() != nil {
		return nil, ErrRecovery
	}
	if fresh && (s.lease.Sync() != nil || dir.Sync() != nil) {
		return nil, ErrRecovery
	}
	return s, nil
}

func OpenNew(ctx context.Context, directory string) (*Setup, error) {
	return open(ctx, directory, true)
}

func (s *Setup) check(ctx context.Context, binding uploadstate.Binding, step int) error {
	if s.closed || s.failed || ctx == nil || ctx.Err() != nil || !validBinding(binding) || s.step != step || (step != 0 && s.binding != binding) {
		s.failed = true
		return ErrRecovery
	}
	entries, err := s.entries()
	want := map[string]bool{lockName: true}
	if step >= 1 {
		want[attemptName] = true
	}
	if step >= 2 {
		want[credentialName] = true
	}
	if step >= 3 {
		want[ledgerName] = true
	}
	if err != nil || !reflect.DeepEqual(entries, want) {
		s.failed = true
		return ErrRecovery
	}
	return nil
}

func (s *Setup) write(ctx context.Context, name, state string, secret []byte) error {
	data, err := json.Marshal(makeRecord(s.binding, state, secret))
	defer clear(data)
	if err != nil || s.publish(name, data) != nil || ctx.Err() != nil {
		s.failed = true
		return ErrRecovery
	}
	s.step++
	return nil
}

func (s *Setup) Begin(ctx context.Context, binding uploadstate.Binding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx, binding, 0); err != nil {
		return err
	}
	s.binding = binding
	return s.write(ctx, attemptName, "ATTEMPTED", nil)
}

func (s *Setup) Create(ctx context.Context, credential enrollmentcoord.Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx, credential.Binding, 1); err != nil {
		return err
	}
	if len(credential.Secret) != 47 || !secretPattern.Match(credential.Secret) {
		s.failed = true
		return ErrRecovery
	}
	if _, err := s.read(attemptName, "ATTEMPTED"); err != nil {
		s.failed = true
		return ErrRecovery
	}
	return s.write(ctx, credentialName, "CREDENTIAL", credential.Secret)
}

func (s *Setup) Provision(ctx context.Context, binding uploadstate.Binding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx, binding, 2); err != nil {
		return err
	}
	if _, err := s.read(attemptName, "ATTEMPTED"); err != nil {
		s.failed = true
		return ErrRecovery
	}
	if _, err := s.read(credentialName, "CREDENTIAL"); err != nil {
		s.failed = true
		return ErrRecovery
	}
	if s.root.Mkdir(ledgerName, 0o700) != nil {
		s.failed = true
		return ErrRecovery
	}
	if _, err := s.entries(); err != nil {
		s.failed = true
		return ErrRecovery
	}
	l, err := uploadledger.Provision(ctx, filepath.Join(s.path, ledgerName), binding)
	if err != nil {
		s.failed = true
		return ErrRecovery
	}
	r, loadErr := l.Load(ctx)
	closeErr := l.Close()
	if loadErr != nil || closeErr != nil || !reflect.DeepEqual(r, uploadstate.Record{Binding: binding}) || s.ledgerPrivacy() != nil || s.dir.Sync() != nil || ctx.Err() != nil {
		s.failed = true
		return ErrRecovery
	}
	s.step++
	return nil
}

func (s *Setup) MarkReady(ctx context.Context, binding uploadstate.Binding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx, binding, 3); err != nil {
		return err
	}
	if _, err := s.read(attemptName, "ATTEMPTED"); err != nil {
		s.failed = true
		return ErrRecovery
	}
	if _, err := s.read(credentialName, "CREDENTIAL"); err != nil {
		s.failed = true
		return ErrRecovery
	}
	if s.ledgerPrivacy() != nil {
		s.failed = true
		return ErrRecovery
	}
	l, err := uploadledger.OpenExisting(ctx, filepath.Join(s.path, ledgerName), binding)
	if err != nil {
		s.failed = true
		return ErrRecovery
	}
	r, loadErr := l.Load(ctx)
	closeErr := l.Close()
	if loadErr != nil || closeErr != nil || !reflect.DeepEqual(r, uploadstate.Record{Binding: binding}) {
		s.failed = true
		return ErrRecovery
	}
	return s.write(ctx, readyName, "READY", nil)
}

func OpenReady(ctx context.Context, directory string, binding uploadstate.Binding) (_ *Ready, result error) {
	if !validBinding(binding) {
		return nil, ErrRecovery
	}
	s, err := open(ctx, directory, false)
	if err != nil {
		return nil, err
	}
	r := &Ready{setup: s}
	defer func() {
		if result != nil {
			r.Close()
		}
	}()
	s.binding = binding
	entries, err := s.entries()
	want := map[string]bool{lockName: true, attemptName: true, credentialName: true, readyName: true, ledgerName: true}
	if err != nil || !reflect.DeepEqual(entries, want) {
		return nil, ErrRecovery
	}
	if _, err := s.read(attemptName, "ATTEMPTED"); err != nil {
		return nil, ErrRecovery
	}
	if _, err := s.read(readyName, "READY"); err != nil {
		return nil, ErrRecovery
	}
	credential, err := s.read(credentialName, "CREDENTIAL")
	if err != nil {
		return nil, ErrRecovery
	}
	// Sync only record handles, never raw SQLite files while D1 owns its locks.
	for _, name := range []string{lockName, attemptName, credentialName, readyName} {
		f, err := s.file(name, false, maxRecord)
		if err != nil {
			return nil, ErrRecovery
		}
		syncErr := f.Sync()
		closeErr := f.Close()
		if syncErr != nil || closeErr != nil {
			return nil, ErrRecovery
		}
	}
	if s.ledgerPrivacy() != nil {
		return nil, ErrRecovery
	}
	r.ledger, err = uploadledger.OpenExisting(ctx, filepath.Join(s.path, ledgerName), binding)
	if err != nil {
		return nil, ErrRecovery
	}
	if _, err := r.ledger.Load(ctx); err != nil {
		return nil, ErrRecovery
	}
	d, err := openDirectory(filepath.Join(s.path, ledgerName))
	if err != nil {
		return nil, ErrRecovery
	}
	syncErr := d.Sync()
	closeErr := d.Close()
	if syncErr != nil || closeErr != nil || s.dir.Sync() != nil || s.checkRoot() != nil || ctx.Err() != nil {
		return nil, ErrRecovery
	}
	r.credential = enrollmentcoord.Credential{Binding: binding, Secret: []byte(credential.Secret)}
	return r, nil
}

func (r *Ready) Credential() (enrollmentcoord.Credential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.setup == nil {
		return enrollmentcoord.Credential{}, ErrRecovery
	}
	return enrollmentcoord.Credential{Binding: r.credential.Binding, Secret: append([]byte(nil), r.credential.Secret...)}, nil
}

func (r *Ready) Ledger() *uploadledger.Ledger {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ledger
}

func (r *Ready) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result error
	if r.ledger != nil {
		if r.ledger.Close() != nil {
			result = ErrRecovery
		}
		r.ledger = nil
	}
	clear(r.credential.Secret)
	r.credential = enrollmentcoord.Credential{}
	if r.setup != nil {
		if r.setup.Close() != nil {
			result = ErrRecovery
		}
		r.setup = nil
	}
	return result
}

func (s *Setup) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var result error
	for _, f := range []*os.File{s.dir, s.lease} {
		if f != nil && f.Close() != nil {
			result = ErrRecovery
		}
	}
	if s.root != nil && s.root.Close() != nil {
		result = ErrRecovery
	}
	return result
}
