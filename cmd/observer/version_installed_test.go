package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/buildidentity"
	"github.com/braidenm/home-lab-observer/internal/releasepack"
)

// Execute only the real static version verifier in a temporary extraction root.
// No collection, network, helper launch or managed host installation occurs.
func TestRealInstalledVersionVerifier(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("release targets only")
	}
	directory := t.TempDir()
	metadata := t.TempDir()
	identity := buildidentity.Identity{Role: "observer", Version: "0.1.0-preview.99", Commit: strings.Repeat("a", 40), OS: runtime.GOOS, Arch: runtime.GOARCH}
	compile := func(path, source string, i buildidentity.Identity) {
		t.Helper()
		record, err := buildidentity.Encode(i)
		if err != nil {
			t.Fatal(err)
		}
		command := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X main.releaseIdentity="+record, "-o", path, source)
		for _, entry := range os.Environ() {
			key, _, _ := strings.Cut(entry, "=")
			if key != "CGO_ENABLED" && key != "GOFLAGS" && key != "GOOS" && key != "GOARCH" {
				command.Env = append(command.Env, entry)
			}
		}
		command.Env = append(command.Env, "CGO_ENABLED=0", "GOFLAGS=", "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("static fixture build failed: %v %s", err, output)
		}
	}
	var helper *releasepack.JournalHelper
	if runtime.GOOS == "linux" {
		i := identity
		i.Role = "journal-helper"
		path := filepath.Join(directory, "observer-journal-helper")
		compile(path, "../../internal/releasepack/testdata/main.go", i)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(data)
		identity.HelperSHA256 = hex.EncodeToString(hash[:])
		helper = &releasepack.JournalHelper{Filename: "observer-journal-helper", SHA256: identity.HelperSHA256, SizeBytes: int64(len(data))}
	}
	binary, launcher := "observer", "run-observer.sh"
	if runtime.GOOS == "windows" {
		binary = "observer.exe"
		launcher = "Run-Observer.cmd"
	}
	if runtime.GOOS == "darwin" {
		launcher = "Run-Observer.command"
	}
	compile(filepath.Join(directory, binary), ".", identity)
	for _, name := range []string{"LICENSE", "START-HERE.md", launcher} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("synthetic metadata"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	manifest := releasepack.Manifest{SchemaVersion: releasepack.SchemaVersionV2, Version: identity.Version, Tag: "v" + identity.Version, Repository: releasepack.Repository, CommitSHA: identity.Commit, SigningPolicy: releasepack.SigningPolicy}
	for _, goos := range []string{"linux", "darwin", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			format := "tar.gz"
			if goos == "windows" {
				format = "zip"
			}
			filename := "home-lab-observer_" + identity.Version + "_" + goos + "_" + arch + "." + format
			asset := releasepack.Asset{OS: goos, Arch: arch, Filename: filename, SHA256: strings.Repeat("c", 64), SizeBytes: 100, Format: format, DownloadURL: "https://github.com/" + releasepack.Repository + "/releases/download/" + manifest.Tag + "/" + filename, ContentProfile: "native-core-v1"}
			if goos == "linux" {
				asset.ContentProfile = "linux-journal-helper-v1"
				asset.JournalHelper = &releasepack.JournalHelper{Filename: "observer-journal-helper", SHA256: strings.Repeat("b", 64), SizeBytes: 1}
				if goos == runtime.GOOS && arch == runtime.GOARCH {
					asset.JournalHelper = helper
				}
			}
			manifest.Assets = append(manifest.Assets, asset)
		}
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(metadata, releasepack.ManifestName)
	if err := os.WriteFile(manifestPath, payload, 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(filepath.Join(directory, binary), "version", "--release-manifest", manifestPath, "--archive-sha256", strings.Repeat("c", 64), "--archive-size", "100")
	output, err := command.CombinedOutput()
	if err != nil || string(output) != "RELEASE_MANIFEST_VERIFIED\n" {
		t.Fatalf("real verifier failed: %v %s", err, output)
	}
	probe := exec.Command(filepath.Join(directory, binary), "version", "--release-schema")
	output, err = probe.CombinedOutput()
	if err != nil || string(output) != "observer-release/v2\n" {
		t.Fatal("real schema probe failed")
	}
	// A non-Linux v2 identity has no helper digest, but still cannot downgrade
	// to a v1 manifest with the same otherwise-valid archive/build identity.
	manifest.SchemaVersion = releasepack.SchemaVersion
	for index := range manifest.Assets {
		manifest.Assets[index].ContentProfile = ""
		manifest.Assets[index].JournalHelper = nil
	}
	payload, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, payload, 0600); err != nil {
		t.Fatal(err)
	}
	command = exec.Command(filepath.Join(directory, binary), "version", "--release-manifest", manifestPath, "--archive-sha256", strings.Repeat("c", 64), "--archive-size", "100")
	output, err = command.CombinedOutput()
	if err == nil || string(output) != "INSTALLED_RELEASE_INVALID\n" {
		t.Fatal("real v2 binary accepted manifest downgrade")
	}
	if err := os.WriteFile(manifestPath, []byte("private-canary"), 0600); err != nil {
		t.Fatal(err)
	}
	command = exec.Command(filepath.Join(directory, binary), "version", "--release-manifest", manifestPath, "--archive-sha256", strings.Repeat("c", 64), "--archive-size", "100")
	output, err = command.CombinedOutput()
	if err == nil || string(output) != "INSTALLED_RELEASE_INVALID\n" {
		t.Fatal("invalid manifest accepted or leaked")
	}
}
