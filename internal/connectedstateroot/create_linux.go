//go:build linux

// Package connectedstateroot creates only the fixed first-install state root.
// It does not acquire the installation lease or activate a worker.
package connectedstateroot

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

const stateName = "home-lab-observer-connected"

var ErrUnsafe = errors.New("connected_state_root_unsafe")
var ErrRecovery = errors.New("connected_state_root_recovery_required")

// Create reaches only /var/lib/home-lab-observer-connected. The future caller
// must separately hold the lease and prove an entirely new installation.
func Create() error {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return ErrUnsafe
	}
	rootFD, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrUnsafe
	}
	defer unix.Close(rootFD)
	varDir, err := openDirAt(rootFD, "var")
	if err != nil {
		return ErrUnsafe
	}
	defer varDir.Close()
	return createAt(varDir, nil)
}

// createAt accepts a pinned /var only for an explicit synthetic-root fixture.
// No existing target or failed operation is adopted, repaired or cleaned up.
func createAt(varDir *os.File, syncFile func(*os.File) error) (result error) {
	if varDir == nil || os.Getuid() != 0 || os.Geteuid() != 0 || !trustedDirectory(varDir, false) {
		return ErrUnsafe
	}
	lib, err := openDirAt(int(varDir.Fd()), "lib")
	if err != nil {
		return ErrUnsafe
	}
	defer lib.Close()
	if !trustedDirectory(lib, false) {
		return ErrUnsafe
	}
	if syncFile == nil {
		syncFile = (*os.File).Sync
	}
	if unix.Mkdirat(int(lib.Fd()), stateName, 0700) != nil {
		return ErrRecovery
	}
	state, err := openDirAt(int(lib.Fd()), stateName)
	if err != nil {
		return ErrRecovery
	}
	defer func() {
		if state.Close() != nil && result == nil {
			result = ErrRecovery
		}
	}()
	if state.Chown(0, 0) != nil || state.Chmod(0755) != nil || !trustedDirectory(state, true) ||
		syncFile(state) != nil || syncFile(lib) != nil {
		return ErrRecovery
	}
	return nil
}

func openDirAt(parent int, name string) (*os.File, error) {
	fd, err := unix.Openat2(parent, name, &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS,
	})
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

func trustedDirectory(dir *os.File, exactMode bool) bool {
	i, err := dir.Stat()
	if err != nil || !i.IsDir() || i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 ||
		i.Mode().Perm()&0022 != 0 || exactMode && i.Mode().Perm() != 0755 || !noACL(int(dir.Fd())) {
		return false
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	return ok && s.Uid == 0 && s.Gid == 0
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
