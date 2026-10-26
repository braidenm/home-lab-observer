package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedbootstrap"
	"github.com/braidenm/home-lab-observer/internal/connectedbundle"
	"github.com/braidenm/home-lab-observer/internal/connectedcompat"
	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
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

// One pack journey verifies archive/manifest/helper/checksum wiring together.
// Tiny cross-built role fixtures carry real Go ELF identities but never run.
func TestPackEmitsVerifiedHelperAndFinalChecksums(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("connected packer requires POSIX filesystem admission")
	}
	input, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "go.mod"), []byte("module github.com/braidenm/home-lab-observer\n\ngo 1.27.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const version = "0.1.0-canary.1"
	commit := strings.Repeat("a", 40)
	for _, role := range []string{"collector", "uploader", "install"} {
		name := "observer-connected-" + role
		directory := filepath.Join(source, "cmd", name)
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "main.go"), []byte("package main\nimport \"fmt\"\nvar releaseIdentity string\nfunc main(){fmt.Print(releaseIdentity)}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		record, err := connectedidentity.Encode(connectedidentity.Identity{Role: role, Version: version, Commit: commit, OS: "linux", Arch: "amd64", ContractSHA256: connectedcompat.Digest()})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		command := exec.CommandContext(ctx, "go", "build", "-trimpath", "-ldflags=-s -w -X main.releaseIdentity="+record, "-o", filepath.Join(input, name), "./cmd/"+name)
		command.Dir = source
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "GOOS=") && !strings.HasPrefix(entry, "GOARCH=") && !strings.HasPrefix(entry, "CGO_ENABLED=") && !strings.HasPrefix(entry, "GOTOOLCHAIN=") {
				command.Env = append(command.Env, entry)
			}
		}
		command.Env = append(command.Env, "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0", "GOTOOLCHAIN=local")
		result, err := command.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("fixture build: %v %s", err, result)
		}
	}
	outputParent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(outputParent, "release")
	if err := run([]string{"--mode", "pack", "--version", version, "--commit", commit, "--binaries", input, "--output", output}); err != nil {
		t.Fatal(err)
	}
	read := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join(output, name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	archiveName := "home-lab-observer-connected_" + version + "_linux_amd64.tar.gz"
	archive := read(archiveName)
	manifest := read(connectedbundle.ManifestName)
	helper := read("install-connected-linux.sh")
	archiveSum := sha256.Sum256(archive)
	expected, err := connectedbootstrap.Render(manifest, hex.EncodeToString(archiveSum[:]), int64(len(archive)))
	if err != nil || string(helper) != string(expected) {
		t.Fatal("helper not pinned to emitted archive/manifest")
	}
	checksums := string(read("SHA256SUMS"))
	lines := strings.Split(strings.TrimSuffix(checksums, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatal("unexpected checksum membership")
	}
	for index, name := range []string{archiveName, connectedbundle.ManifestName, "install-connected-linux.sh"} {
		sum := sha256.Sum256(read(name))
		if lines[index] != hex.EncodeToString(sum[:])+"  "+name {
			t.Fatal("checksum does not match published asset")
		}
	}
}
