//go:build linux

package connectedinstall

import (
	"bytes"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// This fixture has no installed paths or service calls. It runs only when a
// reviewer explicitly invokes tests as root in a disposable local environment.
func TestRootPublicationBoundsAndNoReplace(t *testing.T) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		t.Skip("owned temporary root fixture requires explicit root invocation")
	}
	for _, scenario := range []string{"normal", "collision", "symlink", "large-ca", "too-large"} {
		t.Run(scenario, func(t *testing.T) {
			path := t.TempDir()
			root, err := os.OpenRoot(path)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			payload := []byte("synthetic-record")
			switch scenario {
			case "normal":
				if err := publishNew(root, "record", payload, 0600); err != nil {
					t.Fatal(err)
				}
				got, err := root.ReadFile("record")
				if err != nil || !bytes.Equal(got, payload) {
					t.Fatal("publication changed bytes")
				}
			case "collision", "symlink":
				if err := root.WriteFile("original", []byte("unchanged"), 0600); err != nil {
					t.Fatal(err)
				}
				if scenario == "symlink" {
					if err := root.Symlink("original", "record"); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := root.WriteFile("record", []byte("unchanged"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if publishNew(root, "record", payload, 0600) != ErrRecovery {
					t.Fatal("existing target overwritten")
				}
				got, err := root.ReadFile("record")
				if err != nil || string(got) != "unchanged" {
					t.Fatal("existing evidence changed")
				}
			case "large-ca":
				ca := bytes.Repeat([]byte("x"), 128<<10)
				if publishNew(root, "record", ca, 0644) != ErrUnsafe {
					t.Fatal("ordinary record bound relaxed")
				}
				if err := publishCA(root, ca); err != nil {
					t.Fatal("bounded CA refused", err)
				}
			case "too-large":
				if publishCA(root, bytes.Repeat([]byte("x"), (1<<20)+1)) != ErrUnsafe {
					t.Fatal("unbounded CA accepted")
				}
				entries, err := root.Open(".")
				if err != nil {
					t.Fatal(err)
				}
				defer entries.Close()
				names, err := entries.Readdirnames(-1)
				if err != nil || len(names) != 0 {
					t.Fatal("refused data mutated root")
				}
			}
		})
	}
}

func TestMissingAccountsDoNotHideLookupFailure(t *testing.T) {
	if !missingUser(user.UnknownUserError("synthetic")) || !missingGroup(user.UnknownGroupError("synthetic")) {
		t.Fatal("actual missing principal refused")
	}
	for _, err := range []error{nil, errors.New("lookup unavailable")} {
		if missingUser(err) || missingGroup(err) {
			t.Fatal("ambiguous lookup treated as absent")
		}
	}
}

func TestPrivateDirectoryPinsOnlyExactOwnedDirectory(t *testing.T) {
	base := t.TempDir()
	parent, err := os.Open(base)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	path := filepath.Join(base, "owned")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	uid, gid := uint32(os.Getuid()), uint32(os.Getgid())
	f, err := privateDirectoryAt(parent, "owned", uid, gid)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := privateDirectoryAt(parent, "owned", uid+1, gid); err == nil {
		t.Fatal("wrong principal accepted")
	}
	if err := os.Symlink("owned", filepath.Join(base, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := privateDirectoryAt(parent, "link", uid, gid); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := unix.Mkfifo(filepath.Join(base, "fifo"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := privateDirectoryAt(parent, "fifo", uid, gid); err == nil {
		t.Fatal("FIFO accepted")
	}
	if err := os.Chmod(path, 0750); err != nil {
		t.Fatal(err)
	}
	if _, err := privateDirectoryAt(parent, "owned", uid, gid); err == nil {
		t.Fatal("broad directory accepted")
	}
}
