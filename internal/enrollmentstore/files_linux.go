//go:build linux

package enrollmentstore

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/ownerfs"
)

func noACL(f *os.File, directory bool) bool {
	names := []string{"system.posix_acl_access"}
	if directory {
		names = append(names, "system.posix_acl_default")
	}
	for _, name := range names {
		_, err := unix.Fgetxattr(int(f.Fd()), name, nil)
		if !errors.Is(err, unix.ENODATA) {
			return false
		}
	}
	return true
}

func private(f *os.File, directory bool, limit int64) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return false
	}
	if directory {
		return info.IsDir() && info.Mode().Perm() == 0o700 && noACL(f, true)
	}
	return info.Mode().IsRegular() && info.Mode().Perm() == 0o600 && st.Nlink == 1 && info.Size() >= 0 && info.Size() <= limit && noACL(f, false)
}

func openDirectory(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrUnsafe
	}
	if !private(f, true, 0) {
		f.Close()
		return nil, ErrUnsafe
	}
	return f, nil
}

func safePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil || ownerfs.ValidateDedicatedDirectory(abs) != nil {
		return "", ErrUnsafe
	}
	for p := abs; ; p = filepath.Dir(p) {
		i, err := os.Lstat(p)
		if err != nil || !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
			return "", ErrUnsafe
		}
		st, ok := i.Sys().(*syscall.Stat_t)
		if !ok || (st.Uid != 0 && st.Uid != uint32(os.Geteuid())) {
			return "", ErrUnsafe
		}
		if p != abs && i.Mode().Perm()&0o022 != 0 && i.Mode()&os.ModeSticky == 0 {
			return "", ErrUnsafe
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	return abs, nil
}

// The root handle remains pinned. Path checks detect accidental replacement;
// a malicious current UID or root is not isolated by this library.
func (s *Setup) checkRoot() error {
	if !private(s.dir, true, 0) {
		return ErrUnsafe
	}
	want, err := s.dir.Stat()
	got, pathErr := os.Lstat(s.path)
	if err != nil || pathErr != nil || !os.SameFile(want, got) {
		return ErrUnsafe
	}
	return nil
}

func (s *Setup) file(name string, create bool, limit int64) (*os.File, error) {
	flags := os.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if create {
		flags = os.O_RDWR | os.O_CREATE | os.O_EXCL | unix.O_NOFOLLOW | unix.O_NONBLOCK
	}
	f, err := s.root.OpenFile(name, flags, 0o600)
	if err != nil {
		return nil, ErrRecovery
	}
	if !private(f, false, limit) {
		f.Close()
		return nil, ErrUnsafe
	}
	want, err := f.Stat()
	got, pathErr := s.root.Lstat(name)
	if err != nil || pathErr != nil || !os.SameFile(want, got) {
		f.Close()
		return nil, ErrUnsafe
	}
	return f, nil
}

func (s *Setup) entries() (map[string]bool, error) {
	if err := s.checkRoot(); err != nil {
		return nil, err
	}
	dir, err := s.root.Open(".")
	if err != nil {
		return nil, ErrUnsafe
	}
	entries, readErr := dir.ReadDir(7)
	closeErr := dir.Close()
	if (readErr != nil && !errors.Is(readErr, io.EOF)) || closeErr != nil || len(entries) > 6 {
		return nil, ErrUnsafe
	}
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if name == ledgerName {
			f, err := s.root.OpenFile(name, os.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
			if err != nil {
				return nil, ErrUnsafe
			}
			ok := private(f, true, 0)
			closeErr := f.Close()
			if !ok || closeErr != nil {
				return nil, ErrUnsafe
			}
		} else {
			limit := int64(maxRecord)
			switch name {
			case lockName:
				limit = 0
			case stageName, attemptName, credentialName, readyName:
			default:
				return nil, ErrUnsafe
			}
			f, err := s.file(name, false, limit)
			if err != nil {
				return nil, err
			}
			if f.Close() != nil {
				return nil, ErrRecovery
			}
		}
		seen[name] = true
	}
	return seen, nil
}

func (s *Setup) read(name, state string) (record, error) {
	f, err := s.file(name, false, maxRecord)
	if err != nil {
		return record{}, err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, maxRecord+1))
	defer clear(data)
	ok := private(f, false, maxRecord)
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || !ok {
		return record{}, ErrRecovery
	}
	return decode(data, s.binding, state)
}

// This check is only called before D1 opens a connection, or after it closes.
// Closing an unrelated database descriptor while SQLite holds POSIX locks can
// release those locks. D1 remains responsible for schema, bounds and recovery.
func (s *Setup) ledgerPrivacy() error {
	d, err := s.root.OpenRoot(ledgerName)
	if err != nil {
		return ErrUnsafe
	}
	defer d.Close()
	f, err := d.Open(".")
	if err != nil {
		return ErrUnsafe
	}
	entries, readErr := f.ReadDir(4)
	closeErr := f.Close()
	if (readErr != nil && !errors.Is(readErr, io.EOF)) || closeErr != nil || len(entries) > 3 {
		return ErrUnsafe
	}
	for _, entry := range entries {
		limit := int64(1024 * 1024)
		switch entry.Name() {
		case "upload.sqlite":
			limit = 4096 * 64
		case ".upload-lock":
			limit = 0
		case "upload.sqlite-journal":
		default:
			return ErrUnsafe
		}
		f, err := d.OpenFile(entry.Name(), os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			return ErrUnsafe
		}
		ok := private(f, false, limit)
		closeErr := f.Close()
		if !ok || closeErr != nil {
			return ErrUnsafe
		}
	}
	return nil
}

// A hook receives only fixed stage names, never a path or credential. Tests can
// interrupt exact durability boundaries; nil always invokes real operations.
func (s *Setup) boundary(stage string) error {
	if s.hook != nil {
		return s.hook(stage)
	}
	return nil
}

func (s *Setup) publish(name string, data []byte) error {
	if len(data) == 0 || len(data) > maxRecord || s.checkRoot() != nil {
		return ErrRecovery
	}
	f, err := s.file(stageName, true, maxRecord)
	if err != nil {
		return err
	}
	if s.boundary("before_write") != nil {
		f.Close()
		return ErrRecovery
	}
	n, err := f.Write(data)
	if err != nil || n != len(data) || s.boundary("after_write") != nil {
		f.Close()
		return ErrRecovery
	}
	if f.Sync() != nil || s.boundary("after_file_sync") != nil {
		f.Close()
		return ErrRecovery
	}
	if f.Close() != nil || s.boundary("after_close") != nil {
		return ErrRecovery
	}
	if unix.Renameat2(int(s.dir.Fd()), stageName, int(s.dir.Fd()), name, unix.RENAME_NOREPLACE) != nil {
		return ErrRecovery
	}
	if s.boundary("after_rename") != nil || s.dir.Sync() != nil || s.boundary("after_directory_sync") != nil {
		return ErrRecovery
	}
	return nil
}
