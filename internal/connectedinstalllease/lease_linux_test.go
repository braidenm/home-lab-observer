//go:build linux

package connectedinstalllease

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNonRootAcquisitionRefusesWithoutCreatingLock(t *testing.T) {
	if os.Getuid() == 0 || os.Geteuid() == 0 {
		t.Skip("non-root refusal runs in ordinary native tests")
	}
	if lease, err := Acquire(); lease != nil || err != ErrUnsafe {
		t.Fatal("non-root installation lease admitted")
	}
}

// This fixture runs only under the explicit GitHub-hosted synthetic root job.
// It never opens the production /run/lock path or an installed service.
func TestOwnedInstallLeaseFixture(t *testing.T) {
	if os.Getenv("OBSERVER_CONNECTED_LEASE_ACCEPTANCE") != "1" || os.Getuid() != 0 || os.Geteuid() != 0 {
		t.Skip("explicit synthetic root lease fixture only")
	}
	t.Run("exclusive and retained", func(t *testing.T) {
		parent, path := ownedParent(t)
		defer parent.Close()
		first, err := acquireAt(parent)
		if err != nil {
			t.Fatal("first lease refused", err)
		}
		if second, err := acquireAt(parent); second != nil || err != ErrBusy {
			t.Fatal("contending lease admitted")
		}
		if first.Close() != nil {
			t.Fatal("lease close failed")
		}
		if _, err := os.Lstat(filepath.Join(path, lockName)); err != nil {
			t.Fatal("lock file was removed")
		}
		third, err := acquireAt(parent)
		if err != nil || third == nil {
			t.Fatal("released lease could not be reacquired", err)
		}
		if third.Close() != nil || third.Close() != ErrUnsafe {
			t.Fatal("lease close was not single-use")
		}
	})
	for _, scenario := range []string{"foreign parent", "writable parent", "symlink lock", "hardlink lock", "FIFO lock", "foreign lock", "broad lock", "nonempty lock"} {
		t.Run(scenario, func(t *testing.T) {
			parent, path := ownedParent(t)
			defer parent.Close()
			lock := filepath.Join(path, lockName)
			switch scenario {
			case "foreign parent":
				if os.Chown(path, 65534, 65534) != nil {
					t.Fatal("chown")
				}
			case "writable parent":
				if os.Chmod(path, 0777) != nil {
					t.Fatal("chmod")
				}
			case "symlink lock":
				if os.Symlink("unowned", lock) != nil {
					t.Fatal("symlink")
				}
			case "hardlink lock":
				source := filepath.Join(path, "source")
				if os.WriteFile(source, nil, 0600) != nil || os.Link(source, lock) != nil {
					t.Fatal("hardlink")
				}
			case "FIFO lock":
				if unix.Mkfifo(lock, 0600) != nil {
					t.Fatal("FIFO")
				}
			case "foreign lock":
				if os.WriteFile(lock, nil, 0600) != nil || os.Chown(lock, 65534, 65534) != nil {
					t.Fatal("foreign lock")
				}
			case "broad lock":
				if os.WriteFile(lock, nil, 0644) != nil {
					t.Fatal("broad lock")
				}
			case "nonempty lock":
				if os.WriteFile(lock, []byte("not-empty"), 0600) != nil {
					t.Fatal("nonempty lock")
				}
			}
			if lease, err := acquireAt(parent); lease != nil || err != ErrUnsafe {
				t.Fatal("unsafe lock entry admitted", err)
			}
		})
	}
	t.Run("sticky shared parent", func(t *testing.T) {
		parent, path := ownedParent(t)
		defer parent.Close()
		if os.Chmod(path, os.ModeSticky|0777) != nil {
			t.Fatal("chmod")
		}
		lease, err := acquireAt(parent)
		if err != nil || lease == nil {
			t.Fatal("root-owned sticky lock directory refused", err)
		}
		if lease.Close() != nil {
			t.Fatal("lease close")
		}
	})
	for _, scenario := range []string{"trusted run", "foreign run", "writable run", "symlink lock directory"} {
		t.Run(scenario, func(t *testing.T) {
			run, path := ownedParent(t)
			defer run.Close()
			lockDir := filepath.Join(path, "lock")
			if scenario == "symlink lock directory" {
				if os.Symlink(t.TempDir(), lockDir) != nil {
					t.Fatal("symlink")
				}
			} else if os.Mkdir(lockDir, 0755) != nil {
				t.Fatal("mkdir")
			}
			switch scenario {
			case "foreign run":
				if os.Chown(path, 65534, 65534) != nil {
					t.Fatal("chown")
				}
			case "writable run":
				if os.Chmod(path, 0777) != nil {
					t.Fatal("chmod")
				}
			}
			lease, err := acquireUnder(run)
			if scenario == "trusted run" {
				if err != nil || lease == nil || lease.Close() != nil {
					t.Fatal("trusted /run fixture refused", err)
				}
			} else if lease != nil || err != ErrUnsafe {
				t.Fatal("hostile /run fixture admitted", err)
			}
		})
	}
}

func ownedParent(t *testing.T) (*os.File, string) {
	t.Helper()
	path := t.TempDir()
	parent, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return parent, path
}
