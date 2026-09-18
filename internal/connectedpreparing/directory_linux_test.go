//go:build linux

package connectedpreparing

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNonRootConfigDirectoryCreationRefuses(t *testing.T) {
	if os.Getuid() == 0 || os.Geteuid() == 0 {
		t.Skip("ordinary Linux native test proves non-root refusal")
	}
	if err := CreateConfigDirectory(); err != ErrUnsafe {
		t.Fatal("non-root directory creator admitted", err)
	}
}

// Explicit synthetic-root fixture only. It never calls CreateConfigDirectory
// or touches /etc, accounts, units, credentials, or an installed service.
func TestOwnedConfigDirectoryFixture(t *testing.T) {
	if os.Getenv("OBSERVER_CONNECTED_CONFIG_ROOT_ACCEPTANCE") != "1" || os.Getuid() != 0 || os.Geteuid() != 0 {
		t.Skip("explicit synthetic root config fixture only")
	}
	t.Run("creates only fixed directory", func(t *testing.T) {
		etc, path := fixtureDirectory(t)
		defer etc.Close()
		if err := createAt(etc, nil); err != nil {
			t.Fatal("new config directory refused", err)
		}
		config, err := openDirAt(int(etc.Fd()), configDirectoryName)
		if err != nil || !trustedDirectory(config, true) {
			t.Fatal("created directory has unsafe metadata", err)
		}
		config.Close()
		if err := createAt(etc, nil); err != ErrRecovery {
			t.Fatal("existing directory adopted", err)
		}
		entries, err := os.ReadDir(path)
		if err != nil || len(entries) != 1 || entries[0].Name() != configDirectoryName {
			t.Fatal("unexpected created entries", err)
		}
	})
	for _, scenario := range []string{"foreign parent", "writable parent", "ACL parent", "existing directory", "existing file", "symlink target"} {
		t.Run(scenario, func(t *testing.T) {
			etc, path := fixtureDirectory(t)
			defer etc.Close()
			target := filepath.Join(path, configDirectoryName)
			wantErr := ErrRecovery
			switch scenario {
			case "foreign parent":
				wantErr = ErrUnsafe
				if os.Chown(path, 65534, 65534) != nil {
					t.Fatal("chown")
				}
			case "writable parent":
				wantErr = ErrUnsafe
				if os.Chmod(path, 0777) != nil {
					t.Fatal("chmod")
				}
			case "ACL parent":
				wantErr = ErrUnsafe
				fixtureACL(t, etc)
			case "existing directory":
				if os.Mkdir(target, 0755) != nil {
					t.Fatal("mkdir")
				}
			case "existing file":
				if os.WriteFile(target, []byte("existing"), 0600) != nil {
					t.Fatal("write")
				}
			case "symlink target":
				if os.Symlink(t.TempDir(), target) != nil {
					t.Fatal("symlink")
				}
			}
			if err := createAt(etc, nil); err != wantErr {
				t.Fatal("hostile parent or occupied path admitted", err)
			}
			if scenario == "foreign parent" || scenario == "writable parent" || scenario == "ACL parent" {
				if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("creator wrote into unsafe parent")
				}
			}
		})
	}
	for _, failedSync := range []int{1, 2} {
		t.Run(map[int]string{1: "new directory sync fails", 2: "parent sync fails"}[failedSync], func(t *testing.T) {
			etc, path := fixtureDirectory(t)
			defer etc.Close()
			calls := 0
			err := createAt(etc, func(file *os.File) error {
				calls++
				if calls == failedSync {
					return errors.New("synthetic sync failure")
				}
				return file.Sync()
			})
			if err != ErrRecovery || calls != failedSync {
				t.Fatal("sync failure did not require recovery", err)
			}
			config, openErr := openDirAt(int(etc.Fd()), configDirectoryName)
			if openErr != nil || !trustedDirectory(config, true) {
				t.Fatal("failed sync lost creation residue", openErr)
			}
			config.Close()
			if err := createAt(etc, nil); err != ErrRecovery {
				t.Fatal("retry adopted failed-sync residue", err)
			}
			if _, err := os.Lstat(filepath.Join(path, configDirectoryName)); err != nil {
				t.Fatal("retry removed residue", err)
			}
		})
	}
}
