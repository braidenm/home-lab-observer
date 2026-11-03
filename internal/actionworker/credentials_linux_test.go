//go:build linux

package actionworker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/actionprofile"
	"golang.org/x/sys/unix"
)

func TestReaderPathsStayBoundToFixedProfile(t *testing.T) {
	for path, parts := range map[string][]protectedPart{actionprofile.WorkerSettingsPath: settingsParts(actionprofile.Identity{}), actionprofile.ActiveCredentialPath: activeParts(actionprofile.Identity{})} {
		names := make([]string, len(parts))
		for index, part := range parts {
			names[index] = part.name
		}
		if "/"+strings.Join(names, "/") != path {
			t.Fatal("reader drifted from fixed process profile")
		}
	}
}

func TestProtectedCredentialReadRefusesInPlaceContentDrift(t *testing.T) {
	root := t.TempDir()
	if os.Chmod(root, 0700) != nil {
		t.Fatal("private root")
	}
	path := filepath.Join(root, "active.json")
	if os.WriteFile(path, []byte(activeFixture()), 0600) != nil {
		t.Fatal("private active state")
	}
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal("private root descriptor")
	}
	parts := []protectedPart{{"active.json", uint32(os.Geteuid()), uint32(os.Getegid()), 0600}}
	raw, err := readProtectedAtObserved(context.Background(), fd, parts, func() {
		changed := strings.Replace(activeFixture(), `"credential_revision":"1"`, `"credential_revision":"2"`, 1)
		if os.WriteFile(path, []byte(changed), 0600) != nil {
			t.Fatal("private in-place update")
		}
	})
	clear(raw)
	if err == nil {
		t.Fatal("in-place read-window change admitted")
	}
}

func TestProtectedActiveFile(t *testing.T) {
	root := t.TempDir()
	if os.Chmod(root, 0700) != nil {
		t.Fatal("temp root mode")
	}
	dir := filepath.Join(root, "worker")
	if os.Mkdir(dir, 0700) != nil {
		t.Fatal("temp child")
	}
	path := filepath.Join(dir, "active.json")
	if os.WriteFile(path, []byte(activeFixture()), 0600) != nil {
		t.Fatal("temp record")
	}
	parts := []protectedPart{{"worker", uint32(os.Geteuid()), uint32(os.Getegid()), 0700}, {"active.json", uint32(os.Geteuid()), uint32(os.Getegid()), 0600}}
	read := func(ctx context.Context, plan []protectedPart) ([]byte, error) {
		fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatal("temp root open")
		}
		return readProtectedAt(ctx, fd, plan)
	}
	before, _ := os.ReadDir("/proc/self/fd")
	for range 4 {
		raw, err := read(context.Background(), parts)
		if err != nil || string(raw) != activeFixture() {
			t.Fatal("protected read refused")
		}
		clear(raw)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := read(ctx, parts); err == nil {
		t.Fatal("canceled read admitted")
	}
	after, _ := os.ReadDir("/proc/self/fd")
	if len(after) != len(before) {
		t.Fatal("descriptor leak")
	}
	if os.Chmod(path, 0640) != nil {
		t.Fatal("temp mode change")
	}
	if _, err := read(context.Background(), parts); err == nil {
		t.Fatal("broad record admitted")
	}
	if os.Chmod(path, 0600) != nil {
		t.Fatal("temp mode restore")
	}
	link := filepath.Join(dir, "duplicate")
	if os.Link(path, link) != nil {
		t.Fatal("temp hardlink")
	}
	if _, err := read(context.Background(), parts); err == nil {
		t.Fatal("hardlink admitted")
	}
	if os.Remove(link) != nil {
		t.Fatal("temp hardlink cleanup")
	}
	if os.Rename(path, filepath.Join(dir, "saved")) != nil || os.Symlink("saved", path) != nil {
		t.Fatal("temp symlink")
	}
	if _, err := read(context.Background(), parts); err == nil {
		t.Fatal("file symlink admitted")
	}
	if os.Remove(path) != nil || unix.Mkfifo(path, 0600) != nil {
		t.Fatal("temp fifo")
	}
	if _, err := read(context.Background(), parts); err == nil {
		t.Fatal("fifo admitted")
	}
	if os.Remove(path) != nil || os.WriteFile(path, make([]byte, documentLimit+1), 0600) != nil {
		t.Fatal("temp oversized file")
	}
	if _, err := read(context.Background(), parts); err == nil {
		t.Fatal("oversized file admitted")
	}
	if os.Chmod(dir, 0770) != nil {
		t.Fatal("temp writable child")
	}
	if _, err := read(context.Background(), parts); err == nil {
		t.Fatal("writable child admitted")
	}
	if _, err := read(context.Background(), []protectedPart{{"..", 0, 0, 0}}); err == nil {
		t.Fatal("parent traversal admitted")
	}
}

func TestProtectedWorkerFileRejectsForeignIdentityAndACL(t *testing.T) {
	root := t.TempDir()
	if os.Chmod(root, 0700) != nil {
		t.Fatal("temp mode")
	}
	path := filepath.Join(root, "active.json")
	if os.WriteFile(path, []byte(activeFixture()), 0600) != nil {
		t.Fatal("temp record")
	}
	read := func(part protectedPart) error {
		fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatal("temp root open")
		}
		raw, err := readProtectedAt(context.Background(), fd, []protectedPart{part})
		clear(raw)
		return err
	}
	part := protectedPart{"active.json", uint32(os.Geteuid()) + 1, uint32(os.Getegid()), 0600}
	if read(part) == nil {
		t.Fatal("foreign UID admitted")
	}
	part.uid = uint32(os.Geteuid())
	part.gid++
	if read(part) == nil {
		t.Fatal("foreign GID admitted")
	}
	// Set a real named-user POSIX ACL (version2, owner/named/group/mask/other).
	acl := []byte{2, 0, 0, 0, 1, 0, 6, 0, 255, 255, 255, 255, 2, 0, 4, 0, 254, 255, 0, 0, 4, 0, 0, 0, 255, 255, 255, 255, 16, 0, 0, 0, 255, 255, 255, 255, 32, 0, 0, 0, 255, 255, 255, 255}
	if unix.Setxattr(path, "system.posix_acl_access", acl, 0) != nil {
		t.Fatal("test filesystem must support POSIX ACL")
	}
	part.gid = uint32(os.Getegid())
	if read(part) == nil {
		t.Fatal("extended ACL admitted")
	}
}
