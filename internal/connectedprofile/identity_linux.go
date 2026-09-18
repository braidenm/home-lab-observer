//go:build linux

package connectedprofile

import "golang.org/x/sys/unix"

func CheckIdentity(c Config, collector bool) error {
	// Numeric identity alone is not the installed privilege boundary: the
	// manager must already have made elevation impossible.
	nnp, err := unix.PrctlRetInt(unix.PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0)
	if err != nil || nnp != 1 {
		return ErrUnsafe
	}
	uid, gid := c.UploaderUID, c.UploaderGID
	if collector {
		uid, gid = c.CollectorUID, c.SharedGID
	}
	r, e, s := unix.Getresuid()
	gr, ge, gs := unix.Getresgid()
	if r != int(uid) || e != int(uid) || s != int(uid) || gr != int(gid) || ge != int(gid) || gs != int(gid) {
		return ErrUnsafe
	}
	groups, err := unix.Getgroups()
	if err != nil {
		return ErrUnsafe
	}
	seenShared := false
	for _, g := range groups {
		if g != int(gid) && g != int(c.SharedGID) {
			return ErrUnsafe
		}
		if g == int(c.SharedGID) {
			seenShared = true
		}
	}
	if !collector && !seenShared {
		return ErrUnsafe
	}
	h := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	d := [2]unix.CapUserData{}
	if unix.Capget(&h, &d[0]) != nil {
		return ErrUnsafe
	}
	for _, v := range d {
		if v.Effective != 0 || v.Permitted != 0 || v.Inheritable != 0 {
			return ErrUnsafe
		}
	}
	return nil
}
