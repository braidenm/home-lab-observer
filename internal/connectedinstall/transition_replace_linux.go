//go:build linux

package connectedinstall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// replaceTransitionAt is a private filesystem primitive, not an installer
// command. Its future caller must hold the installation lease, prove a
// complete stage and published authoritative proposal, stop both workers,
// and pin the private ledger before supplying code-derived previous/next
// bytes. Failure preserves evidence for explicit forward recovery.
func replaceTransitionAt(
	ctx context.Context,
	parent *os.File,
	target, role string,
	proposal []byte,
	previous, next []byte,
	mode os.FileMode,
	limit int,
	after func(string) error,
) error {
	if ctx == nil || ctx.Err() != nil || parent == nil || os.Getuid() != 0 || os.Geteuid() != 0 ||
		!transitionRole(role, target) || len(proposal) == 0 || len(proposal) > 16<<10 ||
		len(previous) == 0 || len(previous) > limit || len(next) == 0 || len(next) > limit ||
		(mode != 0600 && mode != 0644) || limit <= 0 || limit > 1<<20 {
		return ErrUnsafe
	}
	var fs unix.Statfs_t
	if unix.Fstatfs(int(parent.Fd()), &fs) != nil || fs.Type != unix.EXT4_SUPER_MAGIC || !transitionParent(parent) {
		return ErrUnsafe
	}
	digest := sha256.Sum256(proposal)
	temp := ".observer-transition-" + role + "-" + hex.EncodeToString(digest[:])
	current, identity, err := readTransitionMember(parent, target, mode, limit)
	if err != nil || (!bytes.Equal(current, previous) && !bytes.Equal(current, next)) {
		return ErrRecovery
	}
	if err := clearTransitionTemp(parent, temp, mode, limit); err != nil {
		return ErrRecovery
	}
	if bytes.Equal(current, next) {
		confirmed, confirmedIdentity, err := readTransitionMember(parent, target, mode, limit)
		if err != nil || !os.SameFile(identity, confirmedIdentity) || !bytes.Equal(confirmed, next) || ctx.Err() != nil {
			return ErrRecovery
		}
		return nil
	}
	if ctx.Err() != nil {
		return ErrRecovery
	}
	fd, err := unix.Openat2(int(parent.Fd()), temp, &unix.OpenHow{
		Flags: uint64(unix.O_WRONLY | unix.O_CREAT | unix.O_EXCL | unix.O_NOFOLLOW | unix.O_CLOEXEC),
		Mode: uint64(mode),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV,
	})
	if err != nil {
		return ErrRecovery
	}
	f := os.NewFile(uintptr(fd), temp)
	if f.Chown(0, 0) != nil || f.Chmod(mode) != nil || !transitionRegular(f, mode, limit, true) {
		f.Close()
		return ErrRecovery
	}
	n, writeErr := f.Write(next)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || n != len(next) || syncErr != nil || closeErr != nil ||
		(after != nil && after("file-sync") != nil) {
		return ErrRecovery
	}
	// A concurrent replacement, even by another root process, invalidates the
	// earlier byte observation. Never rename onto a changed target.
	checked, checkedIdentity, err := readTransitionMember(parent, target, mode, limit)
	if err != nil || !os.SameFile(identity, checkedIdentity) || !bytes.Equal(checked, previous) || ctx.Err() != nil {
		return ErrRecovery
	}
	if unix.Renameat(int(parent.Fd()), temp, int(parent.Fd()), target) != nil {
		return ErrRecovery
	}
	if after != nil && after("rename") != nil {
		return ErrRecovery
	}
	if parent.Sync() != nil || (after != nil && after("parent-sync") != nil) {
		return ErrRecovery
	}
	observed, _, err := readTransitionMember(parent, target, mode, limit)
	if err != nil || !bytes.Equal(observed, next) || ctx.Err() != nil {
		return ErrRecovery
	}
	return nil
}

func transitionRole(role, target string) bool {
	switch role {
	case "ca":
		return target == "ca-certificates.crt"
	case "hosts":
		return target == "hosts"
	case "collector":
		return target == collectorUnit
	case "uploader":
		return target == uploaderUnit
	case "config":
		return target == "installed.json"
	}
	return false
}

func transitionParent(parent *os.File) bool {
	i, err := parent.Stat()
	if err != nil || !i.IsDir() || i.Mode().Perm()&0022 != 0 || i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || !noACL(int(parent.Fd())) {
		return false
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	return ok && s.Uid == 0 && s.Gid == 0
}

func transitionRegular(f *os.File, mode os.FileMode, limit int, allowEmpty bool) bool {
	if !regular(f, 0, mode, int64(limit)) {
		return false
	}
	i, err := f.Stat()
	if err != nil || (!allowEmpty && i.Size() == 0) || i.Size() < 0 {
		return false
	}
	s, ok := i.Sys().(*syscall.Stat_t)
	return ok && s.Gid == 0
}

func readTransitionMember(parent *os.File, name string, mode os.FileMode, limit int) ([]byte, os.FileInfo, error) {
	fd, err := unix.Openat2(int(parent.Fd()), name, &unix.OpenHow{
		Flags: uint64(unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV,
	})
	if err != nil {
		return nil, nil, ErrUnsafe
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	if !transitionRegular(f, mode, limit, false) {
		return nil, nil, ErrUnsafe
	}
	before, err := f.Stat()
	if err != nil {
		return nil, nil, ErrUnsafe
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	after, statErr := f.Stat()
	if err != nil || statErr != nil || len(b) == 0 || len(b) > limit || !os.SameFile(before, after) || !transitionRegular(f, mode, limit, false) {
		return nil, nil, ErrUnsafe
	}
	return b, before, nil
}

func clearTransitionTemp(parent *os.File, temp string, mode os.FileMode, limit int) error {
	var st unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), temp, &st, unix.AT_SYMLINK_NOFOLLOW); err == unix.ENOENT {
		return nil
	} else if err != nil {
		return ErrUnsafe
	}
	fd, err := unix.Openat2(int(parent.Fd()), temp, &unix.OpenHow{
		Flags: uint64(unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV,
	})
	if err != nil {
		return ErrUnsafe
	}
	f := os.NewFile(uintptr(fd), temp)
	valid := transitionRegular(f, mode, limit, true)
	before, statErr := f.Stat()
	closeErr := f.Close()
	if !valid || statErr != nil || closeErr != nil {
		return ErrUnsafe
	}
	var current unix.Stat_t
	if unix.Fstatat(int(parent.Fd()), temp, &current, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		current.Mode&unix.S_IFMT != unix.S_IFREG || current.Uid != 0 || current.Gid != 0 || current.Nlink != 1 ||
		before.Size() != current.Size || before.Mode().Perm() != os.FileMode(current.Mode&0777) {
		return ErrUnsafe
	}
	owner, ok := before.Sys().(*syscall.Stat_t)
	if !ok || owner.Ino != current.Ino || owner.Dev != current.Dev {
		return ErrUnsafe
	}
	if unix.Unlinkat(int(parent.Fd()), temp, 0) != nil || parent.Sync() != nil {
		return ErrRecovery
	}
	return nil
}
