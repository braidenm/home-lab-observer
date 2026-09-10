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
	asset := manifest.Assets[0]
	extracted := t.TempDir()
	for name, source := range map[string]string{"observer": filepath.Join(config.BinariesDir, targets[0].binaryName), "observer-journal-helper": filepath.Join(config.BinariesDir, journalInputName("amd64")), "LICENSE": filepath.Join(config.ResourcesDir, "LICENSE"), "START-HERE.md": filepath.Join(config.ResourcesDir, "START-HERE.md"), "run-observer.sh": filepath.Join(config.ResourcesDir, "run-observer.sh")} {
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(extracted, name), data, 0700); err != nil {
			t.Fatal(err)
		}
	}
	installed := Installed{ExpectedSchemaVersion: SchemaVersionV2, ManifestPath: filepath.Join(config.OutputDir, ManifestName), ExecutablePath: filepath.Join(extracted, "observer"), Identity: buildidentity.Identity{Role: "observer", Version: config.Version, Commit: config.Commit, OS: "linux", Arch: "amd64", HelperSHA256: asset.JournalHelper.SHA256}, ArchiveSHA256: asset.SHA256, ArchiveSize: asset.SizeBytes}
	if err := VerifyInstalled(installed); err != nil {
		t.Fatal(err)
	}
	wrong := installed
	wrong.ArchiveSize++
	if VerifyInstalled(wrong) == nil {
		t.Fatal("wrong archive size accepted")
	}
	wrong = installed
	wrong.Identity.Commit = strings.Repeat("b", 40)
	if VerifyInstalled(wrong) == nil {
		t.Fatal("wrong installed commit accepted")
	}
	wrong = installed
	wrong.Identity.HelperSHA256 = strings.Repeat("c", 64)
	if VerifyInstalled(wrong) == nil {
		t.Fatal("wrong runtime pair accepted")
	}
	if err := os.WriteFile(filepath.Join(extracted, "observer-journal-helper"), []byte("tampered"), 0700); err != nil {
		t.Fatal(err)
	}
	if VerifyInstalled(installed) == nil {
		t.Fatal("tampered adjacent helper accepted")
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
