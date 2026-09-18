//go:build linux

package connectedpreparing

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

var fixtureRecord = Record{
	ServerID:       "srv_0123456789abcdef0123456789abcdef",
	ManifestSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
}

func TestTypedPreparingRecord(t *testing.T) {
	data, err := fixtureRecord.encode()
	if err != nil || !bytes.Equal(data, []byte(`{"version":"observer-connected-preparing/v1","state":"PREPARING","server_id":"srv_0123456789abcdef0123456789abcdef","manifest_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`)) {
		t.Fatal("preparing marker is not the fixed canonical record", err)
	}
	for _, record := range []Record{
		{},
		{ServerID: "srv_invalid", ManifestSHA256: fixtureRecord.ManifestSHA256},
		{ServerID: fixtureRecord.ServerID, ManifestSHA256: "short"},
		{ServerID: fixtureRecord.ServerID, ManifestSHA256: "A" + fixtureRecord.ManifestSHA256[1:]},
	} {
		if data, err := record.encode(); data != nil || err != ErrUnsafe {
			t.Fatal("invalid marker admitted")
		}
	}
}

func TestNonRootPreparingPublicationRefuses(t *testing.T) {
	if os.Getuid() == 0 || os.Geteuid() == 0 {
		t.Skip("ordinary Linux native test proves non-root refusal")
	}
	if err := Publish(fixtureRecord); err != ErrUnsafe {
		t.Fatal("non-root publisher admitted", err)
	}
}

// Explicit synthetic-root fixture only. It never calls Publish or touches /etc.
func TestOwnedPreparingPublicationFixture(t *testing.T) {
	if os.Getenv("OBSERVER_CONNECTED_PREPARING_ACCEPTANCE") != "1" || os.Getuid() != 0 || os.Geteuid() != 0 {
		t.Skip("explicit synthetic root preparing fixture only")
	}
	t.Run("canonical durable marker", func(t *testing.T) {
		dir, path := fixtureDirectory(t)
		defer dir.Close()
		if err := publishAt(dir, fixtureRecord, nil); err != nil {
			t.Fatal("new marker refused", err)
		}
		want, _ := fixtureRecord.encode()
		got, err := os.ReadFile(filepath.Join(path, markerName))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("unexpected published bytes", err)
		}
		if _, err := os.Lstat(filepath.Join(path, nextName)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("temporary role remained after success")
		}
		fd, err := unix.Openat(int(dir.Fd()), markerName, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatal(err)
		}
		file := os.NewFile(uintptr(fd), markerName)
		if !trustedMarker(file, int64(len(want))) {
			t.Fatal("published marker has unsafe ownership or mode")
		}
		file.Close()
		if err := publishAt(dir, fixtureRecord, nil); err != ErrRecovery {
			t.Fatal("existing marker adopted", err)
		}
	})
	for _, scenario := range []string{"foreign directory", "writable directory", "wrong directory mode", "ACL directory", "unknown entry", "existing target", "symlink target", "existing temp", "symlink temp", "hardlink temp", "FIFO temp"} {
		t.Run(scenario, func(t *testing.T) {
			dir, path := fixtureDirectory(t)
			defer dir.Close()
			wantErr := ErrRecovery
			switch scenario {
			case "foreign directory":
				wantErr = ErrUnsafe
				if os.Chown(path, 65534, 65534) != nil {
					t.Fatal("chown")
				}
			case "writable directory":
				wantErr = ErrUnsafe
				if os.Chmod(path, 0777) != nil {
					t.Fatal("chmod")
				}
			case "wrong directory mode":
				wantErr = ErrUnsafe
				if os.Chmod(path, 0700) != nil {
					t.Fatal("chmod")
				}
			case "ACL directory":
				wantErr = ErrUnsafe
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
			case "unknown entry":
				if os.WriteFile(filepath.Join(path, "other"), nil, 0600) != nil {
					t.Fatal("write")
				}
			case "existing target":
				if os.WriteFile(filepath.Join(path, markerName), []byte("existing"), 0600) != nil {
					t.Fatal("write")
				}
			case "symlink target":
				if os.Symlink("other", filepath.Join(path, markerName)) != nil {
					t.Fatal("symlink")
				}
			case "existing temp":
				if os.WriteFile(filepath.Join(path, nextName), []byte("existing"), 0600) != nil {
					t.Fatal("write")
				}
			case "symlink temp":
				if os.Symlink("other", filepath.Join(path, nextName)) != nil {
					t.Fatal("symlink")
				}
			case "hardlink temp":
				source := filepath.Join(path, "source")
				if os.WriteFile(source, nil, 0600) != nil || os.Link(source, filepath.Join(path, nextName)) != nil {
					t.Fatal("hardlink")
				}
			case "FIFO temp":
				if unix.Mkfifo(filepath.Join(path, nextName), 0600) != nil {
					t.Fatal("mkfifo")
				}
			}
			if err := publishAt(dir, fixtureRecord, nil); err != wantErr {
				t.Fatal("unsafe/occupied directory admitted", err)
			}
			if _, err := os.Lstat(filepath.Join(path, markerName)); scenario != "existing target" && scenario != "symlink target" && !errors.Is(err, os.ErrNotExist) {
				t.Fatal("publisher wrote marker into refused directory")
			}
		})
	}
	t.Run("synced interruption is recovery evidence", func(t *testing.T) {
		dir, path := fixtureDirectory(t)
		defer dir.Close()
		interrupted := false
		if err := publishAt(dir, fixtureRecord, func() error {
			interrupted = true
			return errors.New("synthetic interruption")
		}); err != ErrRecovery || !interrupted {
			t.Fatal("interruption was not reported as recovery-required", err)
		}
		want, _ := fixtureRecord.encode()
		got, err := os.ReadFile(filepath.Join(path, nextName))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("interrupted temporary evidence missing", err)
		}
		if err := publishAt(dir, fixtureRecord, nil); err != ErrRecovery {
			t.Fatal("interrupted marker was adopted or cleaned up", err)
		}
		if _, err := os.Lstat(filepath.Join(path, markerName)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("retry promoted interrupted evidence")
		}
	})
	t.Run("symlinked child directory refuses", func(t *testing.T) {
		parent, path := fixtureDirectory(t)
		defer parent.Close()
		if os.Symlink(t.TempDir(), filepath.Join(path, "child")) != nil {
			t.Fatal("symlink")
		}
		if child, err := openDirAt(int(parent.Fd()), "child"); err == nil || child != nil {
			t.Fatal("symlinked directory opened")
		}
	})
}

func fixtureDirectory(t *testing.T) (*os.File, string) {
	t.Helper()
	path := t.TempDir()
	if os.Chmod(path, 0755) != nil {
		t.Fatal("chmod")
	}
	dir, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return dir, path
}
