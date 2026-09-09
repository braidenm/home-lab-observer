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
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("token permissions=%v err=%v", info.Mode().Perm(), err)
		}
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
