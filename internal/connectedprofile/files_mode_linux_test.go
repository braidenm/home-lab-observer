//go:build linux

package connectedprofile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnedExactModeReaderFixture(t *testing.T) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		if os.Getenv("OBSERVER_CONNECTED_ACTIVE_MODE_ACCEPTANCE") == "1" {
			t.Fatal("active-mode acceptance fixture was not privileged")
		}
		t.Skip("root-owned active-mode fixture")
	}
	parent, err := os.MkdirTemp("/var/lib", "observer-active-mode-")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "installed.json")
	alias := filepath.Join(parent, "alias")
	t.Cleanup(func() {
		os.Remove(alias)
		os.Remove(path)
		if err := os.Remove(parent); err != nil {
			t.Error("synthetic fixture cleanup failed", err)
		}
	})
	want := []byte("synthetic fixed resource\n")
	if err := os.WriteFile(path, want, 0644); err != nil || os.Chmod(path, 0644) != nil {
		t.Fatal("synthetic fixed resource unavailable", err)
	}
	got, err := ReadRootFileMode(path, 4096, 0644)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("exact root-owned mode refused", err)
	}
	for _, mode := range []os.FileMode{0, 0666, 0755, 0600} {
		if _, err := ReadRootFileMode(path, 4096, mode); err != ErrUnsafe {
			t.Fatal("wrong requested mode accepted", mode)
		}
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRootFileMode(path, 4096, 0644); err != ErrUnsafe {
		t.Fatal("installed mode drift accepted")
	}
	if got, err := ReadRootFileMode(path, 4096, 0600); err != nil || !bytes.Equal(got, want) {
		t.Fatal("exact private mode refused", err)
	}
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRootFileMode(path, 4096, 0600); err != ErrUnsafe {
		t.Fatal("linked active resource accepted")
	}
}
