//go:build linux

// Package connectedinstalllease owns only the fixed, privileged installation
// lease. Holding it is mutual exclusion, not permission to install or start.
package connectedinstalllease

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

const lockName = "home-lab-observer-connected.install.lock"

var ErrUnsafe = errors.New("connected_install_lease_unsafe")
var ErrBusy = errors.New("connected_install_lease_busy")

type Lease struct{ file *os.File }

// Acquire opens only the fixed /run/lock member. It does not create an
// installation, inspect an installed profile, or grant activation authority.
func Acquire() (*Lease, error) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return nil, ErrUnsafe
	}
	rootFD, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrUnsafe
	}
	defer unix.Close(rootFD)
	fd, err := unix.Openat2(rootFD, "run", &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS,
	})
	if err != nil {
		return nil, ErrUnsafe
	}
	run := os.NewFile(uintptr(fd), "connected-install-run-directory")
	defer run.Close()
	return acquireUnder(run)
}

// acquireUnder validates /run before opening its fixed lock child. A writable
// ancestor must not be able to replace the directory containing the lease.
func acquireUnder(run *os.File) (*Lease, error) {
	if run == nil || !trustedRun(run) {
		return nil, ErrUnsafe
	}
	fd, err := unix.Openat2(int(run.Fd()), "lock", &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS,
	})
	if err != nil {
		return nil, ErrUnsafe
	}
	parent := os.NewFile(uintptr(fd), "connected-install-lock-directory")
	defer parent.Close()
	return acquireAt(parent)
}

// acquireAt is a test boundary; the production entrypoint selects the only
// path. The caller must hold a root-owned, pinned directory descriptor.
func acquireAt(parent *os.File) (*Lease, error) {
	if parent == nil || os.Getuid() != 0 || os.Geteuid() != 0 || !trustedParent(parent) {
		return nil, ErrUnsafe
	}
	fd, err := unix.Openat(int(parent.Fd()), lockName, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, ErrUnsafe
	}
	f := os.NewFile(uintptr(fd), "connected-install-lease")
	if !trustedLock(f) {
		f.Close()
		return nil, ErrUnsafe
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		if err == unix.EWOULDBLOCK || err == unix.EAGAIN {
			return nil, ErrBusy
		}
		return nil, ErrUnsafe
	}
	if !trustedLock(f) {
		f.Close()
		return nil, ErrUnsafe
	}
	return &Lease{file: f}, nil
}

// Close releases the kernel lock but retains the fixed file. Unlinking it
// could let two installers lock different inodes concurrently.
func (l *Lease) Close() error {
	if l == nil || l.file == nil || l.file.Close() != nil {
		return ErrUnsafe
	}
	l.file = nil
	return nil
}

func trustedParent(parent *os.File) bool {
	i, err := parent.Stat()
	if err != nil || !i.IsDir() || i.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 ||
		(i.Mode().Perm()&0022 != 0 && i.Mode()&os.ModeSticky == 0) || !noACL(int(parent.Fd())) {
		return false
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	return ok && s.Uid == 0 && s.Gid == 0
}

func trustedRun(run *os.File) bool {
	i, err := run.Stat()
	if err != nil || !i.IsDir() || i.Mode().Perm()&0022 != 0 ||
		i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || !noACL(int(run.Fd())) {
		return false
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	return ok && s.Uid == 0 && s.Gid == 0
}

func trustedLock(f *os.File) bool {
	i, err := f.Stat()
	if err != nil || !i.Mode().IsRegular() || i.Mode().Perm() != 0600 || i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || i.Size() != 0 || !noACL(int(f.Fd())) {
		return false
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	return ok && s.Uid == 0 && s.Gid == 0 && s.Nlink == 1
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
