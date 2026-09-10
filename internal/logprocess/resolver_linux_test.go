package logprocess

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func linuxIdentityFixture(t *testing.T) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	main := filepath.Join(dir, "observer")
	helper := filepath.Join(dir, "observer-journal-helper")
	if err := os.WriteFile(main, []byte("synthetic-main"), 0700); err != nil {
		t.Fatal(err)
	}
	content := []byte("synthetic-helper-not-executable")
	if err := os.WriteFile(helper, content, 0700); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return main, helper, hex.EncodeToString(sum[:])
}

func TestLinuxFixedDigestAndEnvironment(t *testing.T) {
	main, helper, digest := linuxIdentityFixture(t)
	spec, err := resolvePlatformHelper(main, digest)
	if err != nil || spec.path != helper || len(spec.args) != 0 || strings.Join(spec.env, ";") != "LANG=C;LC_ALL=C;TZ=UTC" {
		t.Fatalf("fixed identity: %v", err)
	}
	for _, wrong := range []string{"", strings.ToUpper(digest), strings.Repeat("0", 64), "../../synthetic"} {
		if _, err := resolvePlatformHelper(main, wrong); !errors.Is(err, ErrHelperMismatch) {
			t.Fatalf("bad digest: %v", err)
		}
	}
}

func TestLinuxRejectsLinkedWritablePrivilegedOrOversizedFiles(t *testing.T) {
	for _, change := range []string{"symlink", "hardlink", "write", "setuid", "setgid", "empty", "oversized", "directory", "parent-write", "main-link"} {
		t.Run(change, func(t *testing.T) {
			main, helper, digest := linuxIdentityFixture(t)
			var err error
			switch change {
			case "symlink":
				err = os.Rename(helper, helper+"-original")
				if err == nil {
					err = os.Symlink(helper+"-original", helper)
				}
			case "hardlink":
				err = os.Link(helper, helper+"-alias")
			case "write":
				err = os.Chmod(helper, 0777)
			case "setuid":
				err = os.Chmod(helper, 0700|os.ModeSetuid)
			case "setgid":
				err = os.Chmod(helper, 0700|os.ModeSetgid)
			case "empty":
				err = os.Truncate(helper, 0)
			case "oversized":
				err = os.Truncate(helper, maxHelperBytes+1)
			case "directory":
				err = os.Remove(helper)
				if err == nil {
					err = os.Mkdir(helper, 0700)
				}
			case "parent-write":
				err = os.Chmod(filepath.Dir(main), 0777)
			case "main-link":
				err = os.Rename(main, main+"-original")
				if err == nil {
					err = os.Symlink(main+"-original", main)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := resolvePlatformHelper(main, digest); !errors.Is(err, ErrHelperUnavailable) {
				t.Fatalf("unsafe identity accepted: %v", err)
			}
		})
	}
}

func TestLinuxOwnerAndStickyAncestorPolicy(t *testing.T) {
	current := uint32(os.Geteuid())
	for _, tc := range []struct {
		mode uint32
		uid  uint32
		safe bool
	}{
		{unix.S_IFDIR | 0755, current, true}, {unix.S_IFDIR | 0777, current, false},
		{unix.S_IFDIR | unix.S_ISVTX | 0777, 0, true}, {unix.S_IFDIR | 0755, current + 1, false},
	} {
		if got := safeLinuxDirectory(unix.Stat_t{Mode: tc.mode, Uid: tc.uid}); got != tc.safe {
			t.Fatalf("directory policy %v", tc)
		}
	}
	base := unix.Stat_t{Mode: unix.S_IFREG | 0755, Uid: current, Nlink: 1, Size: 1}
	if !safeLinuxExecutable(base) {
		t.Fatal("safe file refused")
	}
	for _, bad := range []unix.Stat_t{
		{Mode: unix.S_IFREG | 0755, Uid: current + 1, Nlink: 1, Size: 1},
		{Mode: unix.S_IFREG | 0644, Uid: current, Nlink: 1, Size: 1},
		{Mode: unix.S_IFREG | 0755 | unix.S_ISUID, Uid: current, Nlink: 1, Size: 1},
	} {
		if safeLinuxExecutable(bad) {
			t.Fatal("untrusted executable accepted")
		}
	}
}
