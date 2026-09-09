package localauth

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsureCreatesAndReusesPrivateToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "local-api.token")
	first, created, err := Ensure(path)
	if err != nil || !created || first == "" {
		t.Fatalf("first Ensure() token=%q created=%v err=%v", first, created, err)
	}
	second, created, err := Ensure(path)
	if err != nil || created || second != first {
		t.Fatalf("second Ensure() token_match=%v created=%v err=%v", second == first, created, err)
	}
	loaded, err := Load(path)
	if err != nil || loaded != first {
		t.Fatalf("Load() token_match=%v err=%v", loaded == first, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("token permissions=%v err=%v", info.Mode().Perm(), err)
		}
	}
}

func TestLoadDoesNotCreateMissingToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "local-api.token")
	if _, err := Load(path); err == nil {
		t.Fatal("Load() accepted a missing token")
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load() changed missing token path: %v", err)
	}
}

func TestLoadRejectsUnsafeTokenPathsWithoutRepairingPermissions(t *testing.T) {
	t.Run("hard link", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "state", "local-api.token")
		if _, _, err := Ensure(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(path, filepath.Join(filepath.Dir(path), "token-copy")); err != nil {
			t.Skipf("hard links unavailable: %v", err)
		}
		if _, err := Load(path); !errors.Is(err, ErrInvalidTokenFile) {
			t.Fatalf("Load() hard-link error=%v", err)
		}
	})

	t.Run("linked ancestor", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "actual", "state", "local-api.token")
		if _, _, err := Ensure(path); err != nil {
			t.Fatal(err)
		}
		linkedParent := filepath.Join(root, "linked")
		if err := os.Symlink(filepath.Join(root, "actual"), linkedParent); err != nil {
			t.Skipf("directory links unavailable: %v", err)
		}
		if _, err := Load(filepath.Join(linkedParent, "state", "local-api.token")); !errors.Is(err, ErrInvalidTokenFile) {
			t.Fatalf("Load() linked-ancestor error=%v", err)
		}
	})

	if runtime.GOOS != "windows" {
		t.Run("weak permissions", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state", "local-api.token")
			if _, _, err := Ensure(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); !errors.Is(err, ErrInvalidTokenFile) {
				t.Fatalf("Load() weak-mode error=%v", err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o644 {
				t.Fatalf("Load() repaired permissions during status probe: mode=%v", info.Mode().Perm())
			}
		})
	}
}

func TestEnsureRejectsInvalidAndSymlinkTokenFiles(t *testing.T) {
	directory := t.TempDir()
	invalid := filepath.Join(directory, "invalid.token")
	if err := os.WriteFile(invalid, []byte("not-a-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Ensure(invalid); !errors.Is(err, ErrInvalidTokenFile) {
		t.Fatalf("expected invalid token error, got %v", err)
	}

	if runtime.GOOS == "windows" {
		return
	}
	target := filepath.Join(directory, "target.token")
	if _, _, err := Ensure(target); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "link.token")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Ensure(link); !errors.Is(err, ErrInvalidTokenFile) {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func TestMatchesAuthorization(t *testing.T) {
	token, _, err := Ensure(filepath.Join(t.TempDir(), "token"))
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", token, "Basic " + token, "Bearer", "Bearer wrong", "Bearer " + token + " ", "Bearer " + token + "\n"} {
		if MatchesAuthorization(value, token) {
			t.Fatalf("unexpected match for %q", value)
		}
	}
	if !MatchesAuthorization("Bearer "+token, token) || !MatchesAuthorization("bearer "+token, token) {
		t.Fatal("valid bearer token did not match")
	}
}
