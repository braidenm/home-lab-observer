//go:build linux

package actionbroker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"

	"github.com/braidenm/home-lab-observer/internal/actionprofile"
	"golang.org/x/sys/unix"
)

type listenerKind uint8

const (
	workerListener listenerKind = iota + 1
)

type listenerParent struct {
	fds                []int
	witnesses          []unix.Stat_t
	names              []string
	root, path, socket string
	owner, group, mode uint32
}

// Directory timestamps are not immutable security identity: /run changes for
// unrelated services and creating our own socket changes its parent. Require
// exact device/inode/type/owner/mode and ACLs instead. Socket witnesses are full.
func listenerDirectorySame(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Uid == b.Uid && a.Gid == b.Gid
}

func (p *listenerParent) close() error {
	if p == nil {
		return nil
	}
	fds := p.fds
	p.fds = nil
	failed := false
	for n := len(fds) - 1; n >= 0; n-- {
		if unix.Close(fds[n]) != nil {
			failed = true
		}
	}
	if failed {
		return ErrRecoveryRequired
	}
	return nil
}

// Only production caller supplies root=/ and root UID/GID0. The private FD/root
// seam allows ordinary owned-temp fixtures; no caller paths/role flags escape.
func pinListenerParentAt(ctx context.Context, rootFD int, root string, kind listenerKind, rootUID, rootGID uint32, i actionprofile.Identity) (_ *listenerParent, result error) {
	p := &listenerParent{fds: []int{rootFD}, root: root, owner: i.BrokerUID, group: i.IPCGID, mode: 0660, socket: "worker.sock"}
	defer func() {
		if result != nil {
			p.close()
		}
	}()
	leaf := "worker"
	hostPath := actionprofile.WorkerSocketPath
	leafMode, leafGroup := uint32(0750), i.IPCGID
	if kind != workerListener {
		return nil, ErrRecoveryRequired
	}
	p.names = []string{"run", "home-lab-observer-actions", leaf}
	p.path = filepath.Join(root, strings.TrimPrefix(hostPath, "/"))
	var stat unix.Stat_t
	if ctx == nil || ctx.Err() != nil || unix.Fstat(rootFD, &stat) != nil || stat.Mode != unix.S_IFDIR|0755 || stat.Uid != rootUID || stat.Gid != rootGID || !brokerNoACL(rootFD) {
		return nil, ErrRecoveryRequired
	}
	p.witnesses = append(p.witnesses, stat)
	for index, name := range p.names {
		fd, err := unix.Openat(p.fds[len(p.fds)-1], name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return nil, ErrRecoveryRequired
		}
		p.fds = append(p.fds, fd)
		mode, uid, gid := uint32(0755), rootUID, rootGID
		if index == 2 {
			mode, uid, gid = leafMode, i.BrokerUID, leafGroup
		}
		if unix.Fstat(fd, &stat) != nil || stat.Mode != unix.S_IFDIR|mode || stat.Uid != uid || stat.Gid != gid || !brokerNoACL(fd) {
			return nil, ErrRecoveryRequired
		}
		p.witnesses = append(p.witnesses, stat)
	}
	if p.current(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	return p, nil
}

func (p *listenerParent) current(ctx context.Context) error {
	if p == nil || ctx == nil || ctx.Err() != nil || len(p.fds) != 4 || len(p.witnesses) != 4 {
		return ErrRecoveryRequired
	}
	for n, fd := range p.fds {
		var st unix.Stat_t
		if unix.Fstat(fd, &st) != nil || !listenerDirectorySame(st, p.witnesses[n]) || !brokerNoACL(fd) {
			return ErrRecoveryRequired
		}
		var err error
		if n == 0 {
			err = unix.Lstat(p.root, &st)
		} else {
			err = unix.Fstatat(p.fds[n-1], p.names[n-1], &st, unix.AT_SYMLINK_NOFOLLOW)
		}
		if err != nil || !listenerDirectorySame(st, p.witnesses[n]) {
			return ErrRecoveryRequired
		}
	}
	if ctx.Err() != nil {
		return ErrRecoveryRequired
	}
	return nil
}

type brokerEndpoint struct {
	listener  *net.UnixListener
	parent    *listenerParent
	nodeFD    int
	nodeOwned bool
	witness   unix.Stat_t
	closed    bool
	closeErr  error
}

func pinnedSocketMode(fd int, mode uint32) error {
	return unix.Fchmodat(fd, "", mode, unix.AT_EMPTY_PATH)
}
func (p *listenerParent) bind(ctx context.Context) (*brokerEndpoint, error) {
	return p.bindUsing(ctx, pinnedSocketMode)
}

// Private syscall-failure fixture seam. Production always selects the concrete
// pinned-inode syscall above and validates its actual resulting metadata.
func (p *listenerParent) bindUsing(ctx context.Context, chmod func(int, uint32) error) (_ *brokerEndpoint, result error) {
	if chmod == nil || p.current(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	var before unix.Stat_t
	if err := unix.Fstatat(p.fds[3], p.socket, &before, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
		return nil, ErrRecoveryRequired
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: p.path})
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	l.SetUnlinkOnClose(false) // Never let net.Close unlink a foreign replacement.
	e := &brokerEndpoint{listener: l, parent: p, nodeFD: -1}
	defer func() {
		if result != nil {
			e.close()
		}
	}()
	if p.current(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	e.nodeFD, err = unix.Openat(p.fds[3], p.socket, unix.O_PATH|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	e.nodeOwned = true
	if unix.Fstat(e.nodeFD, &e.witness) != nil || e.witness.Mode&unix.S_IFMT != unix.S_IFSOCK || e.witness.Uid != p.owner || e.witness.Nlink != 1 || e.matchNode(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	// Leave UID unchanged. The validated process is a member of this exact group.
	if unix.Fchownat(e.nodeFD, "", -1, int(p.group), unix.AT_EMPTY_PATH) != nil {
		return nil, ErrRecoveryRequired
	}
	if unix.Fstat(e.nodeFD, &e.witness) != nil || e.matchNode(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	if chmod(e.nodeFD, p.mode) != nil {
		return nil, ErrRecoveryRequired
	}
	if unix.Fstat(e.nodeFD, &e.witness) != nil || e.current(ctx) != nil {
		return nil, ErrRecoveryRequired
	}
	return e, nil
}

func (e *brokerEndpoint) matchNode(ctx context.Context) error {
	if e == nil || !e.nodeOwned || e.nodeFD < 0 || e.parent.current(ctx) != nil {
		return ErrRecoveryRequired
	}
	check := func() bool {
		var fd, path unix.Stat_t
		return unix.Fstat(e.nodeFD, &fd) == nil && unix.Fstatat(e.parent.fds[3], e.parent.socket, &path, unix.AT_SYMLINK_NOFOLLOW) == nil && brokerSame(fd, e.witness) && brokerSame(path, e.witness) && fd.Mode&unix.S_IFMT == unix.S_IFSOCK && fd.Uid == e.parent.owner && fd.Nlink == 1
	}
	if !check() {
		return ErrRecoveryRequired
	}
	// O_PATH cannot Fgetxattr. This fixed read-only alias is anchored to the
	// retained parent FD; it is NOT a chmod/procfs or arbitrary-path fallback.
	path := fmt.Sprintf("/proc/self/fd/%d/%s", e.parent.fds[3], e.parent.socket)
	for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
		n, err := unix.Lgetxattr(path, name, nil)
		if !errors.Is(err, unix.ENODATA) && !errors.Is(err, unix.ENOTSUP) && (err != nil || n != 0) {
			return ErrRecoveryRequired
		}
	}
	if !check() || e.parent.current(ctx) != nil {
		return ErrRecoveryRequired
	}
	return nil
}

func (e *brokerEndpoint) current(ctx context.Context) error {
	if e.matchNode(ctx) != nil || e.witness.Mode != unix.S_IFSOCK|e.parent.mode || e.witness.Gid != e.parent.group {
		return ErrRecoveryRequired
	}
	return nil
}

// Caller first closes admission and joins all handlers, then closes/unlinks only
// this captured node. A foreign entry is retained, never adopted or removed.
func (e *brokerEndpoint) close() error {
	if e == nil {
		return nil
	}
	if e.closed {
		return e.closeErr
	}
	e.closed = true
	failed := false
	if e.listener != nil {
		if err := e.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			failed = true
		}
	}
	if e.nodeOwned {
		if e.matchNode(context.Background()) != nil || unix.Unlinkat(e.parent.fds[3], e.parent.socket, 0) != nil {
			failed = true
		}
		if unix.Close(e.nodeFD) != nil {
			failed = true
		}
		e.nodeFD = -1
		e.nodeOwned = false
	} // No captured inode means no proof to delete a failed-start pathname.
	if failed {
		e.closeErr = ErrRecoveryRequired
	}
	return e.closeErr
}

func openListenerParent(ctx context.Context, kind listenerKind, i actionprofile.Identity) (*listenerParent, error) {
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	return pinListenerParentAt(ctx, fd, "/", kind, 0, 0, i)
}
