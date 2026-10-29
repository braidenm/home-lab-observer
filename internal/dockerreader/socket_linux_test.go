//go:build linux

package dockerreader

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnedSocketWitnessRejectsReplacementAndPermissionDrift(t *testing.T) {
	if os.Getenv("OBSERVER_SHARED_ACCEPTANCE") != "1" {
		t.Skip("owned root socket fixture not requested")
	}
	if os.Geteuid() != 0 {
		t.Fatal("root orchestrator required")
	}
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err != nil || resolved != dir || !strings.HasPrefix(dir, "/tmp/") {
		t.Fatal("native private fixture required")
	}
	path := filepath.Join(dir, "engine.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal("synthetic socket creation")
	}
	defer listener.Close()
	if os.Chown(path, 0, 61011) != nil || os.Chmod(path, 0660) != nil {
		t.Fatal("synthetic socket ownership")
	}
	witness, err := captureSocketAt(path)
	if err != nil || witness.checkAt(path) != nil {
		t.Fatal("approved socket rejected")
	}
	if os.Chmod(path, 0666) != nil {
		t.Fatal("permission fixture")
	}
	if witness.checkAt(path) == nil {
		t.Fatal("permission drift accepted")
	}
	if os.Chmod(path, 0660) != nil {
		t.Fatal("permission cleanup")
	}
	link := filepath.Join(dir, "alias")
	if os.Link(path, link) != nil {
		t.Fatal("hardlink fixture")
	}
	if witness.checkAt(path) == nil {
		t.Fatal("socket hardlink accepted")
	}
	if os.Remove(link) != nil {
		t.Fatal("hardlink cleanup")
	}
	if os.Chown(path, 0, 0) != nil {
		t.Fatal("root group fixture")
	}
	if witness.checkAt(path) == nil {
		t.Fatal("root group accepted")
	}
	if os.Chown(path, 0, 61011) != nil || os.Rename(path, filepath.Join(dir, "retained.sock")) != nil {
		t.Fatal("replacement fixture")
	}
	replacement, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal("replacement socket")
	}
	defer replacement.Close()
	if os.Chown(path, 0, 61011) != nil || os.Chmod(path, 0660) != nil {
		t.Fatal("replacement ownership")
	}
	if witness.checkAt(path) == nil {
		t.Fatal("replacement adopted")
	}
	if (socketWitness{}).checkAt(path) == nil {
		t.Fatal("zero witness accepted")
	}
	if os.Symlink(path, link) != nil {
		t.Fatal("symlink fixture")
	}
	if _, err := captureSocketAt(link); err == nil {
		t.Fatal("symlink accepted")
	}
}
