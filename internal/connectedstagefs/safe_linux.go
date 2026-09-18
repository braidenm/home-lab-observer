//go:build linux

package connectedstagefs

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func regular(f *os.File, uid uint32, mode os.FileMode, limit int64) bool {
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != mode ||
		info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || info.Size() > limit {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uid && stat.Nlink == 1 && noACL(int(f.Fd()))
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
