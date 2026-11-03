//go:build linux

package actionprofile

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

// ErrBrokerAbsent is only an unavailable fixed endpoint, never a profile or
// peer refusal. Only that distinction permits the worker's paced reconnect.
var ErrBrokerAbsent = errors.New("action_broker_unavailable")

type endpointDirectory struct {
	name           string
	uid, gid, mode uint32
}

// endpointPin owns only read descriptors. It never creates, adopts, unlinks or
// repairs a listener. The production plan and absolute dial path are fixed.
type endpointPin struct {
	fds         []int
	directories []endpointDirectory
	witnesses   []unix.Stat_t
	socket      unix.Stat_t
	uid, gid    uint32
	name        string
	mode        uint32
}

func (p *endpointPin) close() {
	for _, fd := range p.fds {
		unix.Close(fd)
	}
	p.fds = nil
}

func stableDirectory(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Uid == b.Uid && a.Gid == b.Gid
}

// Preserve the existing fixed-dial filesystem policy; unsupported POSIX ACLs
// do not become an extra ACL grant. The role/host-namespace checks remain
// independently required and keep their existing stricter provenance policy.
func dialNoACL(fd int) bool {
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

func directoryMatches(fd int, part endpointDirectory) (unix.Stat_t, error) {
	var s unix.Stat_t
	if unix.Fstat(fd, &s) != nil || s.Mode&unix.S_IFMT != unix.S_IFDIR || s.Uid != part.uid || s.Gid != part.gid || s.Mode&07022 != 0 || part.mode != 0 && s.Mode&07777 != part.mode || !dialNoACL(fd) {
		return s, ErrProfile
	}
	return s, nil
}

// The private root-descriptor seam permits unprivileged temporary fixtures;
// production opens only / and supplies its fixed three-directory plan.
func pinEndpointAt(ctx context.Context, root int, parts []endpointDirectory, socketUID, socketGID uint32, admin bool) (_ *endpointPin, result error) {
	p := &endpointPin{fds: []int{root}, directories: parts, uid: socketUID, gid: socketGID, name: "worker.sock", mode: 0660}
	if admin {
		p.name, p.mode = "recovery.sock", 0600
	}
	defer func() {
		if result != nil {
			p.close()
		}
	}()
	if ctx == nil || ctx.Err() != nil || len(parts) != 3 {
		return nil, ErrProfile
	}
	var rootStat unix.Stat_t
	if unix.Fstat(root, &rootStat) != nil || rootStat.Mode&unix.S_IFMT != unix.S_IFDIR || rootStat.Mode&07022 != 0 || !dialNoACL(root) {
		return nil, ErrProfile
	}
	p.witnesses = append(p.witnesses, rootStat)
	for _, part := range parts {
		if part.name == "" || part.name == "." || part.name == ".." {
			return nil, ErrProfile
		}
		for _, c := range part.name {
			if c == '/' || c == 0 {
				return nil, ErrProfile
			}
		}
		fd, err := unix.Openat(p.fds[len(p.fds)-1], part.name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return nil, ErrProfile
		}
		p.fds = append(p.fds, fd)
		stat, err := directoryMatches(fd, part)
		if err != nil {
			return nil, ErrProfile
		}
		p.witnesses = append(p.witnesses, stat)
	}
	if err := unix.Fstatat(p.fds[3], p.name, &p.socket, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil, ErrBrokerAbsent
		}
		return nil, ErrProfile
	}
	if p.check(ctx) != nil {
		return nil, ErrProfile
	}
	return p, nil
}

func (p *endpointPin) check(ctx context.Context) error {
	if p == nil || len(p.fds) != 4 || len(p.witnesses) != 4 || ctx == nil || ctx.Err() != nil {
		return ErrProfile
	}
	var root unix.Stat_t
	if unix.Fstat(p.fds[0], &root) != nil || !stableDirectory(root, p.witnesses[0]) || !dialNoACL(p.fds[0]) {
		return ErrProfile
	}
	for index, part := range p.directories {
		current, err := directoryMatches(p.fds[index+1], part)
		var named unix.Stat_t
		if err != nil || !stableDirectory(current, p.witnesses[index+1]) || unix.Fstatat(p.fds[index], part.name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || !stableDirectory(named, current) {
			return ErrProfile
		}
	}
	var current unix.Stat_t
	if unix.Fstatat(p.fds[3], p.name, &current, unix.AT_SYMLINK_NOFOLLOW) != nil || !stable(p.socket, current) || current.Mode != unix.S_IFSOCK|p.mode || current.Uid != p.uid || current.Gid != p.gid || current.Nlink != 1 {
		return ErrProfile
	}
	// Unix socket nodes cannot be opened for ordinary fgetxattr. Anchor the
	// no-follow xattr read to the retained leaf fd, then recheck that exact node.
	anchored := "/proc/self/fd/" + strconv.Itoa(p.fds[3]) + "/" + p.name
	if _, err := unix.Lgetxattr(anchored, "system.posix_acl_access", nil); !errors.Is(err, unix.ENODATA) {
		return ErrProfile
	}
	var after unix.Stat_t
	if unix.Fstatat(p.fds[3], p.name, &after, unix.AT_SYMLINK_NOFOLLOW) != nil || !stable(current, after) || ctx.Err() != nil {
		return ErrProfile
	}
	return nil
}

// DialWorkerBroker admits only the actual fixed worker role. Expected is a
// drift witness compared with freshly resolved roles, never an identity grant.
func DialWorkerBroker(ctx context.Context, expected Identity) (*net.UnixConn, error) {
	return dialBrokerRole(ctx, expected, false)
}

// DialAdminBroker admits only host-root administration on the distinct private
// admin path. It cannot fall back to the worker socket or another transport.
func DialAdminBroker(ctx context.Context, expected Identity) (*net.UnixConn, error) {
	return dialBrokerRole(ctx, expected, true)
}

func dialIdentity(ctx context.Context, admin bool) (Identity, error) {
	if ctx == nil || ctx.Err() != nil {
		return Identity{}, ErrProfile
	}
	var i Identity
	var err error
	if admin {
		i, err = VerifyAdmin()
	} else {
		i, err = VerifyWorker()
	}
	if err != nil || ctx.Err() != nil {
		return Identity{}, ErrProfile
	}
	return i, nil
}

func dialBrokerRole(ctx context.Context, expected Identity, admin bool) (*net.UnixConn, error) {
	i, err := dialIdentity(ctx, admin)
	if err != nil || i != expected {
		return nil, ErrProfile
	}
	root, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrProfile
	}
	var s unix.Stat_t
	if unix.Fstat(root, &s) != nil || s.Uid != 0 || s.Gid != 0 || s.Mode != unix.S_IFDIR|0755 {
		unix.Close(root)
		return nil, ErrProfile
	}
	leaf, group, mode, path := "worker", i.IPCGID, uint32(0750), WorkerSocketPath
	if admin {
		leaf, group, mode, path = "admin", i.BrokerGID, 0700, AdminSocketPath
	}
	p, err := pinEndpointAt(ctx, root, []endpointDirectory{{"run", 0, 0, 0}, {"home-lab-observer-actions", 0, 0, 0755}, {leaf, i.BrokerUID, group, mode}}, i.BrokerUID, group, admin)
	if err != nil {
		return nil, err
	}
	defer p.close()
	return connectPinned(ctx, p, path, func(c *net.UnixConn) error {
		current, err := dialIdentity(ctx, admin)
		if err != nil || current != i {
			return ErrProfile
		}
		raw, err := c.SyscallConn()
		if err != nil {
			return ErrProfile
		}
		return VerifyBrokerPeer(raw, i)
	})
}

// Only dialBrokerRole calls this seam outside tests: its pathname and peer
// policy are code-owned. Path and kernel peer must match before protocol bytes.
func connectPinned(ctx context.Context, p *endpointPin, path string, peer func(*net.UnixConn) error) (*net.UnixConn, error) {
	if p.check(ctx) != nil || peer == nil {
		return nil, ErrProfile
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, err := (&net.Dialer{}).DialContext(bounded, "unix", path)
	if err != nil {
		if p.check(ctx) != nil {
			return nil, ErrProfile
		}
		if errors.Is(err, unix.ECONNREFUSED) || errors.Is(err, unix.ENOENT) || errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrBrokerAbsent
		}
		return nil, ErrProfile
	}
	u, ok := c.(*net.UnixConn)
	if !ok {
		c.Close()
		return nil, ErrProfile
	}
	if p.check(ctx) != nil || peer(u) != nil || p.check(ctx) != nil {
		u.Close()
		return nil, ErrProfile
	}
	return u, nil
}
