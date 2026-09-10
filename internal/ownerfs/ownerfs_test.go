package ownerfs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsurePrivateSubdirCreatesOnlyFixedChild(t *testing.T) {
	state := filepath.Join(canonicalTempDir(t), "missing", "state")
	directory, err := EnsurePrivateSubdir(state, "diagnostics")
	if err != nil {
		t.Fatal(err)
	}
	if directory != filepath.Join(state, "diagnostics") {
		t.Fatalf("directory = %q", directory)
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("private directory is unsafe: %v, %v", info, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %o", info.Mode().Perm())
	}
}

func TestEnsurePrivateSubdirDoesNotMutateExistingUnknownChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits do not describe Windows ACL mutation")
	}
	state := testStateDir(t)
	directory := filepath.Join(state, "diagnostics")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "unknown"), []byte("synthetic-canary"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsurePrivateSubdir(state, "diagnostics"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("existing unknown child mode mutated to %o", info.Mode().Perm())
	}
}

func TestEnsurePrivateSubdirRejectsLinkedAncestorBeforeMutation(t *testing.T) {
	root := canonicalTempDir(t)
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := EnsurePrivateSubdir(filepath.Join(link, "missing"), "diagnostics")
	if !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("error = %v, want ErrUnsafePath", err)
	}
	if _, err := os.Lstat(filepath.Join(target, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("linked target was mutated: %v", err)
	}
}

func TestValidateRegularRejectsHardLink(t *testing.T) {
	root := canonicalTempDir(t)
	source := filepath.Join(root, "source")
	alias := filepath.Join(root, "alias")
	if err := os.WriteFile(source, []byte("synthetic-canary"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(source, alias); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if _, err := ValidateRegular(alias, 1024); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("error = %v, want ErrUnsafePath", err)
	}
}

func testStateDir(t *testing.T) string {
	t.Helper()
	state := filepath.Join(canonicalTempDir(t), "state")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	return state
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	canonical, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}
