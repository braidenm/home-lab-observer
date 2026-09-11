//go:build linux

package sharedhandoff

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
)

type slot struct {
	mu        sync.Mutex
	policy    Policy
	root      *os.Root
	directory *os.File
	lock      *os.File
	syncFile  func(*os.File) error
	syncDir   func(*os.File) error
	writeFile func(*os.File, []byte) (int, error)
	replace   func(string, string) error
}
type Writer struct{ s *slot }
type Reader struct{ s *slot }

func open(path string, p Policy, writer bool) (*slot, error) {
	if identity(p, writer) != nil || safePath(path, p) != nil {
		return nil, ErrUnsafe
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, ErrUnsafe
	}
	directory, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, ErrUnsafe
	}
	s := &slot{policy: p, root: root, directory: directory, syncFile: (*os.File).Sync, syncDir: (*os.File).Sync, writeFile: (*os.File).Write, replace: root.Rename}
	if err := s.checkDirectory(); err != nil {
		s.close()
		return nil, err
	}
	if writer {
		f, err := s.openFile(lockName, os.O_RDWR|os.O_CREATE, 0, 0o600)
		if err != nil {
			s.close()
			return nil, err
		}
		if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
			f.Close()
			s.close()
			if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
				return nil, ErrBusy
			}
			return nil, ErrUnavailable
		}
		s.lock = f
	}
	return s, nil
}
func OpenWriter(path string, p Policy) (*Writer, error) {
	s, err := open(path, p, true)
	if err != nil {
		return nil, err
	}
	return &Writer{s}, nil
}
func OpenReader(path string, p Policy) (*Reader, error) {
	s, err := open(path, p, false)
	if err != nil {
		return nil, err
	}
	return &Reader{s}, nil
}

func (s *slot) checkDirectory() error {
	if s.root == nil {
		return ErrUnavailable
	}
	if err := checkHandle(s.directory, s.policy, true, 0o750); err != nil {
		return err
	}
	// A fresh descriptor provides an independent directory enumeration offset.
	f, err := s.root.Open(".")
	if err != nil {
		return ErrUnavailable
	}
	defer f.Close()
	entries, err := f.ReadDir(4)
	if err != nil && !errors.Is(err, io.EOF) {
		return ErrUnavailable
	}
	if len(entries) > 3 {
		return ErrUnsafe
	}
	var total int64
	for _, entry := range entries {
		info, err := s.root.Lstat(entry.Name())
		if err != nil {
			return ErrUnavailable
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.Mode().IsRegular() || st.Uid != s.policy.CollectorUID || st.Gid != s.policy.SharedGID || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
			return ErrUnsafe
		}
		if st.Nlink == 0 {
			return ErrUnavailable
		}
		if st.Nlink != 1 {
			return ErrUnsafe
		}
		mode := info.Mode().Perm()
		switch entry.Name() {
		case latestName:
			if mode != 0o640 {
				return ErrUnsafe
			}
		case stageName:
			if mode != 0o600 && mode != 0o640 {
				return ErrUnsafe
			}
		case lockName:
			if mode != 0o600 || info.Size() != 0 {
				return ErrUnsafe
			}
		default:
			return ErrUnsafe
		}
		if info.Size() < 0 || info.Size() > remoteprojection.MaxBytes {
			return ErrUnsafe
		}
		total += info.Size()
	}
	if total > 2*remoteprojection.MaxBytes {
		return ErrUnsafe
	}
	return nil
}

func (s *slot) openFile(name string, flags int, limit int64, modes ...os.FileMode) (*os.File, error) {
	before, err := s.root.Lstat(name)
	if err == nil && !before.Mode().IsRegular() {
		return nil, ErrUnsafe
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, ErrUnsafe
	}
	f, err := s.root.OpenFile(name, flags|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600)
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
	if !os.SameFile(info, after) {
		f.Close()
		return nil, ErrUnavailable
	}
	if info.Size() < 0 || info.Size() > limit {
		f.Close()
		return nil, ErrUnsafe
	}
	if err := checkHandle(f, s.policy, false, modes...); err != nil {
		current, currentErr := s.root.Lstat(name)
		f.Close()
		if errors.Is(err, ErrUnavailable) || errors.Is(currentErr, os.ErrNotExist) || (currentErr == nil && !os.SameFile(info, current)) {
			return nil, ErrUnavailable
		}
		return nil, ErrUnsafe
	}
	return f, nil
}

func (w *Writer) Publish(raw observation.Snapshot, id remoteprojection.Identity) error {
	s := w.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil || s.lock == nil {
		return ErrUnavailable
	}
	if identity(s.policy, true) != nil {
		return ErrUnsafe
	}
	if id.SourceID != s.policy.ServerID {
		return ErrInvalid
	}
	data, err := remoteprojection.Encode(raw, id)
	if err != nil {
		return ErrInvalid
	}
	if err := s.checkDirectory(); err != nil {
		return err
	}
	if err := checkHandle(s.lock, s.policy, false, 0o600); err != nil {
		return err
	}
	if staged, err := s.openFile(stageName, os.O_RDONLY, remoteprojection.MaxBytes, 0o600, 0o640); err == nil {
		staged.Close()
		if s.root.Remove(stageName) != nil {
			return ErrUnavailable
		}
	} else if !errors.Is(err, ErrMissing) {
		return err
	}
	f, err := s.openFile(stageName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, remoteprojection.MaxBytes, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if n, err := s.writeFile(f, data); err != nil || n != len(data) {
		return ErrUnavailable
	}
	// Only our proven new, complete staging handle receives group-read access.
	if f.Chmod(0o640) != nil || checkHandle(f, s.policy, false, 0o640) != nil || s.syncFile(f) != nil {
		return ErrUnavailable
	}
	if f.Close() != nil {
		return ErrUnavailable
	}
	if old, err := s.openFile(latestName, os.O_RDONLY, remoteprojection.MaxBytes, 0o640); err == nil {
		old.Close()
	} else if !errors.Is(err, ErrMissing) {
		return err
	}
	if err := s.checkDirectory(); err != nil {
		return err
	}
	if s.replace(stageName, latestName) != nil || s.syncDir(s.directory) != nil {
		return ErrUnavailable
	}
	return nil
}

func (r *Reader) Read(ctx context.Context, expected string) ([]byte, error) {
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil || ctx == nil || ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	if identity(s.policy, false) != nil {
		return nil, ErrUnsafe
	}
	if expected != s.policy.ServerID {
		return nil, ErrInvalid
	}
	if err := s.checkDirectory(); err != nil {
		return nil, err
	}
	f, err := s.openFile(latestName, os.O_RDONLY, remoteprojection.MaxBytes, 0o640)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, remoteprojection.MaxBytes+1))
	if err != nil || ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	if len(data) > remoteprojection.MaxBytes || remoteprojection.Validate(data, expected) != nil {
		return nil, ErrInvalid
	}
	if err := checkHandle(f, s.policy, false, 0o640); err != nil {
		return nil, err
	}
	if err := s.checkDirectory(); err != nil {
		return nil, err
	}
	return data, nil
}

func (s *slot) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil {
		return nil
	}
	failed := false
	if s.lock != nil {
		failed = s.lock.Close() != nil
		s.lock = nil
	}
	failed = s.directory.Close() != nil || failed
	failed = s.root.Close() != nil || failed
	s.root = nil
	if failed {
		return ErrUnavailable
	}
	return nil
}
func (w *Writer) Close() error { return w.s.close() }
func (r *Reader) Close() error { return r.s.close() }
