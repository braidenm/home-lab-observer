//go:build linux

package connectedinstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedactivation"
)

// Only synthetic records in test-owned temporary directories; no installed path.
func TestActivationRecordBoundary(t *testing.T) {
	for _, scenario := range []string{"valid", "missing", "empty", "large", "mode", "group", "symlink", "hardlink"} {
		t.Run(scenario, func(t *testing.T) {
			path := t.TempDir()
			d, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			name := filepath.Join(path, "response.json")
			data := []byte("synthetic")
			if scenario == "empty" {
				data = nil
			}
			if scenario == "large" {
				data = []byte(strings.Repeat("x", connectedactivation.MaxBytes+1))
			}
			if scenario != "missing" {
				if err := os.WriteFile(name, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			group := uint32(os.Getgid())
			switch scenario {
			case "mode":
				if err := os.Chmod(name, 0644); err != nil {
					t.Fatal(err)
				}
			case "group":
				group++
			case "symlink":
				if err := os.Rename(name, filepath.Join(path, "original")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("original", name); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(name, filepath.Join(path, "alias")); err != nil {
					t.Fatal(err)
				}
			}
			got, err := activationRecord(d, "response.json", uint32(os.Getuid()), group, 0600)
			if scenario == "valid" {
				if err != nil || string(got) != "synthetic" {
					t.Fatal("valid record refused")
				}
			} else if err == nil {
				t.Fatal("unsafe record accepted")
			}
		})
	}
}
