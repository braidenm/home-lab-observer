//go:build linux

package connectedruntime

import (
	"io"
	"os"

	"github.com/braidenm/home-lab-observer/internal/connectedunits"
	"golang.org/x/sys/unix"
)

// DockerReaderNamespaceIdentity binds the two paused reader audits to the same
// native propagation source. It is not an authority or a caller-selected path.
type DockerReaderNamespaceIdentity struct{ Dev, Ino uint64 }

var dockerIncomingParts = [...]string{"/", "run", "systemd", "propagate", connectedunits.RichProcessesDockerReaderUnit}

// Fixed-path syscall seams only; no production caller can supply a source path.
type dockerIncomingOperations struct {
	open       func(int, string, int) (int, error)
	stat       func(int) (unix.Stat_t, error)
	child      func(int, string) (unix.Stat_t, error)
	acl        func(int) error
	filesystem func(int) (int64, error)
	empty      func(int) error
	close      func(int) error
}

type dockerReaderIncoming struct {
	ops   dockerIncomingOperations
	fds   []int
	stats []unix.Stat_t
}

func pinDockerReaderIncoming() (*dockerReaderIncoming, error) {
	return pinDockerReaderIncomingWith(dockerIncomingSystemOperations())
}

func dockerIncomingSystemOperations() dockerIncomingOperations {
	return dockerIncomingOperations{
		open: func(parent int, name string, flags int) (int, error) { return unix.Openat(parent, name, flags, 0) },
		stat: func(fd int) (s unix.Stat_t, err error) { err = unix.Fstat(fd, &s); return },
		child: func(parent int, name string) (s unix.Stat_t, err error) {
			err = unix.Fstatat(parent, name, &s, unix.AT_SYMLINK_NOFOLLOW)
			return
		},
		acl: dockerIncomingNoACL,
		filesystem: func(fd int) (int64, error) {
			var s unix.Statfs_t
			if unix.Fstatfs(fd, &s) != nil {
				return 0, ErrUnsafe
			}
			return s.Type, nil
		},
		empty: dockerIncomingEmpty,
		close: unix.Close,
	}
}

func pinDockerReaderIncomingWith(ops dockerIncomingOperations) (*dockerReaderIncoming, error) {
	p := &dockerReaderIncoming{ops: ops}
	for i, part := range dockerIncomingParts {
		parent, flags := unix.AT_FDCWD, unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC
		if i > 0 {
			parent = p.fds[i-1]
		}
		if i == len(dockerIncomingParts)-1 {
			flags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
		}
		fd, err := ops.open(parent, part, flags)
		if err != nil {
			p.close()
			return nil, ErrUnsafe
		}
		p.fds = append(p.fds, fd)
		s, err := ops.stat(fd)
		if err != nil || !safeDockerIncomingMetadata(s, i == len(dockerIncomingParts)-1) {
			p.close()
			return nil, ErrUnsafe
		}
		p.stats = append(p.stats, s)
	}
	if p.check() != nil {
		p.close()
		return nil, ErrUnsafe
	}
	return p, nil
}

func safeDockerIncomingMetadata(s unix.Stat_t, source bool) bool {
	if s.Mode&unix.S_IFMT != unix.S_IFDIR || s.Uid != 0 || s.Gid != 0 {
		return false
	}
	if source {
		return s.Mode == unix.S_IFDIR|0600 && s.Dev != 0 && s.Ino != 0
	}
	// Parent modes are manager/distribution details; trust ownership and deny
	// group/world writes instead of inventing an exact upstream mode contract.
	return s.Mode&0022 == 0
}

func (p *dockerReaderIncoming) check() error {
	if len(p.fds) != len(dockerIncomingParts) || len(p.stats) != len(p.fds) {
		return ErrUnsafe
	}
	for i, fd := range p.fds {
		s, err := p.ops.stat(fd)
		if err != nil || !sameViewObject(s, p.stats[i]) || !safeDockerIncomingMetadata(s, i == len(p.fds)-1) {
			return ErrUnsafe
		}
		parent := unix.AT_FDCWD
		if i > 0 {
			parent = p.fds[i-1]
		}
		path, err := p.ops.child(parent, dockerIncomingParts[i])
		if err != nil || !sameViewObject(path, s) || path.Mode != s.Mode || path.Uid != s.Uid || path.Gid != s.Gid {
			return ErrUnsafe
		}
	}
	fd := p.fds[len(p.fds)-1]
	filesystem, err := p.ops.filesystem(fd)
	if err != nil || filesystem == unix.PROC_SUPER_MAGIC || filesystem == unix.SYSFS_MAGIC || p.ops.acl(fd) != nil || p.ops.empty(fd) != nil {
		return ErrUnsafe
	}
	return nil
}

func (p *dockerReaderIncoming) identity() DockerReaderNamespaceIdentity {
	s := p.stats[len(p.stats)-1]
	return DockerReaderNamespaceIdentity{uint64(s.Dev), s.Ino}
}

func (p *dockerReaderIncoming) close() error {
	var result error
	for i := len(p.fds) - 1; i >= 0; i-- {
		if p.ops.close(p.fds[i]) != nil {
			result = ErrUnsafe
		}
	}
	p.fds = nil
	return result
}

func dockerIncomingNoACL(fd int) error {
	for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
		_, err := unix.Fgetxattr(fd, name, nil)
		if err != unix.ENODATA && err != unix.ENOTSUP {
			return ErrUnsafe
		}
	}
	return nil
}

func dockerIncomingEmpty(fd int) (result error) {
	// Dup gives os.File sole ownership of its descriptor; reset the shared
	// directory offset for every audit so a later addition cannot hide at EOF.
	dup, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return ErrUnsafe
	}
	f := os.NewFile(uintptr(dup), "reader-incoming")
	defer func() {
		if f.Close() != nil {
			result = ErrUnsafe
		}
	}()
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return ErrUnsafe
	}
	entries, err := f.Readdirnames(1)
	if len(entries) != 0 || err != io.EOF {
		return ErrUnsafe
	}
	return nil
}

func checkDockerReaderIncomingBinding(source unix.Stat_t, actual func(string) (unix.Stat_t, error)) error {
	s, err := actual("run/systemd/incoming")
	if err != nil || !sameViewObject(s, source) || !safeDockerIncomingMetadata(s, true) {
		return ErrUnsafe
	}
	return nil
}
