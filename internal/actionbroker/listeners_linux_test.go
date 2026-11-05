//go:build linux

package actionbroker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/actionprofile"
	"golang.org/x/sys/unix"
)

// Synthetic ownership only: this proves private Linux filesystem/socket behavior,
// not installed roles, host-namespace provenance or actual systemd confinement.
func listenerFixture(t *testing.T, kind listenerKind) (*listenerParent, string) {
	t.Helper()
	root, err := os.MkdirTemp("", "hbl-") // Keep Unix pathname below108 bytes.
	if err != nil {
		t.Fatal("private listener fixture")
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	uid, gid := uint32(os.Geteuid()), uint32(os.Getegid())
	i := actionprofile.Identity{BrokerUID: uid, BrokerGID: gid, IPCGID: gid}
	leaf := "worker"
	mode := os.FileMode(0750)
	for _, name := range []string{root, filepath.Join(root, "run"), filepath.Join(root, "run/home-lab-observer-actions")} {
		if name != root {
			if os.Mkdir(name, 0755) != nil {
				t.Fatal("fixture ancestor")
			}
		}
		if os.Chmod(name, 0755) != nil {
			t.Fatal("fixture mode")
		}
	}
	if os.Mkdir(filepath.Join(root, "run/home-lab-observer-actions", leaf), mode) != nil {
		t.Fatal("fixture leaf")
	}
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal("private root FD")
	}
	p, err := pinListenerParentAt(context.Background(), fd, root, kind, uid, gid, i)
	if err != nil {
		t.Fatal("pin fixed fixture plan")
	}
	t.Cleanup(func() { p.close() })
	return p, p.path
}

func TestBrokerListenerUsesActualInodePermissionsAndOwnedCleanup(t *testing.T) {
	for _, kind := range []listenerKind{workerListener} {
		t.Run([]string{"worker"}[int(kind)-1], func(t *testing.T) {
			p, path := listenerFixture(t, kind)
			e, err := p.bind(context.Background())
			if err != nil {
				t.Fatal("actual supported socket metadata syscalls")
			}
			if e.current(context.Background()) != nil {
				t.Fatal("captured listener witness")
			}
			var stat unix.Stat_t
			if unix.Lstat(path, &stat) != nil || stat.Mode != unix.S_IFSOCK|p.mode || stat.Uid != p.owner || stat.Gid != p.group {
				t.Fatal("wrong listener ownership/mode")
			}
			if e.close() != nil {
				t.Fatal("owned close")
			}
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				t.Fatal("owned endpoint retained")
			}
		})
	}
}

func TestBrokerListenerNeverAdoptsOrUnlinksExistingOrReplacedEndpoint(t *testing.T) {
	for _, mode := range []string{"regular", "dangling-link", "replacement"} {
		t.Run(mode, func(t *testing.T) {
			p, path := listenerFixture(t, workerListener)
			if mode == "replacement" {
				e, err := p.bind(context.Background())
				if err != nil {
					t.Fatal("owned listener")
				}
				if os.Rename(path, path+".retained") != nil || os.WriteFile(path, []byte("foreign"), 0600) != nil {
					t.Fatal("private replacement")
				}
				if e.close() == nil {
					t.Fatal("replacement cleanup claimed success")
				}
			} else {
				if mode == "regular" {
					if os.WriteFile(path, []byte("foreign"), 0600) != nil {
						t.Fatal("foreign fixture")
					}
				} else if os.Symlink("absent", path) != nil {
					t.Fatal("private link")
				}
				if e, err := p.bind(context.Background()); err == nil || e != nil {
					t.Fatal("existing endpoint adopted")
				}
			}
			if mode != "dangling-link" {
				if raw, err := os.ReadFile(path); err != nil || string(raw) != "foreign" {
					t.Fatal("foreign endpoint altered")
				}
			} else if _, err := os.Lstat(path); err != nil {
				t.Fatal("foreign link removed")
			}
		})
	}
}

func TestBrokerListenerUnsupportedInodeCapabilityRefusesAndCleansOnlyOwnNode(t *testing.T) {
	p, path := listenerFixture(t, workerListener)
	called := false
	chmod := func(fd int, mode uint32) error { called = true; return unix.EOPNOTSUPP }
	if e, err := p.bindUsing(context.Background(), chmod); err == nil || e != nil || !called {
		t.Fatal("unsupported capability accepted")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("captured failed-start endpoint retained")
	}
	if p.current(context.Background()) != nil {
		t.Fatal("failed-start cleanup damaged parent")
	}
}

func TestBrokerListenerParentReplacementAndCancellationRefuseBeforeBind(t *testing.T) {
	for _, mode := range []string{"cancel", "replace-parent", "writable-ancestor", "parent-default-acl"} {
		t.Run(mode, func(t *testing.T) {
			p, path := listenerFixture(t, workerListener)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "cancel":
				cancel()
			case "replace-parent":
				dir := filepath.Dir(path)
				if os.Rename(dir, dir+".retained") != nil || os.Mkdir(dir, 0750) != nil {
					t.Fatal("private parent replacement")
				}
			case "writable-ancestor":
				if os.Chmod(filepath.Dir(filepath.Dir(path)), 0777) != nil {
					t.Fatal("private ancestor drift")
				}
			case "parent-default-acl":
				// Named foreign UID with read access, mode still0750. This proves
				// the ACL refusal independently of a changed permission-bit mask.
				acl := []byte{2, 0, 0, 0, 1, 0, 7, 0, 255, 255, 255, 255, 2, 0, 4, 0, 0, 240, 0, 0, 4, 0, 5, 0, 255, 255, 255, 255, 16, 0, 5, 0, 255, 255, 255, 255, 32, 0, 0, 0, 255, 255, 255, 255}
				if unix.Setxattr(filepath.Dir(path), "system.posix_acl_default", acl, 0) != nil {
					t.Fatal("private default ACL fixture")
				}
				var stat unix.Stat_t
				if unix.Lstat(filepath.Dir(path), &stat) != nil || stat.Mode != unix.S_IFDIR|0750 {
					t.Fatal("ACL fixture changed permission bits")
				}
			}
			if e, err := p.bind(ctx); err == nil || e != nil {
				t.Fatal("unsafe parent admitted")
			}
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				t.Fatal("unsafe preflight created endpoint")
			}
		})
	}
}
