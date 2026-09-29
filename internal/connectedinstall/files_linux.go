//go:build linux

// Package connectedinstall owns the fixed privileged installed-profile transaction.
// No operation is a remote API or accepts arbitrary destination paths/commands.
package connectedinstall

import (
	"errors"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

var ErrUnsafe = errors.New("connected_install_unsafe")
var ErrRecovery = errors.New("connected_install_recovery_required")

func regular(f *os.File, uid uint32, mode os.FileMode, limit int64) bool {
	i, err := f.Stat()
	if err != nil || !i.Mode().IsRegular() || i.Mode().Perm() != mode || i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || i.Size() > limit {
		return false
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	return ok && s.Uid == uid && s.Gid == 0 && s.Nlink == 1 && noACL(int(f.Fd()))
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
	for index, part := range parts {
		if part == "" || part == "." || part == ".." {
			unix.Close(fd)
			return nil, ErrUnsafe
		}
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if err != nil {
			if allowMissing && index == len(parts)-1 && err == unix.ENOENT {
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
