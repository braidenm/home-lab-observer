//go:build linux

package connectedstagefs

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnedTransitionStageLegacyFixture(t *testing.T) {
	if _, err := ReadLegacyAt(nil); err != ErrUnsafe {
		t.Fatal("missing anchored config accepted")
	}
	requireOwnedTransitionStageFixture(t)
	for _, scenario := range []string{
		"absent", "both", "journal-only", "completion-only", "empty",
		"broad", "symlink", "hardlink", "foreign-owner", "oversized",
	} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			if err := os.Mkdir(path, 0755); err != nil {
				t.Fatal(err)
			}
			journal := filepath.Join(path, LegacyJournalName)
			completion := filepath.Join(path, LegacyCompletionName)
			write := func(name string, data []byte) {
				t.Helper()
				if err := os.WriteFile(name, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "both", "journal-only", "hardlink", "foreign-owner", "oversized":
				write(journal, []byte("synthetic legacy journal"))
			}
			switch scenario {
			case "both", "completion-only":
				write(completion, []byte("synthetic legacy receipt"))
			case "empty":
				write(completion, nil)
			case "broad":
				write(completion, []byte("synthetic legacy receipt"))
				if err := os.Chmod(completion, 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				write(filepath.Join(path, "target"), []byte("synthetic legacy receipt"))
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
				write(journal, bytes.Repeat([]byte("x"), 8193))
			}
			directory, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer directory.Close()
			got, err := ReadLegacyAt(directory)
			switch scenario {
			case "absent":
				if err != nil || got.Journal != nil || got.Completion != nil {
					t.Fatal("absent legacy names invented", err)
				}
			case "both":
				if err != nil || string(got.Journal) != "synthetic legacy journal" || string(got.Completion) != "synthetic legacy receipt" {
					t.Fatal("fixed legacy evidence lost", err)
				}
			case "journal-only":
				if err != nil || got.Journal == nil || got.Completion != nil {
					t.Fatal("partial legacy evidence hidden", err)
				}
			case "completion-only":
				if err != nil || got.Journal != nil || got.Completion == nil {
					t.Fatal("partial legacy evidence hidden", err)
				}
			default:
				if err != ErrRecovery || got.Journal != nil || got.Completion != nil {
					t.Fatal("unsafe legacy evidence accepted", err)
				}
			}
		})
	}
}
