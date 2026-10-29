//go:build linux

package connectedprofile

import "golang.org/x/sys/unix"

func LoadRichProcessesDockerConfig() (RichProcessesDockerConfig, error) {
	b, err := ReadRootFileMode(RichProcessesDockerConfigPath, MaxConfigBytes, 0644)
	if err != nil {
		return RichProcessesDockerConfig{}, ErrUnsafe
	}
	return DecodeRichProcessesDockerConfig(b)
}

func LoadRichProcessesDockerPolicy() (RichProcessesDockerPolicy, error) {
	b, err := ReadRootFileMode(RichProcessesDockerPolicyPath, RichProcessesDockerMaxPolicyBytes, 0644)
	if err != nil {
		return RichProcessesDockerPolicy{}, ErrUnsafe
	}
	return DecodeRichProcessesDockerPolicy(b)
}

func CheckRichProcessesDockerIdentity(c RichProcessesDockerConfig, collector bool) error {
	if c.Validate() != nil {
		return ErrUnsafe
	}
	if collector {
		return checkDockerIdentity(c.CollectorUID, c.SharedGID, c.FrameGID)
	}
	return checkDockerIdentity(c.UploaderUID, c.UploaderGID, c.SharedGID)
}

func CheckRichProcessesDockerReaderIdentity(c RichProcessesDockerConfig) error {
	if c.Validate() != nil {
		return ErrUnsafe
	}
	return checkDockerIdentity(c.ReaderUID, c.FrameGID, c.SocketGID)
}

// Each worker receives exactly its primary group and the one service-only
// supplementary grant. No NSS Docker membership or ambient capability counts.
func checkDockerIdentity(uid, gid, supplementary uint32) error {
	nnp, err := unix.PrctlRetInt(unix.PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0)
	if err != nil || nnp != 1 {
		return ErrUnsafe
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
	found := false
	for _, g := range groups {
		if g != int(gid) && g != int(supplementary) {
			return ErrUnsafe
		}
		if g == int(supplementary) {
			found = true
		}
	}
	if !found {
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
