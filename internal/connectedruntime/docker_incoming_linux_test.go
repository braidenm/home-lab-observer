//go:build linux

package connectedruntime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedunits"
	"golang.org/x/sys/unix"
)

func TestDockerIncomingRealEmptyReadRewindsAndRemainsBounded(t *testing.T) {
	directory := t.TempDir()
	fd, err := unix.Open(directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal("cannot open private fixture")
	}
	defer unix.Close(fd)
	if dockerIncomingEmpty(fd) != nil || dockerIncomingEmpty(fd) != nil || dockerIncomingNoACL(fd) != nil {
		t.Fatal("empty directory refused")
	}
	if os.WriteFile(filepath.Join(directory, "synthetic"), nil, 0600) != nil {
		t.Fatal("cannot populate private fixture")
	}
	if dockerIncomingEmpty(fd) != ErrUnsafe || dockerIncomingEmpty(fd) != ErrUnsafe {
		t.Fatal("repeated read hid later population")
	}
	if unix.Fstat(fd, &unix.Stat_t{}) != nil {
		t.Fatal("adapter closed caller's pinned descriptor")
	}
	if dockerIncomingEmpty(-1) != ErrUnsafe || dockerIncomingNoACL(-1) != ErrUnsafe {
		t.Fatal("invalid descriptor accepted")
	}
}

func TestDockerIncomingRealWalkRejectsLinksAndRetainedPathReplacement(t *testing.T) {
	for _, change := range []string{"", "parent-link", "source-link", "parent-replaced", "source-replaced", "nonempty"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			parent := filepath.Join(root, "run", "systemd", "propagate")
			source := filepath.Join(parent, connectedunits.RichProcessesDockerReaderUnit)
			if os.MkdirAll(parent, 0700) != nil || os.Mkdir(source, 0600) != nil {
				t.Fatal("cannot create private fixture")
			}
			defer os.Chmod(source, 0700) // Only the owned test fixture, for t.TempDir cleanup.
			if change == "parent-link" {
				if os.Rename(parent, parent+"-held") != nil || os.Symlink(parent+"-held", parent) != nil {
					t.Fatal("cannot create link fixture")
				}
			}
			if change == "source-link" {
				if os.Rename(source, source+"-held") != nil || os.Symlink(source+"-held", source) != nil {
					t.Fatal("cannot create link fixture")
				}
			}
			ops := dockerIncomingSystemOperations()
			open, stat, child := ops.open, ops.stat, ops.child
			ops.open = func(fd int, name string, flags int) (int, error) {
				if fd == unix.AT_FDCWD && name == "/" {
					return open(fd, root, flags)
				}
				return open(fd, name, flags)
			}
			// These are ordinary-user syscall fixtures, not privileged host proof.
			// Normalize only ownership so real no-follow/FD/empty adapters run on
			// caller-owned temporary files; owner refusal is tested independently.
			ops.stat = func(fd int) (unix.Stat_t, error) { s, err := stat(fd); s.Uid, s.Gid = 0, 0; return s, err }
			ops.child = func(fd int, name string) (unix.Stat_t, error) {
				if fd == unix.AT_FDCWD && name == "/" {
					name = root
				}
				s, err := child(fd, name)
				s.Uid, s.Gid = 0, 0
				return s, err
			}
			pin, err := pinDockerReaderIncomingWith(ops)
			if change == "parent-link" || change == "source-link" {
				if err == nil {
					pin.close()
					t.Fatal("symlink source walk accepted")
				}
				return
			}
			if err != nil {
				t.Fatal("real descriptor walk refused")
			}
			fds := append([]int(nil), pin.fds...)
			switch change {
			case "parent-replaced":
				if os.Rename(parent, parent+"-held") != nil || os.Mkdir(parent, 0700) != nil {
					t.Fatal("cannot replace parent fixture")
				}
			case "source-replaced":
				if os.Rename(source, source+"-held") != nil || os.Mkdir(source, 0600) != nil {
					t.Fatal("cannot replace source fixture")
				}
			case "nonempty":
				if os.Chmod(source, 0700) != nil || os.WriteFile(filepath.Join(source, "synthetic"), nil, 0600) != nil || os.Chmod(source, 0600) != nil {
					t.Fatal("cannot populate source fixture")
				}
			}
			if (pin.check() == nil) != (change == "") {
				t.Fatal("retained descriptor failed to detect changed path")
			}
			if pin.close() != nil {
				t.Fatal("descriptor close failed")
			}
			for _, fd := range fds {
				if unix.Fstat(fd, &unix.Stat_t{}) != unix.EBADF {
					t.Fatal("real source descriptor leaked")
				}
			}
		})
	}
}

type incomingFixture struct {
	t              *testing.T
	stats          map[int]unix.Stat_t
	opened, closed []int
	failure        string
	checks         int
}

func newIncomingFixture(t *testing.T, failure string) *incomingFixture {
	f := &incomingFixture{t: t, failure: failure, stats: map[int]unix.Stat_t{}}
	for fd := 10; fd < 15; fd++ {
		f.stats[fd] = unix.Stat_t{Dev: 2, Ino: uint64(fd), Mode: unix.S_IFDIR | 0755}
	}
	s := f.stats[14]
	s.Mode = unix.S_IFDIR | 0600
	f.stats[14] = s
	return f
}

func (f *incomingFixture) operations() dockerIncomingOperations {
	parts := []string{"/", "run", "systemd", "propagate", connectedunits.RichProcessesDockerReaderUnit}
	return dockerIncomingOperations{
		open: func(parent int, name string, flags int) (int, error) {
			i := len(f.opened)
			if i >= len(parts) || name != parts[i] || (i == 0 && parent != unix.AT_FDCWD) || (i > 0 && parent != 9+i) {
				f.t.Fatal("unexpected source traversal")
			}
			want := unix.O_PATH | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC
			if i == 4 {
				want = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
			}
			if flags != want {
				f.t.Fatal("unsafe directory open flags")
			}
			if f.failure == "open" && i == 2 {
				return -1, unix.ELOOP
			}
			fd := 10 + i
			f.opened = append(f.opened, fd)
			return fd, nil
		},
		stat: func(fd int) (unix.Stat_t, error) {
			s := f.stats[fd]
			if fd == 12 {
				switch f.failure {
				case "parent-owner":
					s.Uid = 1
				case "parent-group":
					s.Gid = 1
				case "parent-writable":
					s.Mode |= 0020
				case "parent-link":
					s.Mode = unix.S_IFLNK | 0777
				case "stat":
					return s, unix.EIO
				}
			}
			if fd == 14 {
				switch f.failure {
				case "source-mode":
					s.Mode = unix.S_IFDIR | 0755
				case "source-owner":
					s.Uid = 1
				case "source-group":
					s.Gid = 1
				case "source-type":
					s.Mode = unix.S_IFREG | 0600
				case "source-zero-device":
					s.Dev = 0
				case "source-zero-inode":
					s.Ino = 0
				}
			}
			return s, nil
		},
		child: func(parent int, name string) (unix.Stat_t, error) {
			fd := parent + 1
			if parent == unix.AT_FDCWD && name == "/" {
				fd = 10
			} else if parent < 10 || parent > 13 || name != parts[parent-9] {
				f.t.Fatal("unexpected path identity check")
			}
			s := f.stats[fd]
			if (f.failure == "parent-replaced" && fd == 12) || (f.failure == "source-replaced" && fd == 14) {
				s.Ino++
			}
			if f.failure == "path-error" {
				return s, unix.EIO
			}
			return s, nil
		},
		acl: func(fd int) error {
			if fd != 14 {
				f.t.Fatal("unexpected ACL descriptor")
			}
			if f.failure == "acl" {
				return ErrUnsafe
			}
			return nil
		},
		filesystem: func(fd int) (int64, error) {
			if fd != 14 {
				f.t.Fatal("unexpected filesystem descriptor")
			}
			switch f.failure {
			case "procfs":
				return unix.PROC_SUPER_MAGIC, nil
			case "sysfs":
				return unix.SYSFS_MAGIC, nil
			case "filesystem-error":
				return 0, unix.EIO
			}
			return unix.EXT4_SUPER_MAGIC, nil
		},
		empty: func(fd int) error {
			if fd != 14 {
				f.t.Fatal("unexpected listing descriptor")
			}
			f.checks++
			if f.failure == "nonempty" || (f.failure == "late-entry" && f.checks > 1) {
				return ErrUnsafe
			}
			return nil
		},
		close: func(fd int) error {
			for _, closed := range f.closed {
				if fd == closed {
					f.t.Fatal("descriptor closed twice")
				}
			}
			f.closed = append(f.closed, fd)
			if f.failure == "close" {
				return unix.EIO
			}
			return nil
		},
	}
}

func TestDockerIncomingFixedSourcePinsAndClosesEveryDescriptor(t *testing.T) {
	for _, failure := range []string{"", "open", "stat", "parent-owner", "parent-group", "parent-writable", "parent-link", "source-mode", "source-owner", "source-group", "source-type", "source-zero-device", "source-zero-inode", "parent-replaced", "source-replaced", "path-error", "acl", "procfs", "sysfs", "filesystem-error", "nonempty", "late-entry", "close"} {
		t.Run(failure, func(t *testing.T) {
			f := newIncomingFixture(t, failure)
			p, err := pinDockerReaderIncomingWith(f.operations())
			if failure == "" || failure == "late-entry" || failure == "close" {
				if err != nil {
					t.Fatal("valid source refused")
				}
				if p.identity() != (DockerReaderNamespaceIdentity{Dev: 2, Ino: 14}) {
					t.Fatal("source identity lost")
				}
				err = p.check()
				if failure == "late-entry" && err == nil {
					t.Fatal("source population after first audit accepted")
				}
				if failure == "" && err != nil {
					t.Fatal("stable source refused")
				}
				if closeErr := p.close(); (closeErr == nil) != (failure != "close") {
					t.Fatal("close uncertainty hidden")
				}
			} else if err == nil {
				p.close()
				t.Fatal("unsafe source accepted")
			}
			if len(f.opened) != len(f.closed) {
				t.Fatal("source descriptor leak")
			}
		})
	}
}

func TestDockerIncomingNamespaceRequiresExactSource(t *testing.T) {
	source := unix.Stat_t{Dev: 2, Ino: 14, Mode: unix.S_IFDIR | 0600}
	for _, change := range []string{"", "inode", "device", "mode", "owner", "group", "error"} {
		t.Run(change, func(t *testing.T) {
			err := checkDockerReaderIncomingBinding(source, func(name string) (unix.Stat_t, error) {
				if name != "run/systemd/incoming" {
					t.Fatal("unfixed namespace path")
				}
				s := source
				switch change {
				case "inode":
					s.Ino++
				case "device":
					s.Dev++
				case "mode":
					s.Mode = unix.S_IFDIR | 0755
				case "owner":
					s.Uid++
				case "group":
					s.Gid++
				case "error":
					return s, unix.EIO
				}
				return s, nil
			})
			if (err == nil) != (change == "") {
				t.Fatal("source binding mismatch")
			}
		})
	}
}
