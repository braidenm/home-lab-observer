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

	"github.com/braidenm/home-lab-observer/internal/buildidentity"
	"github.com/braidenm/home-lab-observer/internal/releasepack"
)

// Actual Go artifacts and releasepack archives, never target execution or native reads.
// Export is optional so an already-built Linux test binary can stage the same artifacts under WSL without a Linux Go installation.
func TestStageRealV2GoArtifacts(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("Go compiler unavailable; run exported-artifact test instead")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(errProof)
	}
	temporary := t.TempDir()
	binaries := filepath.Join(temporary, "binaries")
	if os.Mkdir(binaries, 0700) != nil {
		t.Fatal(errProof)
	}
	version, commit := "0.0.0-staging-test", strings.Repeat("a", 40)
	build := func(name, goos, arch, role, digest string) {
		t.Helper()
		record, err := buildidentity.Encode(buildidentity.Identity{Role: role, Version: version, Commit: commit, OS: goos, Arch: arch, HelperSHA256: digest})
		if err != nil {
			t.Fatal(errProof)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, "go", "build", "-trimpath", "-ldflags", "-s -w -X main.releaseIdentity="+record, "-o", filepath.Join(binaries, name), "./internal/releasepack/testdata/main.go")
		command.Dir = root
		for _, value := range os.Environ() {
			key, _, _ := strings.Cut(value, "=")
			if !strings.HasPrefix(key, "GO") && key != "CGO_ENABLED" {
				command.Env = append(command.Env, value)
			}
		}
		command.Env = append(command.Env, "GOOS="+goos, "GOARCH="+arch, "CGO_ENABLED=0", "GOENV=off", "GOWORK=off", "GOFLAGS=-buildvcs=false", "GOTOOLCHAIN=local")
		if command.Run() != nil {
			t.Fatal("synthetic Go artifact build failed")
		}
	}
	for _, goos := range []string{"linux", "darwin", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			digest := ""
			if goos == "linux" {
				name := "observer-journal-helper_linux_" + arch
				build(name, goos, arch, "journal-helper", "")
				data, err := os.ReadFile(filepath.Join(binaries, name))
				if err != nil {
					t.Fatal(errProof)
				}
				sum := sha256.Sum256(data)
				digest = hex.EncodeToString(sum[:])
			}
			name := "observer_" + goos + "_" + arch
			if goos == "windows" {
				name += ".exe"
			}
			build(name, goos, arch, "observer", digest)
		}
	}
	output := filepath.Join(temporary, "release")
	if exported := os.Getenv("OBSERVER_TEST_STAGE_EXPORT"); exported != "" {
		output = exported
		if _, err := os.Lstat(output); !os.IsNotExist(err) {
			t.Fatal("export path must be new")
		}
	}
	_, err = releasepack.Build(releasepack.Config{SchemaVersion: releasepack.SchemaVersionV2, Version: version, Commit: commit, BinariesDir: binaries, OutputDir: output, ResourcesDir: filepath.Join(root, "packaging", "resources")})
	if err != nil {
		t.Fatal("real v2 releasepack build failed")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		if stageLinux(output, filepath.Join(temporary, "staged-"+arch), arch) != nil {
			t.Fatal("real v2 stage rejected")
		}
	}
}

func TestStageExportedV2(t *testing.T) {
	source := os.Getenv("OBSERVER_TEST_STAGE_INPUT")
	if source == "" {
		t.Skip("explicit previously built v2 fixture required")
	}
	if runtime.GOOS != "linux" {
		t.Skip("native Linux staging proof")
	}
	destination := filepath.Join(t.TempDir(), "context")
	if stage(source, destination) != nil {
		t.Fatal("native Linux real v2 stage rejected")
	}
	entries, err := os.ReadDir(filepath.Join(destination, "package"))
	if err != nil || len(entries) != 5 {
		t.Fatal("incorrect staged package")
	}
}
