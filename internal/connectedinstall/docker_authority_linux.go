//go:build linux

package connectedinstall

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

// auditDockerPrincipals adds the isolated reader/frame authority to the existing
// pair audit. Socket access is service-only: no persistent NSS membership grants
// the reader (or the collector) membership in the host daemon's group.
func auditDockerPrincipals(parent context.Context, c connectedprofile.RichProcessesDockerConfig) error {
	if parent == nil || c.Validate() != nil || auditPrincipalsForPair(parent, dockerWorkerBinding(c), richProcessesDockerPair) != nil {
		return ErrUnsafe
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	nss, err := connectedprofile.ReadRootFile("/etc/nsswitch.conf", 64<<10)
	if err != nil {
		return ErrUnsafe
	}
	passwd, err := connectedprofile.ReadRootFile("/etc/passwd", 1<<20)
	if err != nil {
		return ErrUnsafe
	}
	groups, err := connectedprofile.ReadRootFile("/etc/group", 1<<20)
	if err != nil {
		return ErrUnsafe
	}
	snapshot, err := parseDockerReaderPrincipals(nss, passwd, groups, c)
	if err != nil {
		return ErrUnsafe
	}
	name := connectedprofile.RichProcessesDockerReaderName
	for _, key := range []string{name, strconv.FormatUint(uint64(c.ReaderUID), 10)} {
		out, err := runTool(ctx, toolGetent, []string{"passwd", key}, nil, 4096)
		if err != nil || string(out) != snapshot.users[name]+"\n" {
			return ErrUnsafe
		}
	}
	out, err := runTool(ctx, toolGetent, []string{"initgroups", name}, nil, 4096)
	if err != nil || !exactInitgroups(out, name, nil) {
		return ErrUnsafe
	}
	out, err = runTool(ctx, toolPasswd, []string{"--status", "--", name}, nil, 1024)
	if err != nil || !lockedStatus(out, name) {
		return ErrUnsafe
	}
	group := connectedprofile.RichProcessesDockerFrameName
	for _, key := range []string{group, strconv.FormatUint(uint64(c.FrameGID), 10)} {
		out, err := runTool(ctx, toolGetent, []string{"group", key}, nil, 4096)
		if err != nil || string(out) != snapshot.groups[group]+"\n" {
			return ErrUnsafe
		}
	}
	out, err = runTool(ctx, toolGetent, []string{"gshadow", group}, nil, 4096)
	valid := err == nil && lockedGroup(out, group, "")
	clear(out)
	if !valid || ctx.Err() != nil {
		return ErrUnsafe
	}
	return nil
}

func parseDockerReaderPrincipals(nss, passwd, groups []byte, c connectedprofile.RichProcessesDockerConfig) (principalSnapshot, error) {
	result := principalSnapshot{users: map[string]string{}, groups: map[string]string{}, members: map[string]string{}}
	if c.Validate() != nil || !supportedNSS(nss) || len(passwd) == 0 || len(passwd) > 1<<20 || len(groups) == 0 || len(groups) > 1<<20 || bytes.ContainsAny(passwd, "\r\x00") || bytes.ContainsAny(groups, "\r\x00") {
		return result, ErrUnsafe
	}
	name, group := connectedprofile.RichProcessesDockerReaderName, connectedprofile.RichProcessesDockerFrameName
	for _, line := range strings.Split(strings.TrimSuffix(string(passwd), "\n"), "\n") {
		f := strings.Split(line, ":")
		if len(f) != 7 || len(line) > 4096 {
			return result, ErrUnsafe
		}
		uid, ok := numericID(f[2])
		if !ok {
			return result, ErrUnsafe
		}
		gid, ok := numericID(f[3])
		if !ok {
			return result, ErrUnsafe
		}
		if f[0] != name {
			if uid == c.ReaderUID || gid == c.FrameGID {
				return result, ErrUnsafe
			}
			continue
		}
		if _, seen := result.users[name]; seen || uid != c.ReaderUID || gid != c.FrameGID || f[1] != "x" || f[5] != "/nonexistent" || f[6] != "/usr/sbin/nologin" {
			return result, ErrUnsafe
		}
		result.users[name] = line
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(groups), "\n"), "\n") {
		f := strings.Split(line, ":")
		if len(f) != 4 || len(line) > 4096 {
			return result, ErrUnsafe
		}
		gid, ok := numericID(f[2])
		if !ok {
			return result, ErrUnsafe
		}
		for _, member := range strings.Split(f[3], ",") {
			if member == name {
				return result, ErrUnsafe
			}
		}
		if f[0] != group {
			if gid == c.FrameGID {
				return result, ErrUnsafe
			}
			continue
		}
		if _, seen := result.groups[group]; seen || gid != c.FrameGID || f[1] != "x" || f[3] != "" {
			return result, ErrUnsafe
		}
		result.groups[group] = line
	}
	if len(result.users) != 1 || len(result.groups) != 1 {
		return result, ErrUnsafe
	}
	return result, nil
}

// The O_PATH handle pins the admitted inode throughout a start transaction;
// comparing only dev/ino after closing it could allow inode reuse on replacement.
type dockerSocketIdentity struct {
	dev, ino uint64
	gid      uint32
	handle   *os.File
}

func (s dockerSocketIdentity) Close() error {
	if s.handle == nil {
		return ErrUnsafe
	}
	return s.handle.Close()
}

func validDockerSocket(s unix.Stat_t, expectedGID uint32) bool {
	return expectedGID != 0 && expectedGID != ^uint32(0) && s.Mode == unix.S_IFSOCK|0660 && s.Uid == 0 && s.Gid == expectedGID && s.Nlink == 1
}

// Discovery is only for fresh explicit enrollment approval. Start must use the
// persisted nonzero GID and refuses a changed socket until explicit Stop/Start.
func discoverDockerSocket() (dockerSocketIdentity, error) { return openFixedDockerSocket(0, true) }
func captureDockerSocket(expectedGID uint32) (dockerSocketIdentity, error) {
	return openFixedDockerSocket(expectedGID, false)
}
func openFixedDockerSocket(expectedGID uint32, discover bool) (dockerSocketIdentity, error) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return dockerSocketIdentity{}, ErrUnsafe
	}
	parent, err := rootDirectory("/run")
	if err != nil {
		return dockerSocketIdentity{}, ErrUnsafe
	}
	defer parent.Close()
	return openDockerSocketAt(parent, expectedGID, discover)
}

func captureDockerSocketAt(parent *os.File, expectedGID uint32) (dockerSocketIdentity, error) {
	return openDockerSocketAt(parent, expectedGID, false)
}
func openDockerSocketAt(parent *os.File, expectedGID uint32, discover bool) (dockerSocketIdentity, error) {
	if parent == nil || !discover && (expectedGID == 0 || expectedGID == ^uint32(0)) {
		return dockerSocketIdentity{}, ErrUnsafe
	}
	fd, err := unix.Openat(int(parent.Fd()), "docker.sock", unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return dockerSocketIdentity{}, ErrUnsafe
	}
	f := os.NewFile(uintptr(fd), "approved-docker-socket")
	refuse := func() (dockerSocketIdentity, error) { f.Close(); return dockerSocketIdentity{}, ErrUnsafe }
	var s, current unix.Stat_t
	if unix.Fstat(fd, &s) != nil {
		return refuse()
	}
	if discover {
		expectedGID = s.Gid
	}
	if !validDockerSocket(s, expectedGID) {
		return refuse()
	}
	// ACL lookup stays anchored to the already validated /run directory. Check
	// entry identity again afterward; never query a caller-provided socket path.
	path := "/proc/self/fd/" + strconv.Itoa(int(parent.Fd())) + "/docker.sock"
	for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
		n, e := unix.Lgetxattr(path, name, nil)
		if e != unix.ENODATA && e != unix.ENOTSUP && (e != nil || n != 0) {
			return refuse()
		}
	}
	if unix.Fstatat(int(parent.Fd()), "docker.sock", &current, unix.AT_SYMLINK_NOFOLLOW) != nil || !validDockerSocket(current, expectedGID) || current.Dev != s.Dev || current.Ino != s.Ino {
		return refuse()
	}
	return dockerSocketIdentity{dev: uint64(s.Dev), ino: s.Ino, gid: s.Gid, handle: f}, nil
}

func sameDockerSocket(want dockerSocketIdentity) bool {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return false
	}
	parent, err := rootDirectory("/run")
	if err != nil {
		return false
	}
	defer parent.Close()
	return sameDockerSocketAt(parent, want)
}
func sameDockerSocketAt(parent *os.File, want dockerSocketIdentity) bool {
	if want.handle == nil {
		return false
	}
	var pinned unix.Stat_t
	if unix.Fstat(int(want.handle.Fd()), &pinned) != nil || !validDockerSocket(pinned, want.gid) || uint64(pinned.Dev) != want.dev || pinned.Ino != want.ino {
		return false
	}
	current, err := captureDockerSocketAt(parent, want.gid)
	if err != nil {
		return false
	}
	defer current.Close()
	return current.dev == want.dev && current.ino == want.ino
}
