// Package handoff stores one disposable, credential-free remote host projection.
package handoff

import (
	"errors"
	"io"
	"os"
	"sync"

	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/ownerfs"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
)

var (
	ErrUnsafe      = errors.New("handoff_unsafe_storage")
	ErrUnavailable = errors.New("handoff_unavailable")
	ErrMissing     = errors.New("handoff_missing")
	ErrInvalid     = errors.New("handoff_invalid_snapshot")
	ErrBusy        = errors.New("handoff_writer_busy")
)

const latestName = "snapshot.json"
const stagingName = ".snapshot-next"
const lockName = ".writer-lock"

// Store owns a pinned directory. Its privacy policy is current-OS-user scoped;
// it does not isolate mutually hostile processes running as that same user.
type Store struct {
	mu        sync.Mutex
	root      *os.Root
	directory *os.File
	writer    *os.File
	syncFile  func(*os.File) error
	writeFile func(*os.File, []byte) (int, error)
	syncDir   func(*os.File) error
	replace   func(string, string) error
}

// Open validates an existing private directory without creating or repairing it.
// A reader cannot publish. A writer acquires the single-writer OS lock.
func Open(path string, writable bool) (*Store, error) {
	if ownerfs.ValidateDedicatedDirectory(path) != nil {
		return nil, ErrUnsafe
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, ErrUnsafe
	}
	dir, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, ErrUnsafe
	}
	if privateHandle(dir, true) != nil {
		dir.Close()
		root.Close()
		return nil, ErrUnsafe
	}
	s := &Store{root: root, directory: dir, syncFile: (*os.File).Sync, writeFile: (*os.File).Write, syncDir: syncDirectory, replace: root.Rename}
	if writable {
		lock, err := s.openFile(lockName, os.O_RDWR|os.O_CREATE, 0)
		if err != nil {
			s.Close()
			return nil, err
		}
		if err := lockWriter(lock); err != nil {
			lock.Close()
			s.Close()
			return nil, err
		}
		s.writer = lock
	}
	return s, nil
}

// Publish never accepts arbitrary bytes: all data crosses the explicit projector.
func (s *Store) Publish(raw observation.Snapshot, identity remoteprojection.Identity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil || s.writer == nil {
		return ErrUnavailable
	}
	data, err := remoteprojection.Encode(raw, identity)
	if err != nil {
		return ErrInvalid
	}
	if err := s.removeStaging(); err != nil {
		return err
	}
	f, err := s.openFile(stagingName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, remoteprojection.MaxBytes)
	if err != nil {
		return err
	}
	defer f.Close()
	if n, err := s.writeFile(f, data); err != nil || n != len(data) {
		return ErrUnavailable
	}
	if s.syncFile(f) != nil {
		return ErrUnavailable
	}
	if f.Close() != nil {
		return ErrUnavailable
	}
	// Refuse replacing an unsafe existing target; never repair it implicitly.
	if old, err := s.openFile(latestName, os.O_RDONLY, remoteprojection.MaxBytes); err == nil {
		old.Close()
	} else if !errors.Is(err, ErrMissing) {
		return err
	}
	if s.replace(stagingName, latestName) != nil {
		return ErrUnavailable
	}
	if s.syncDir(s.directory) != nil {
		return ErrUnavailable
	}
	return nil
}

// Read returns owned, canonical, source-bound bytes. It does not judge freshness;
// the later uploader checks the document's collection time against its own clock.
func (s *Store) Read(expectedServerID string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil {
		return nil, ErrUnavailable
	}
	f, err := s.openFile(latestName, os.O_RDONLY, remoteprojection.MaxBytes)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, remoteprojection.MaxBytes+1))
	if err != nil {
		return nil, ErrUnavailable
	}
	if len(data) > remoteprojection.MaxBytes || remoteprojection.Validate(data, expectedServerID) != nil {
		return nil, ErrInvalid
	}
	return data, nil
}

func (s *Store) openFile(name string, flags int, maxBytes int64) (*os.File, error) {
	before, err := s.root.Lstat(name)
	if err == nil && (!before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0) {
		return nil, ErrUnsafe
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, ErrUnsafe
	}
	f, err := s.root.OpenFile(name, flags|safeOpenFlags(), 0o600)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrMissing
	}
	if err != nil {
		return nil, ErrUnavailable
	}
	info, err := f.Stat()
	after, pathErr := s.root.Lstat(name)
	if err != nil || pathErr != nil {
		f.Close()
		return nil, ErrUnavailable
	}
	// A cooperating writer may replace the slot between open and the path check.
	// Do not label that transient race as a storage-permission violation.
	if !os.SameFile(info, after) {
		f.Close()
		return nil, ErrUnavailable
	}
	if !after.Mode().IsRegular() ||
		info.Size() < 0 || info.Size() > maxBytes || privateHandle(f, false) != nil {
		f.Close()
		// Replacement can also occur during handle privacy validation, leaving
		// the formerly valid file unlinked. Reject without misclassifying that race.
		current, currentErr := s.root.Lstat(name)
		if errors.Is(currentErr, os.ErrNotExist) || (currentErr == nil && !os.SameFile(info, current)) {
			return nil, ErrUnavailable
		}
		return nil, ErrUnsafe
	}
	return f, nil
}

func (s *Store) removeStaging() error {
	f, err := s.openFile(stagingName, os.O_RDONLY, remoteprojection.MaxBytes)
	if errors.Is(err, ErrMissing) {
		return nil
	}
	if err != nil {
		return err
	}
	f.Close()
	if s.root.Remove(stagingName) != nil {
		return ErrUnavailable
	}
	return nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil {
		return nil
	}
	var failed bool
	if s.writer != nil {
		failed = s.writer.Close() != nil
		s.writer = nil
	}
	failed = s.directory.Close() != nil || failed
	failed = s.root.Close() != nil || failed
	s.root = nil
	if failed {
		return ErrUnavailable
	}
	return nil
}
