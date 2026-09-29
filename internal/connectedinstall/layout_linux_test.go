//go:build linux

package connectedinstall

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func testLayout() installLayout {
	return installLayout{
		digest:       strings.Repeat("a", 64),
		collectorUID: 60101,
		uploaderUID:  60102,
		uploaderGID:  60102,
		sharedGID:    60103,
	}
}

func TestClosedInstallLayout(t *testing.T) {
	layout := testLayout()
	seen := make(map[string]bool)
	for role := dirOptRoot; role <= dirCredentials; role++ {
		spec, err := layout.directory(role)
		if err != nil || !filepath.IsAbs(spec.parent) || spec.name == "" ||
			strings.ContainsAny(spec.name, `/\`) || spec.name == "." || spec.name == ".." || spec.mode == 0 {
			t.Fatalf("invalid fixed directory role %d", role)
		}
		path := filepath.Join(spec.parent, spec.name)
		if seen[path] {
			t.Fatalf("duplicate fixed destination %s", path)
		}
		seen[path] = true
	}
	for role := fileCollectorUnit; role <= fileNSS; role++ {
		spec, err := layout.file(role)
		if err != nil || !filepath.IsAbs(spec.parent) || spec.name == "" ||
			strings.ContainsAny(spec.name, `/\`) || spec.limit <= 0 || spec.mode == 0 {
			t.Fatalf("invalid fixed file role %d", role)
		}
		path := filepath.Join(spec.parent, spec.name)
		if seen[path] {
			t.Fatalf("duplicate fixed destination %s", path)
		}
		seen[path] = true
	}
	if _, err := layout.directory(0); err != ErrUnsafe {
		t.Fatal("zero directory role admitted")
	}
	if _, err := layout.file(0); err != ErrUnsafe {
		t.Fatal("zero file role admitted")
	}
	layout.digest = "../other"
	if _, err := layout.directory(dirRelease); err != ErrUnsafe {
		t.Fatal("noncanonical release component admitted")
	}
	layout = testLayout()
	layout.collectorUID = 0
	if _, err := layout.directory(dirHandoff); err != ErrUnsafe {
		t.Fatal("missing principal admitted")
	}
}

// The opt-in root fixture creates only new temp directories. It never calls
// Install or a public fixed-path writer and never touches /etc, /var or /opt.
func TestOwnedInstallLayoutFixture(t *testing.T) {
	if os.Getenv("OBSERVER_CONNECTED_INSTALL_LAYOUT_ACCEPTANCE") != "1" || os.Getuid() != 0 || os.Geteuid() != 0 {
		t.Skip("explicit synthetic root layout fixture only")
	}
	layout := testLayout()
	dirSpec, err := layout.directory(dirRelease)
	if err != nil {
		t.Fatal(err)
	}
	fileSpec, err := layout.file(fileCredential)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("exclusive directory and retained failure", func(t *testing.T) {
		parent, path := fixtureParent(t)
		defer parent.Close()
		if err := createDirectoryAt(parent, dirSpec, nil); err != nil {
			t.Fatal("new directory refused", err)
		}
		child, err := os.Open(filepath.Join(path, dirSpec.name))
		if err != nil || !exactDirectory(child, dirSpec) {
			t.Fatal("new directory metadata drifted", err)
		}
		child.Close()
		if err := createDirectoryAt(parent, dirSpec, nil); err != ErrRecovery {
			t.Fatal("existing directory adopted", err)
		}
	})
	for _, scenario := range []string{"foreign parent", "writable parent", "symlink target", "existing target", "child sync fails", "parent sync fails"} {
		t.Run(scenario, func(t *testing.T) {
			parent, path := fixtureParent(t)
			defer parent.Close()
			target := filepath.Join(path, dirSpec.name)
			want := ErrRecovery
			switch scenario {
			case "foreign parent":
				want = ErrUnsafe
				if os.Chown(path, 65534, 65534) != nil {
					t.Fatal("chown")
				}
			case "writable parent":
				want = ErrUnsafe
				if os.Chmod(path, 0777) != nil {
					t.Fatal("chmod")
				}
			case "symlink target":
				if os.Symlink(t.TempDir(), target) != nil {
					t.Fatal("symlink")
				}
			case "existing target":
				if os.WriteFile(target, nil, 0600) != nil {
					t.Fatal("write")
				}
			}
			calls := 0
			syncFile := func(file *os.File) error {
				calls++
				if scenario == "child sync fails" && calls == 1 || scenario == "parent sync fails" && calls == 2 {
					return errors.New("synthetic sync failure")
				}
				return file.Sync()
			}
			if err := createDirectoryAt(parent, dirSpec, syncFile); err != want {
				t.Fatal("unsafe or interrupted directory admitted", err)
			}
			if strings.Contains(scenario, "sync fails") {
				if _, err := os.Lstat(target); err != nil || createDirectoryAt(parent, dirSpec, nil) != ErrRecovery {
					t.Fatal("interrupted directory was lost or adopted")
				}
			}
		})
	}
	t.Run("bounded no-replace credential publication", func(t *testing.T) {
		parent, path := fixtureParent(t)
		defer parent.Close()
		data := []byte("synthetic-secret")
		if err := publishFileAt(parent, fileSpec, data, nil); err != nil {
			t.Fatal("new file refused", err)
		}
		got, err := os.ReadFile(filepath.Join(path, fileSpec.name))
		if err != nil || !bytes.Equal(got, data) {
			t.Fatal("published bytes changed", err)
		}
		if err := publishFileAt(parent, fileSpec, data, nil); err != ErrRecovery {
			t.Fatal("existing credential adopted", err)
		}
		if err := publishFileAt(parent, fileSpec, bytes.Repeat([]byte("x"), fileSpec.limit+1), nil); err != ErrUnsafe {
			t.Fatal("unbounded credential accepted", err)
		}
	})
	for _, scenario := range []string{"symlink target", "FIFO temp", "hardlink temp", "file sync fails", "temp directory sync fails", "rename directory sync fails"} {
		t.Run(scenario, func(t *testing.T) {
			parent, path := fixtureParent(t)
			defer parent.Close()
			target := filepath.Join(path, fileSpec.name)
			temp := filepath.Join(path, "."+fileSpec.name+".install-next")
			switch scenario {
			case "symlink target":
				if os.Symlink("other", target) != nil {
					t.Fatal("symlink")
				}
			case "FIFO temp":
				if unix.Mkfifo(temp, 0600) != nil {
					t.Fatal("mkfifo")
				}
			case "hardlink temp":
				source := filepath.Join(path, "source")
				if os.WriteFile(source, nil, 0600) != nil || os.Link(source, temp) != nil {
					t.Fatal("hardlink")
				}
			}
			calls := 0
			syncFile := func(file *os.File) error {
				calls++
				failure := map[string]int{"file sync fails": 1, "temp directory sync fails": 2, "rename directory sync fails": 3}[scenario]
				if failure != 0 && calls == failure {
					return errors.New("synthetic sync failure")
				}
				return file.Sync()
			}
			if err := publishFileAt(parent, fileSpec, []byte("synthetic-secret"), syncFile); err != ErrRecovery {
				t.Fatal("unsafe or interrupted publication admitted", err)
			}
			if strings.Contains(scenario, "sync fails") {
				if scenario == "rename directory sync fails" {
					if _, err := os.Lstat(target); err != nil {
						t.Fatal("post-rename uncertainty lost target evidence", err)
					}
				} else if _, err := os.Lstat(temp); err != nil {
					t.Fatal("pre-rename uncertainty lost temp evidence", err)
				}
				if publishFileAt(parent, fileSpec, []byte("retry"), nil) != ErrRecovery {
					t.Fatal("interrupted role was adopted")
				}
			}
		})
	}
}

func fixtureParent(t *testing.T) (*os.File, string) {
	t.Helper()
	path := t.TempDir()
	if os.Chmod(path, 0700) != nil {
		t.Fatal("chmod")
	}
	parent, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return parent, path
}
