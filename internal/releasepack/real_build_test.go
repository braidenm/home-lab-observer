package releasepack

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/buildidentity"
)

// Compile synthetic, zero-native-access programs; never execute their output.
// This proves the actual Go build-info representation instead of only fake DTOs.
func TestV2BuildRealGoMetadataWithoutExecutingArtifacts(t *testing.T) {
	config := testConfig(t)
	config.SchemaVersion = SchemaVersionV2
	build := func(name, goos, arch, digest string) {
		t.Helper()
		if err := os.Remove(filepath.Join(config.BinariesDir, name)); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		flags := "-s -w -X main.version=" + config.Version + " -X main.commit=" + config.Commit
		role := "observer"
		if strings.HasPrefix(name, "observer-journal-helper_") {
			role = "journal-helper"
		}
		record, err := buildidentity.Encode(buildidentity.Identity{Role: role, Version: config.Version, Commit: config.Commit, OS: goos, Arch: arch, HelperSHA256: digest})
		if err != nil {
			t.Fatal(err)
		}
		flags += " -X main.releaseIdentity=" + record
		if digest != "" {
			flags += " -X main.journalHelperSHA256=" + digest
		}
		command := exec.Command("go", "build", "-trimpath", "-ldflags", flags, "-o", filepath.Join(config.BinariesDir, name), "./testdata/main.go")
		for _, value := range os.Environ() {
			key, _, _ := strings.Cut(value, "=")
			if key != "GOOS" && key != "GOARCH" && key != "CGO_ENABLED" && key != "GOFLAGS" {
				command.Env = append(command.Env, value)
			}
		}
		command.Env = append(command.Env, "GOOS="+goos, "GOARCH="+arch, "CGO_ENABLED=0", "GOFLAGS=")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("synthetic compiler failed: %v %s", err, output)
		}
	}
	for _, target := range targets {
		digest := ""
		if target.os == "linux" {
			name := journalInputName(target.arch)
			build(name, target.os, target.arch, "")
			var err error
			digest, _, err = hashFileBounded(filepath.Join(config.BinariesDir, name), maxBinarySize)
			if err != nil {
				t.Fatal(err)
			}
		}
		build(target.binaryName, target.os, target.arch, digest)
	}
	manifest, err := Build(config)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != SchemaVersionV2 || Verify(config.OutputDir) != nil {
		t.Fatal("real Go v2 package did not verify")
	}
	config.OutputDir = filepath.Join(filepath.Dir(config.OutputDir), "wrong-pair")
	build(targets[0].binaryName, "linux", "amd64", strings.Repeat("a", 64))
	if _, err := Build(config); err == nil {
		t.Fatal("mixed real helper pair accepted")
	}
	if _, err := os.Lstat(config.OutputDir); !os.IsNotExist(err) {
		t.Fatal("rejected identity created release output")
	}
}
