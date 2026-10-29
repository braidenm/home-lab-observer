//go:build linux

package sharedhandoff

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFailureDiagnosticsAreClosed(t *testing.T) {
	if readFixtureFailure(errors.New("synthetic-private-detail")) != "UNKNOWN" {
		t.Fatal("dependency error escaped")
	}
	for _, code := range []int{0, 1, 80, 86, 90, 94, 255} {
		if readFixtureExit(code) != "UNKNOWN" {
			t.Fatal("unknown exit classified")
		}
	}
	for code, want := range map[int]string{81: "UNSAFE", 82: "INVALID", 83: "MISSING", 84: "UNEXPECTED_ERROR", 85: "INVALID_RETURNED_DOCUMENT", 91: "OPEN_UNAVAILABLE", 92: "OPEN_UNSAFE", 93: "OPEN_OTHER"} {
		if readFixtureExit(code) != want {
			t.Fatal("finite exit classification changed")
		}
	}
}

func capturedStage(t *testing.T) (*slot, []os.DirEntry, string) {
	t.Helper()
	path := t.TempDir()
	for name, mode := range map[string]os.FileMode{latestName: 0640, stageName: 0600, lockName: 0600} {
		p := filepath.Join(path, name)
		if os.WriteFile(p, nil, mode) != nil || os.Chmod(p, mode) != nil {
			t.Fatal("owned directory fixture setup failed")
		}
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal("owned fixture root failed")
	}
	t.Cleanup(func() { root.Close() })
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal("owned fixture enumeration failed")
	}
	return &slot{root: root, policy: Policy{CollectorUID: uint32(os.Geteuid()), SharedGID: uint32(os.Getegid())}}, entries, path
}

func TestCapturedPublicationStageMayDisappearBeforeValidation(t *testing.T) {
	s, entries, path := capturedStage(t)
	if s.checkEntries(entries) != nil {
		t.Fatal("safe captured entries rejected")
	}
	if os.Remove(filepath.Join(path, stageName)) != nil {
		t.Fatal("owned stage removal failed")
	}
	if s.checkEntries(entries) != ErrUnavailable {
		t.Fatal("disappearing stage was not transient")
	}
	if os.Chmod(filepath.Join(path, latestName), 0660) != nil {
		t.Fatal("owned mode setup failed")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal("owned fixture enumeration failed")
	}
	if s.checkEntries(entries) != ErrUnsafe {
		t.Fatal("unsafe metadata became transient")
	}
}

func TestConcurrentFixtureConstructsReaderBeforeWriterMutation(t *testing.T) {
	s, entries, path := capturedStage(t)
	released := false
	err := beginConcurrent(func() error { return s.checkEntries(entries) }, func() error { return os.Remove(filepath.Join(path, stageName)) }, func() error { released = true; return nil })
	if err != nil || !released {
		t.Fatal("writer mutation raced reader construction")
	}
}
