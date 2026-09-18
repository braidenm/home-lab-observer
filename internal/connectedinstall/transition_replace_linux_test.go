//go:build linux

package connectedinstall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func requireOwnedTransitionReplacementFixture(t *testing.T) {
	t.Helper()
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		if os.Getenv("OBSERVER_CONNECTED_REPLACE_ACCEPTANCE") == "1" {
			t.Fatal("replacement acceptance fixture was not privileged")
		}
		t.Skip("root-owned synthetic replacement fixture")
	}
	var fs unix.Statfs_t
	if err := unix.Statfs(t.TempDir(), &fs); err != nil || fs.Type != unix.EXT4_SUPER_MAGIC {
		if os.Getenv("OBSERVER_CONNECTED_REPLACE_ACCEPTANCE") == "1" {
			t.Fatal("ext4 replacement acceptance fixture unavailable", err)
		}
		t.Skip("ext4 replacement fixture unavailable")
	}
}

func TestOwnedTransitionReplacementFixture(t *testing.T) {
	requireOwnedTransitionReplacementFixture(t)
	previous, next := []byte("old synthetic config\n"), []byte("next synthetic config\n")
	proposal := []byte("synthetic canonical proposal")
	sum := sha256.Sum256(proposal)
	temp := ".observer-transition-config-" + hex.EncodeToString(sum[:])
	for _, scenario := range []string{
		"replace", "already-next", "partial-temp", "file-sync-interruption",
		"rename-interruption", "parent-sync-interruption", "already-next-sync-interruption",
		"foreign-target", "linked-temp", "broad-temp",
		"foreign-temp", "hardlink-temp", "oversized-temp", "broad-target",
		"symlink-target", "canceled", "unknown-role",
	} {
		t.Run(scenario, func(t *testing.T) {
			path := t.TempDir()
			target := filepath.Join(path, "installed.json")
			initial := previous
			if scenario == "already-next" || scenario == "already-next-sync-interruption" {
				initial = next
			}
			if scenario == "foreign-target" {
				initial = []byte("foreign config\n")
			}
			if scenario == "symlink-target" {
				if err := os.WriteFile(filepath.Join(path, "other"), initial, 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("other", target); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(target, initial, 0644); err != nil {
				t.Fatal(err)
			}
			if scenario == "broad-target" && os.Chmod(target, 0666) != nil {
				t.Fatal("synthetic mode drift unavailable")
			}
			if scenario == "partial-temp" {
				if err := os.WriteFile(filepath.Join(path, temp), []byte("partial"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "broad-temp" || scenario == "foreign-temp" || scenario == "hardlink-temp" || scenario == "oversized-temp" {
				data := []byte("partial")
				if scenario == "oversized-temp" {
					data = bytes.Repeat([]byte("x"), 4097)
				}
				if err := os.WriteFile(filepath.Join(path, temp), data, 0644); err != nil {
					t.Fatal(err)
				}
				switch scenario {
				case "broad-temp":
					if err := os.Chmod(filepath.Join(path, temp), 0666); err != nil {
						t.Fatal(err)
					}
				case "foreign-temp":
					if err := os.Chown(filepath.Join(path, temp), 65534, 0); err != nil {
						t.Fatal(err)
					}
				case "hardlink-temp":
					if err := os.Link(filepath.Join(path, temp), filepath.Join(path, "second-link")); err != nil {
						t.Fatal(err)
					}
				}
			}
			if scenario == "linked-temp" {
				if err := os.Symlink("missing", filepath.Join(path, temp)); err != nil {
					t.Fatal(err)
				}
			}
			parent, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer parent.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "canceled" {
				cancel()
			}
			role := "config"
			if scenario == "unknown-role" {
				role = "shell"
			}
			var hook func(string) error
			if scenario == "file-sync-interruption" || scenario == "rename-interruption" ||
				scenario == "parent-sync-interruption" || scenario == "already-next-sync-interruption" {
				step := "file-sync"
				if scenario == "rename-interruption" {
					step = "rename"
				} else if scenario == "parent-sync-interruption" || scenario == "already-next-sync-interruption" {
					step = "parent-sync"
				}
				hook = func(at string) error {
					if at == step {
						return ErrRecovery
					}
					return nil
				}
			}
			err = replaceTransitionAt(ctx, parent, "installed.json", role, proposal, previous, next, 0644, 4096, hook)
			switch scenario {
			case "replace", "already-next", "partial-temp":
				if err != nil {
					t.Fatal("recognized replacement refused", err)
				}
			case "file-sync-interruption", "rename-interruption", "parent-sync-interruption", "already-next-sync-interruption":
				if err != ErrRecovery {
					t.Fatal("interrupted replacement looked complete")
				}
				if err := replaceTransitionAt(context.Background(), parent, "installed.json", "config", proposal, previous, next, 0644, 4096, nil); err != nil {
					t.Fatal("exact forward retry refused", err)
				}
			case "canceled", "unknown-role":
				if err != ErrUnsafe {
					t.Fatal("invalid replacement request accepted")
				}
			default:
				if err != ErrRecovery {
					t.Fatal("foreign replacement evidence accepted")
				}
				if scenario == "linked-temp" || scenario == "broad-temp" || scenario == "foreign-temp" || scenario == "hardlink-temp" || scenario == "oversized-temp" {
					if _, statErr := os.Lstat(filepath.Join(path, temp)); statErr != nil {
						t.Fatal("foreign temporary evidence was removed", statErr)
					}
				}
			}
			if scenario == "replace" || scenario == "already-next" || scenario == "partial-temp" ||
				scenario == "file-sync-interruption" || scenario == "rename-interruption" ||
				scenario == "parent-sync-interruption" || scenario == "already-next-sync-interruption" {
				data, readErr := os.ReadFile(target)
				if readErr != nil || !bytes.Equal(data, next) {
					t.Fatal("next bytes not authoritative after successful return", readErr)
				}
				if _, statErr := os.Lstat(filepath.Join(path, temp)); !os.IsNotExist(statErr) {
					t.Fatal("recognized temporary residue remained")
				}
			}
		})
	}
}
