//go:build linux || darwin

package handoff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnsafeExistingPermissionsAreNotRepaired(t *testing.T) {
	dir := privateDirectory(t)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(dir, true); err != ErrUnsafe {
		if s != nil {
			s.Close()
		}
		t.Fatal("public directory accepted")
	}
	info, _ := os.Stat(dir)
	if info.Mode().Perm() != 0o755 {
		t.Fatal("existing permissions changed")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	s := openStore(t, dir, false)
	path := filepath.Join(dir, latestName)
	if err := os.WriteFile(path, []byte("synthetic"), 0o644); err != nil {
		t.Fatal(err)
	}
	if data, err := s.Read(serverID); data != nil || err != ErrUnsafe {
		t.Fatal("public snapshot accepted")
	}
}
