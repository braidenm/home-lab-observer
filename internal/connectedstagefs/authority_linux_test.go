//go:build linux

package connectedstagefs

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnedTransitionStageAuthorityFixture(t *testing.T) {
	requireOwnedTransitionStageFixture(t)
	for _, scenario := range []string{
		"absent", "journal", "completion", "both", "empty", "broad",
		"symlink", "hardlink", "foreign-owner", "oversized",
	} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			if err := os.Mkdir(path, 0755); err != nil {
				t.Fatal(err)
			}
			journal := filepath.Join(path, JournalName)
			completion := filepath.Join(path, CompletionName)
			write := func(name string, data []byte) {
				t.Helper()
				if err := os.WriteFile(name, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "journal", "both", "empty", "broad", "hardlink", "foreign-owner", "oversized":
				write(journal, []byte("synthetic journal"))
			}
			switch scenario {
			case "completion", "both":
				write(completion, []byte("synthetic completion"))
			case "empty":
				if err := os.WriteFile(completion, nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "broad":
				write(completion, []byte("synthetic completion"))
				if err := os.Chmod(completion, 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				write(filepath.Join(path, "target"), []byte("synthetic completion"))
				if err := os.Symlink("target", completion); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(journal, completion); err != nil {
					t.Fatal(err)
				}
			case "foreign-owner":
				if err := os.Chown(journal, 65534, 0); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				write(completion, bytes.Repeat([]byte("x"), 257))
			}
			directory, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer directory.Close()
			got, err := ReadAuthorityAt(directory)
			switch scenario {
			case "absent":
				if err != nil || got.Journal != nil || got.Completion != nil {
					t.Fatal("absent authority invented")
				}
			case "journal":
				if err != nil || string(got.Journal) != "synthetic journal" || got.Completion != nil {
					t.Fatal("journal-only evidence lost")
				}
			case "completion":
				if err != nil || got.Journal != nil || string(got.Completion) != "synthetic completion" {
					t.Fatal("receipt-only evidence lost")
				}
			case "both":
				if err != nil || string(got.Journal) != "synthetic journal" || string(got.Completion) != "synthetic completion" {
					t.Fatal("closed authority evidence lost")
				}
			default:
				if err != ErrRecovery || got.Journal != nil || got.Completion != nil {
					t.Fatal("unsafe authority accepted")
				}
			}
		})
	}
}
