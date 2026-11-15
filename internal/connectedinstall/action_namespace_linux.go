//go:build linux

package connectedinstall

import (
	"context"

	"github.com/braidenm/home-lab-observer/internal/actionprofile"
	"golang.org/x/sys/unix"
)

// prepareActionNamespace creates the fixed reference, never adopting an
// existing file or mount. The caller holds the stopped installation lease.
func prepareActionNamespace(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || unix.Getuid() != 0 || unix.Geteuid() != 0 {
		return ErrUnsafe
	}
	// A private mount view would not publish the reference to the manager.
	for _, name := range []string{"mnt", "user"} {
		var self, init unix.Stat_t
		if unix.Stat("/proc/self/ns/"+name, &self) != nil || unix.Stat("/proc/1/ns/"+name, &init) != nil ||
			self.Dev != init.Dev || self.Ino != init.Ino {
			return ErrUnsafe
		}
	}
	parent, err := rootDirectory(actionRunRoot)
	if err != nil {
		return ErrUnsafe
	}
	defer parent.Close()
	tree, err := unix.OpenTree(unix.AT_FDCWD, "/proc/1/ns/user", unix.OPEN_TREE_CLONE|unix.OPEN_TREE_CLOEXEC)
	if err != nil {
		return ErrUnsafe
	}
	defer unix.Close(tree)
	var fs unix.Statfs_t
	if unix.Fstatfs(tree, &fs) != nil || fs.Type != unix.NSFS_MAGIC ||
		unix.MountSetattr(tree, "", unix.AT_EMPTY_PATH, &unix.MountAttr{Attr_set: unix.MOUNT_ATTR_RDONLY}) != nil {
		return ErrUnsafe
	}
	if ctx.Err() != nil {
		return ErrUnsafe
	}
	dest, err := unix.Openat(int(parent.Fd()), "host-userns", unix.O_RDONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0644)
	if err != nil {
		return ErrRecovery
	}
	defer unix.Close(dest)
	if unix.Fchmod(dest, 0644) != nil || ctx.Err() != nil ||
		unix.MoveMount(tree, "", dest, "", unix.MOVE_MOUNT_F_EMPTY_PATH|unix.MOVE_MOUNT_T_EMPTY_PATH) != nil {
		return ErrRecovery
	}
	if actionprofile.VerifyHostNamespace() != nil {
		return ErrRecovery
	}
	return nil
}
