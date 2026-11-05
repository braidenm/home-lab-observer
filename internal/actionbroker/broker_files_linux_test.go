//go:build linux

package actionbroker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/actionprofile"
	"golang.org/x/sys/unix"
)

func brokerRecordFixture(t *testing.T) (string, actionprofile.Identity) {
	t.Helper()
	root := t.TempDir()
	for _, path := range []string{filepath.Join(root, "etc"), filepath.Join(root, "etc/home-lab-observer-actions")} {
		if os.Mkdir(path, 0750) != nil {
			t.Fatal("owned private tree")
		}
	}
	file := filepath.Join(root, "etc/home-lab-observer-actions/engine.json")
	if os.WriteFile(file, engineDocument(strings.Repeat("a", 32)), 0640) != nil {
		t.Fatal("synthetic approval file")
	}
	i := actionprofile.Identity{BrokerUID: uint32(os.Geteuid()), BrokerGID: uint32(os.Getegid()), IPCGID: uint32(os.Getegid())}
	return root, i
}
func pinFixtureRecord(t *testing.T, root string, i actionprofile.Identity) (*pinnedBrokerRecord, error) {
	t.Helper()
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		t.Fatal("owned fixture root")
	}
	return pinBrokerRecordAt(context.Background(), fd, engineRecord, uint32(os.Geteuid()), uint32(os.Getegid()), &i)
}

func TestBrokerCurrentWitnessAllowsOnlyUnrelatedAncestorEntryChanges(t *testing.T) {
	root, identity := brokerRecordFixture(t)
	first, err := pinFixtureRecord(t, root, identity)
	if err != nil {
		t.Fatal("first validated load")
	}
	if _, err := first.read(context.Background()); err != nil {
		t.Fatal("first bounded capture")
	}
	before := append([]unix.Stat_t(nil), first.witnesses...)
	// A directory sibling changes real parent nlink even with coarse timestamps.
	if os.Mkdir(filepath.Join(root, "etc/unrelated"), 0700) != nil {
		t.Fatal("owned unrelated ancestor entry")
	}
	if first.recheck(context.Background()) == nil {
		t.Fatal("per-read full witness checks were weakened")
	}
	first.close()
	second, err := pinFixtureRecord(t, root, identity)
	if err != nil {
		t.Fatal("second validated load")
	}
	defer second.close()
	if _, err := second.read(context.Background()); err != nil || before[1].Nlink == second.witnesses[1].Nlink {
		t.Fatal("actual changed ancestor fixture")
	}
	for index, old := range before {
		if !brokerCurrentWitnessSame(index, old, second.witnesses[index]) {
			t.Fatal("unchanged protected file refused after unrelated ancestor entry")
		}
	}
	for _, change := range []func(*unix.Stat_t){func(s *unix.Stat_t) { s.Ino++ }, func(s *unix.Stat_t) { s.Dev++ },
		func(s *unix.Stat_t) { s.Uid++ }, func(s *unix.Stat_t) { s.Gid++ }, func(s *unix.Stat_t) { s.Mode ^= 0020 }} {
		after := before[1]
		change(&after)
		if brokerCurrentWitnessSame(1, before[1], after) {
			t.Fatal("ancestor identity/permission drift admitted")
		}
	}
	for _, change := range []func(*unix.Stat_t){func(s *unix.Stat_t) { s.Nlink++ }, func(s *unix.Stat_t) { s.Size++ },
		func(s *unix.Stat_t) { s.Mtim.Nsec++ }, func(s *unix.Stat_t) { s.Ctim.Nsec++ }} {
		after := before[3]
		change(&after)
		if brokerCurrentWitnessSame(3, before[3], after) {
			t.Fatal("leaf full witness was weakened")
		}
	}
}
func TestProtectedBrokerReadPinsActualFilesAndExcludesAtimeOnlyChanges(t *testing.T) {
	root, i := brokerRecordFixture(t)
	p, err := pinFixtureRecord(t, root, i)
	if err != nil {
		t.Fatal("private synthetic owner metadata")
	}
	defer p.close()
	raw, err := p.read(context.Background())
	if err != nil || string(raw) != string(engineDocument(strings.Repeat("a", 32))) {
		t.Fatal("bounded actual file read")
	}
	if p.recheck(context.Background()) != nil {
		t.Fatal("ordinary cold read invalidated witness")
	}
	second, err := p.read(context.Background())
	if err != nil || string(second) != string(raw) {
		t.Fatal("repeated read reused stale descriptor EOF")
	}
}
func TestProtectedBrokerReadRefusesLinksACLsModesAndBounds(t *testing.T) {
	for _, mode := range []string{"leaf-link", "parent-link", "hardlink", "parent-write", "oversized", "acl", "wrong-group", "stage"} {
		t.Run(mode, func(t *testing.T) {
			root, i := brokerRecordFixture(t)
			dir := filepath.Join(root, "etc/home-lab-observer-actions")
			file := filepath.Join(dir, "engine.json")
			switch mode {
			case "leaf-link":
				if os.Rename(file, file+".original") != nil || os.Symlink("engine.json.original", file) != nil {
					t.Fatal("owned leaf link")
				}
			case "parent-link":
				if os.Rename(dir, dir+".original") != nil || os.Symlink("home-lab-observer-actions.original", dir) != nil {
					t.Fatal("owned parent link")
				}
			case "hardlink":
				if os.Link(file, filepath.Join(root, "second-link")) != nil {
					t.Fatal("owned hardlink")
				}
			case "parent-write":
				if os.Chmod(dir, 0770) != nil {
					t.Fatal("owned mode")
				}
			case "oversized":
				if os.WriteFile(file, []byte(strings.Repeat("x", engineDocumentLimit+1)), 0640) != nil {
					t.Fatal("owned bound")
				}
			case "acl":
				// Valid Linux POSIX ACL version2: user_obj rw, named unrelated
				// UID read, group_obj read, mask read, other none. No host ACL.
				acl := []byte{2, 0, 0, 0, 1, 0, 6, 0, 255, 255, 255, 255, 2, 0, 4, 0, 0, 240, 0, 0, 4, 0, 4, 0, 255, 255, 255, 255, 16, 0, 4, 0, 255, 255, 255, 255, 32, 0, 0, 0, 255, 255, 255, 255}
				if unix.Setxattr(file, "system.posix_acl_access", acl, 0) != nil {
					t.Fatal("owned filesystem ACL fixture unavailable")
				}
			case "wrong-group":
				i.BrokerGID++
			case "stage":
				if os.WriteFile(filepath.Join(dir, "policy.json.stage"), []byte("ambiguous"), 0640) != nil {
					t.Fatal("owned stage")
				}
			}
			p, err := pinFixtureRecord(t, root, i)
			if p != nil {
				p.close()
			}
			if err == nil {
				t.Fatal("unsafe protected broker source admitted")
			}
		})
	}
}
func TestProtectedBrokerReadDetectsReadWindowPathAndContentDrift(t *testing.T) {
	for _, mode := range []string{"replace-leaf", "change-content", "replace-parent", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			root, i := brokerRecordFixture(t)
			dir := filepath.Join(root, "etc/home-lab-observer-actions")
			file := filepath.Join(dir, "engine.json")
			p, err := pinFixtureRecord(t, root, i)
			if err != nil {
				t.Fatal("pin")
			}
			defer p.close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p.afterRead = func() {
				switch mode {
				case "replace-leaf":
					if os.Rename(file, file+".original") != nil || os.WriteFile(file, engineDocument(strings.Repeat("a", 32)), 0640) != nil {
						t.Fatal("owned replacement")
					}
				case "change-content":
					if os.WriteFile(file, engineDocument(strings.Repeat("b", 32)), 0640) != nil {
						t.Fatal("owned drift")
					}
				case "replace-parent":
					if os.Rename(dir, dir+".original") != nil || os.Mkdir(dir, 0750) != nil {
						t.Fatal("owned parent replacement")
					}
				case "cancel":
					cancel()
				}
			}
			if raw, err := p.read(ctx); err == nil || len(raw) != 0 {
				t.Fatal("read-window uncertainty exposed approval")
			}
		})
	}
}
