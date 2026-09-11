//go:build linux

package sharedhandoff

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/ownerfs"
)

var serverPattern = regexp.MustCompile(`^srv_[a-f0-9]{32}$`)

func validPolicy(p Policy) bool {
	return p.CollectorUID != 0 && p.UploaderUID != 0 && p.SharedGID != 0 && p.CollectorUID != p.UploaderUID && serverPattern.MatchString(p.ServerID)
}

func identity(p Policy, writer bool) error {
	if !validPolicy(p) {
		return ErrUnsafe
	}
	uid := p.UploaderUID
	if writer {
		uid = p.CollectorUID
	}
	ruid, euid, suid := unix.Getresuid()
	if ruid != int(uid) || euid != int(uid) || suid != int(uid) {
		return ErrUnsafe
	}
	rgid, egid, sgid := unix.Getresgid()
	if !consistentGroups(rgid, egid, sgid) {
		return ErrUnsafe
	}
	var capabilities [2]unix.CapUserData
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	if unix.Capget(&header, &capabilities[0]) != nil {
		return ErrUnsafe
	}
	for _, c := range capabilities {
		if c.Effective != 0 || c.Permitted != 0 || c.Inheritable != 0 {
			return ErrUnsafe
		}
	}
	if writer && os.Getegid() != int(p.SharedGID) {
		return ErrUnsafe
	}
	if os.Getegid() == int(p.SharedGID) {
		return nil
	}
	groups, err := os.Getgroups()
	if err != nil {
		return ErrUnsafe
	}
	for _, gid := range groups {
		if gid == int(p.SharedGID) {
			return nil
		}
	}
	return ErrUnsafe
}

func consistentGroups(real, effective, saved int) bool {
	return real > 0 && real == effective && effective == saved
}

func safePath(path string, p Policy) error {
	if ownerfs.ValidateDedicatedDirectory(path) != nil {
		return ErrUnsafe
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ErrUnsafe
	}
	for parent := filepath.Dir(abs); ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err != nil {
			return ErrUnsafe
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || (st.Uid != 0 && st.Uid != p.CollectorUID) || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ErrUnsafe
		}
		if info.Mode().Perm()&0o022 != 0 && !(st.Uid == 0 && info.Mode()&os.ModeSticky != 0) {
			return ErrUnsafe
		}
		if parent == filepath.Dir(parent) {
			break
		}
	}
	return nil
}

func noACL(file *os.File, directory bool) error {
	conn, err := file.SyscallConn()
	if err != nil {
		return ErrUnsafe
	}
	var policyErr error
	err = conn.Control(func(fd uintptr) {
		names := []string{"system.posix_acl_access"}
		if directory {
			names = append(names, "system.posix_acl_default")
		}
		for _, name := range names {
			_, err := unix.Fgetxattr(int(fd), name, nil)
			if !errors.Is(err, unix.ENODATA) {
				policyErr = ErrUnsafe
				return
			}
		}
	})
	if err != nil || policyErr != nil {
		return ErrUnsafe
	}
	return nil
}

func checkHandle(file *os.File, p Policy, directory bool, modes ...os.FileMode) error {
	info, err := file.Stat()
	if err != nil {
		return ErrUnavailable
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != p.CollectorUID || st.Gid != p.SharedGID || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return ErrUnsafe
	}
	match := false
	for _, mode := range modes {
		if info.Mode().Perm() == mode {
			match = true
		}
	}
	if !match {
		return ErrUnsafe
	}
	if directory {
		if !info.IsDir() {
			return ErrUnsafe
		}
	} else {
		if !info.Mode().IsRegular() {
			return ErrUnsafe
		}
		if st.Nlink == 0 {
			return ErrUnavailable
		}
		if st.Nlink != 1 {
			return ErrUnsafe
		}
	}
	return noACL(file, directory)
}
