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
		return ErrUnsafe
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0o077 != 0 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return ErrUnsafe
	}
	if directory {
		if !info.IsDir() || info.Mode().Perm()&0o700 != 0o700 {
			return ErrUnsafe
		}
	} else if !info.Mode().IsRegular() || stat.Nlink != 1 {
		return ErrUnsafe
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
