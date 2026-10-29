//go:build linux

package dockerreader

import (
	"errors"

	"golang.org/x/sys/unix"
)

// A root-owned bind mount pins the approved node. Never resolve a new endpoint,
// follow a symlink, repair permissions, or adopt a replacement during a run.
type socketWitness struct {
	device, inode uint64
	group         uint32
}

func captureSocket() (socketWitness, error) {
	return captureSocketAt("/engine/docker.sock")
}

func captureSocketAt(path string) (socketWitness, error) {
	var st unix.Stat_t
	if unix.Lstat(path, &st) != nil || st.Mode != unix.S_IFSOCK|0660 || st.Uid != 0 || st.Gid == 0 || st.Nlink != 1 {
		return socketWitness{}, ErrUnsafe
	}
	if _, err := unix.Lgetxattr(path, "system.posix_acl_access", nil); !errors.Is(err, unix.ENODATA) {
		return socketWitness{}, ErrUnsafe
	}
	return socketWitness{uint64(st.Dev), st.Ino, st.Gid}, nil
}

func (s socketWitness) check() error { return s.checkAt("/engine/docker.sock") }

func (s socketWitness) checkAt(path string) error {
	current, err := captureSocketAt(path)
	if err != nil || s.inode == 0 || current != s {
		return ErrUnsafe
	}
	return nil
}
