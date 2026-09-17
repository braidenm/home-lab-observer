//go:build linux

// Package connectedinstall owns the fixed privileged installed-profile transaction.
// No operation is a remote API or accepts arbitrary destination paths/commands.
package connectedinstall

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

var ErrUnsafe = errors.New("connected_install_unsafe")
var ErrRecovery = errors.New("connected_install_recovery_required")

type lease struct{ file *os.File }

func acquireLease() (*lease, error) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return nil, ErrUnsafe
	}
	parent, err := os.OpenRoot("/run/lock")
	if err != nil {
		return nil, ErrUnsafe
	}
	defer parent.Close()
	info, err := parent.Stat(".")
	if err != nil {
		return nil, ErrUnsafe
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 || !info.IsDir() || (info.Mode().Perm()&0022 != 0 && info.Mode()&os.ModeSticky == 0) {
		return nil, ErrUnsafe
	}
	d, err := parent.Open(".")
	if err != nil {
		return nil, ErrUnsafe
	}
	defer d.Close()
	fd, err := unix.Openat(int(d.Fd()), "home-lab-observer-connected.install.lock", unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, ErrUnsafe
	}
	f := os.NewFile(uintptr(fd), "installation-lease")
	if !regular(f, 0, 0600, 0) || unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		f.Close()
		return nil, ErrUnsafe
	}
	return &lease{f}, nil
}

func (l *lease) Close() error { return l.file.Close() }

func regular(f *os.File, uid uint32, mode os.FileMode, limit int64) bool {
	i, err := f.Stat()
	if err != nil || !i.Mode().IsRegular() || i.Mode().Perm() != mode || i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || i.Size() > limit {
		return false
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	return ok && s.Uid == uid && s.Nlink == 1 && noACL(int(f.Fd()))
}

func noACL(fd int) bool {
	for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
		n, err := unix.Fgetxattr(fd, name, nil)
		if err == unix.ENODATA || err == unix.ENOTSUP {
			continue
		}
		if err != nil || n != 0 {
			return false
		}
	}
	return true
}

// publishNew never overwrites an unknown target and syncs both content and the
// containing directory. Failure preserves evidence; callers must not retry setup.
func publishNew(root *os.Root, name string, data []byte, mode os.FileMode) error {
	return publishBounded(root, name, data, mode, 64*1024)
}

func publishCA(root *os.Root, data []byte) error {
	return publishBounded(root, "ca-certificates.crt", data, 0644, 1024*1024)
}

func publishBounded(root *os.Root, name string, data []byte, mode os.FileMode, limit int) error {
	if len(data) == 0 || len(data) > limit {
		return ErrUnsafe
	}
	f, err := root.OpenFile(".install-next", os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return ErrRecovery
	}
	if f.Chmod(mode) != nil {
		f.Close()
		return ErrRecovery
	}
	if !regular(f, 0, mode, 0) {
		f.Close()
		return ErrRecovery
	}
	n, err := f.Write(data)
	if err != nil || n != len(data) || f.Sync() != nil {
		f.Close()
		return ErrRecovery
	}
	if f.Close() != nil {
		return ErrRecovery
	}
	d, err := root.Open(".")
	if err != nil {
		return ErrRecovery
	}
	defer d.Close()
	if unix.Renameat2(int(d.Fd()), ".install-next", int(d.Fd()), name, unix.RENAME_NOREPLACE) != nil || d.Sync() != nil {
		return ErrRecovery
	}
	return nil
}

func mkdirNew(path string, uid, gid int, mode os.FileMode) error {
	if os.Mkdir(path, 0700) != nil {
		return ErrRecovery
	}
	f, err := os.Open(path)
	if err != nil {
		return ErrRecovery
	}
	defer f.Close()
	if f.Chown(uid, gid) != nil || f.Chmod(mode) != nil || !noACL(int(f.Fd())) || f.Sync() != nil {
		return ErrRecovery
	}
	parent, err := os.Open(filepath.Dir(path))
	if err != nil {
		return ErrRecovery
	}
	defer parent.Close()
	if parent.Sync() != nil {
		return ErrRecovery
	}
	return nil
}

func rootDirectory(path string) (*os.File, error) {
	return trustedDirectory(path, false)
}

// trustedDirectory permits a missing descendant only after validating all
// existing ancestors through anchored, no-follow descriptors.
func trustedDirectory(path string, allowMissing bool) (*os.File, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, ErrUnsafe
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrUnsafe
	}
	return trustedDescendant(fd, strings.Split(strings.TrimPrefix(path, "/"), "/"), allowMissing)
}

// trustedDescendant consumes fd, which must already anchor a trusted parent.
func trustedDescendant(fd int, parts []string, allowMissing bool) (*os.File, error) {
	var parent unix.Stat_t
	if unix.Fstat(fd, &parent) != nil || parent.Mode&unix.S_IFMT != unix.S_IFDIR || parent.Uid != 0 || parent.Mode&0022 != 0 || parent.Mode&07000 != 0 || !noACL(fd) {
		unix.Close(fd)
		return nil, ErrUnsafe
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			unix.Close(fd)
			return nil, ErrUnsafe
		}
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if err != nil {
			if allowMissing && err == unix.ENOENT {
				return nil, nil
			}
			return nil, ErrUnsafe
		}
		fd = next
		var s unix.Stat_t
		if unix.Fstat(fd, &s) != nil || s.Uid != 0 || s.Mode&0022 != 0 || s.Mode&07000 != 0 || !noACL(fd) {
			unix.Close(fd)
			return nil, ErrUnsafe
		}
	}
	return os.NewFile(uintptr(fd), "root-owned-directory"), nil
}
