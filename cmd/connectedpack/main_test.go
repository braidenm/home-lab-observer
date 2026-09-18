package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClosedBuildToolOptions(t *testing.T) {
	for _, args := range [][]string{nil, {"--mode", "other"}, {"--unknown"}, {"--mode", "identity", "--role", "observer", "--version", "0.1.0-canary.1", "--commit", strings.Repeat("a", 40)}, {"--mode", "pack", "--version", "0.1.0-canary.1", "--commit", strings.Repeat("a", 40)}} {
		if run(args) == nil {
			t.Fatal("invalid options accepted")
		}
	}
	input := t.TempDir()
	output := filepath.Join(t.TempDir(), "out")
	if run([]string{"--mode", "pack", "--version", "0.1.0-canary.1", "--commit", strings.Repeat("a", 40), "--binaries", input, "--output", output}) == nil {
		t.Fatal("missing binaries accepted")
	}
	if _, err := os.Stat(filepath.Join(output, "SHA256SUMS")); !os.IsNotExist(err) {
		t.Fatal("published incomplete checksums")
	}
}
