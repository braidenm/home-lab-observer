//go:build linux

package connectedstateroot

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func TestFixedStatePathMatchesInstalledContract(t *testing.T) {
	if connectedprofile.StateDirectory != "/var/lib/"+stateName {
		t.Fatal("fixed state path drifted from installed profile")
	}
}

func TestNonRootStateRootCreationRefuses(t *testing.T) {
	if os.Getuid() == 0 || os.Geteuid() == 0 {
		t.Skip("ordinary native tests prove non-root refusal")
	}
	if err := Create(); err != ErrUnsafe {
		t.Fatal("non-root state-root creation admitted", err)
	}
}

// Explicit synthetic-root fixture only; it never calls Create or touches /var.
func TestOwnedStateRootFixture(t *testing.T) {
	if os.Getenv("OBSERVER_CONNECTED_STATE_ROOT_ACCEPTANCE") != "1" || os.Getuid() != 0 || os.Geteuid() != 0 {
		t.Skip("explicit synthetic root fixture only")
	}
	t.Run("creates only fixed state root", func(t *testing.T) {
		varDir, path := fixtureVar(t)
		defer varDir.Close()
		if err := createAt(varDir, nil); err != nil {
			t.Fatal("new state root refused", err)
		}
		state, err := openDirAt(int(varDir.Fd()), "lib/"+stateName)
		if err != nil || !trustedDirectory(state, true) {
			t.Fatal("created state root has unsafe metadata", err)
		}
		state.Close()
		if err := createAt(varDir, nil); err != ErrRecovery {
			t.Fatal("existing state root adopted", err)
		}
		entries, err := os.ReadDir(filepath.Join(path, "lib"))
		if err != nil || len(entries) != 1 || entries[0].Name() != stateName {
			t.Fatal("unexpected state-root entries", err)
		}
	})
	for _, scenario := range []string{"foreign var", "writable var", "ACL var", "foreign lib", "writable lib", "ACL lib", "symlink lib", "existing directory", "existing file", "symlink state"} {
		t.Run(scenario, func(t *testing.T) {
			varDir, path := fixtureVar(t)
			defer varDir.Close()
			lib := filepath.Join(path, "lib")
			target := filepath.Join(lib, stateName)
			wantErr := ErrUnsafe
			switch scenario {
			case "foreign var":
				if os.Chown(path, 65534, 65534) != nil {
					t.Fatal("chown")
				}
			case "writable var":
				if os.Chmod(path, 0777) != nil {
					t.Fatal("chmod")
				}
			case "ACL var":
				fixtureACL(t, varDir)
			case "foreign lib":
				if os.Chown(lib, 65534, 65534) != nil {
					t.Fatal("chown")
				}
			case "writable lib":
				if os.Chmod(lib, 0777) != nil {
					t.Fatal("chmod")
				}
			case "ACL lib":
				file, err := os.Open(lib)
				if err != nil {
					t.Fatal(err)
				}
				fixtureACL(t, file)
				file.Close()
			case "symlink lib":
				if os.Remove(lib) != nil || os.Symlink(t.TempDir(), lib) != nil {
					t.Fatal("symlink")
				}
			case "existing directory":
				wantErr = ErrRecovery
				if os.Mkdir(target, 0755) != nil {
					t.Fatal("mkdir")
				}
			case "existing file":
				wantErr = ErrRecovery
				if os.WriteFile(target, []byte("existing"), 0600) != nil {
					t.Fatal("write")
				}
			case "symlink state":
				wantErr = ErrRecovery
				if os.Symlink(t.TempDir(), target) != nil {
					t.Fatal("symlink")
				}
			}
			if err := createAt(varDir, nil); err != wantErr {
				t.Fatal("hostile ancestor or occupied state root admitted", err)
			}
			if wantErr == ErrUnsafe && scenario != "symlink lib" {
				if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("creator wrote into unsafe ancestor")
				}
			}
		})
	}
	for _, failedSync := range []int{1, 2} {
		t.Run(map[int]string{1: "new state sync fails", 2: "lib sync fails"}[failedSync], func(t *testing.T) {
			varDir, path := fixtureVar(t)
			defer varDir.Close()
			calls := 0
			err := createAt(varDir, func(file *os.File) error {
				calls++
				if calls == failedSync {
					return errors.New("synthetic sync failure")
				}
				return file.Sync()
			})
			if err != ErrRecovery || calls != failedSync {
				t.Fatal("sync failure did not require recovery", err)
			}
			state, openErr := openDirAt(int(varDir.Fd()), "lib/"+stateName)
			if openErr != nil || !trustedDirectory(state, true) {
				t.Fatal("failed sync lost state-root residue", openErr)
			}
			state.Close()
			if err := createAt(varDir, nil); err != ErrRecovery {
				t.Fatal("retry adopted failed-sync state-root residue", err)
			}
			if _, err := os.Lstat(filepath.Join(path, "lib", stateName)); err != nil {
				t.Fatal("retry removed residue", err)
			}
		})
	}
}

func fixtureVar(t *testing.T) (*os.File, string) {
	t.Helper()
	path := t.TempDir()
	lib := filepath.Join(path, "lib")
	if os.Chmod(path, 0755) != nil || os.Mkdir(lib, 0755) != nil || os.Chmod(lib, 0755) != nil {
		t.Fatal("create fixture ancestors")
	}
	dir, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return dir, path
}

func fixtureACL(t *testing.T, dir *os.File) {
	t.Helper()
	acl := []byte{
		2, 0, 0, 0,
		1, 0, 7, 0, 255, 255, 255, 255,
		2, 0, 4, 0, 254, 255, 0, 0,
		4, 0, 5, 0, 255, 255, 255, 255,
		16, 0, 5, 0, 255, 255, 255, 255,
		32, 0, 5, 0, 255, 255, 255, 255,
	}
	if err := unix.Fsetxattr(int(dir.Fd()), "system.posix_acl_access", acl, 0); err != nil {
		if err == unix.ENOTSUP {
			t.Skip("fixture filesystem has no POSIX ACLs")
		}
		t.Fatal("set ACL", err)
	}
}
