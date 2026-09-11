//go:build linux || darwin

package handoff

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func privateHandle(file *os.File, directory bool) error {
	info, err := file.Stat()
	if err != nil {
		return errors.New("private_handle_stat_failed")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0o077 != 0 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return errors.New("private_handle_owner_or_mode")
	}
	if directory {
		if !info.IsDir() || info.Mode().Perm()&0o700 != 0o700 {
			return ErrUnsafe
		}
	} else if !info.Mode().IsRegular() {
		return errors.New("private_handle_not_regular")
	} else if stat.Nlink == 0 {
		// An open snapshot can be unlinked by a cooperating publisher. It must
		// not be read, but this is unavailability rather than a permission grant.
		return ErrUnavailable
	} else if stat.Nlink != 1 {
		return errors.New("private_handle_multiple_links")
	}
	return noExtendedACL(file)
}

func safeOpenFlags() int                { return unix.O_NOFOLLOW | unix.O_NONBLOCK }
func prepareCreatedFile(*os.File) error { return nil }
func syncDirectory(file *os.File) error { return file.Sync() }
func lockWriter(file *os.File) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return ErrBusy
	}
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
