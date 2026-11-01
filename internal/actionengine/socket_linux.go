//go:build linux

package actionengine

import (
	"errors"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const socketPath = "/engine/docker.sock"

type socketEndpoint struct {
	path     string
	owner    uint32
	approved SocketWitness
}

func noACL(path string) bool {
	_, err := unix.Lgetxattr(path, "system.posix_acl_access", nil)
	return errors.Is(err, unix.ENODATA)
}

func directory(path string, owner uint32) bool {
	var st unix.Stat_t
	return unix.Lstat(path, &st) == nil && st.Mode&unix.S_IFMT == unix.S_IFDIR && st.Uid == owner &&
		st.Mode&007022 == 0 && noACL(path)
}

func (e socketEndpoint) check() error {
	var st unix.Stat_t
	if !filepath.IsAbs(e.path) || filepath.Clean(e.path) != e.path || e.approved.Inode == 0 ||
		!directory(filepath.Dir(e.path), e.owner) || unix.Lstat(e.path, &st) != nil ||
		st.Mode != unix.S_IFSOCK|0660 || st.Uid != e.owner || st.Gid != e.approved.GroupID || st.Nlink != 1 ||
		uint64(st.Dev) != e.approved.Device || st.Ino != e.approved.Inode || !noACL(e.path) {
		return ErrUnsafe
	}
	return nil
}
