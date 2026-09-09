package releasepack

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const (
	testVersion = "0.1.0-preview.1"
	testCommit  = "0123456789abcdef0123456789abcdef01234567"
)

func TestBuildCreatesDeterministicFixedArchivesAndManifest(t *testing.T) {
	config := testConfig(t)
	manifest, err := Build(config)
	if err != nil {
		t.Fatal(err)
	}
	secondOutput := filepath.Join(filepath.Dir(config.OutputDir), "second output")
	second := config
	second.OutputDir = secondOutput
	if _, err := Build(second); err != nil {
		t.Fatal(err)
	}
	firstFiles := directoryFiles(t, config.OutputDir)
	secondFiles := directoryFiles(t, secondOutput)
	if !reflect.DeepEqual(firstFiles, secondFiles) {
		t.Fatalf("releasepack file sets differ: %v / %v", firstFiles, secondFiles)
	}
	for _, name := range firstFiles {
		first, err := os.ReadFile(filepath.Join(config.OutputDir, name))
		if err != nil {
			t.Fatal(err)
		}
		other, err := os.ReadFile(filepath.Join(secondOutput, name))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, other) {
			t.Errorf("%s is not deterministic", name)
		}
	}

	if manifest.SchemaVersion != SchemaVersion || manifest.Version != testVersion || manifest.Tag != "v"+testVersion || manifest.Repository != Repository || manifest.CommitSHA != testCommit || manifest.SigningPolicy != SigningPolicy || len(manifest.Assets) != 6 {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(config.OutputDir, ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	var decoded Manifest
	if err := json.Unmarshal(manifestBytes, &decoded); err != nil || !reflect.DeepEqual(decoded, manifest) {
		t.Fatalf("manifest file mismatch: %v %+v", err, decoded)
	}
	for index, asset := range manifest.Assets {
		target := targets[index]
		if asset.OS != target.os || asset.Arch != target.arch || asset.Format != target.format || !strings.Contains(asset.DownloadURL, "/releases/download/v"+testVersion+"/") {
			t.Errorf("unexpected asset: %+v", asset)
		}
		hash, size, err := hashFile(filepath.Join(config.OutputDir, asset.Filename))
		if err != nil || hash != asset.SHA256 || size != asset.SizeBytes {
			t.Errorf("asset bytes do not match manifest: %+v error=%v", asset, err)
		}
	}
	if err := Verify(config.OutputDir); err != nil {
		t.Fatalf("built output did not verify: %v", err)
	}
	assertChecksumFile(t, config.OutputDir, manifest)
	assertTarArchive(t, config.OutputDir, manifest.Assets[0], "run-observer.sh")
	assertTarArchive(t, config.OutputDir, manifest.Assets[2], "Run-Observer.command")
	assertZipArchive(t, config.OutputDir, manifest.Assets[4])
}

func TestVerifyRejectsTamperAndUnexpectedFiles(t *testing.T) {
	config := testConfig(t)
	manifest, err := Build(config)
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(config.OutputDir, manifest.Assets[0].Filename)
	file, err := os.OpenFile(archive, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("tamper")); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Verify(config.OutputDir); err == nil {
		t.Fatal("tampered archive unexpectedly verified")
	}
	if err := os.WriteFile(filepath.Join(config.OutputDir, "unexpected.txt"), []byte("unexpected"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(config.OutputDir); err == nil {
		t.Fatal("unexpected output member was accepted")
	}
}

func TestBuildRefusesUnsafeVersionsInputsAndExistingOutput(t *testing.T) {
	for _, version := range []string{"", "v0.1.0-preview.1", "0.1.0", "01.1.0-preview", "0.1.0-01", "0.1.0-preview+build", "0.1.0-../../escape", strings.Repeat("a", 65)} {
		if _, err := Build(Config{Version: version}); err == nil || err.Error() != "INVALID_VERSION" {
			t.Errorf("unsafe version %q returned %v", version, err)
		}
	}
	config := testConfig(t)
	invalidCommit := config
	invalidCommit.Commit = strings.Repeat("A", 40)
	if _, err := Build(invalidCommit); err == nil || err.Error() != "INVALID_COMMIT_SHA" {
		t.Fatalf("invalid commit returned %v", err)
	}
	if _, err := Build(config); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(config.OutputDir, ChecksumsName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Build(config); err == nil || err.Error() != "OUTPUT_ALREADY_EXISTS" {
		t.Fatalf("existing output returned %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(config.OutputDir, ChecksumsName))
	if !reflect.DeepEqual(before, after) {
		t.Fatal("refused rebuild changed existing output")
	}
}

func TestBuildRejectsIncompleteOrSymlinkedInputs(t *testing.T) {
	config := testConfig(t)
	if err := os.Remove(filepath.Join(config.BinariesDir, targets[0].binaryName)); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(config); err == nil || err.Error() != "BINARY_SET_INVALID" {
		t.Fatalf("incomplete binaries returned %v", err)
	}

	config = testConfig(t)
	link := filepath.Join(filepath.Dir(config.ResourcesDir), "resource-link")
	if err := os.Symlink(config.ResourcesDir, link); err != nil {
		if runtime.GOOS == "windows" || errors.Is(err, fs.ErrPermission) {
			t.Skip("symlink creation is not permitted on this platform")
		}
		t.Fatal(err)
	}
	config.ResourcesDir = link
	if _, err := Build(config); err == nil || err.Error() != "RESOURCES_DIRECTORY_UNAVAILABLE" {
		t.Fatalf("symlinked resources returned %v", err)
	}
}

func testConfig(t *testing.T) Config {
	t.Helper()
	root := t.TempDir()
	binaries := filepath.Join(root, "staged binaries")
	resources := filepath.Join(root, "archive resources")
	if err := os.Mkdir(binaries, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(resources, 0o700); err != nil {
		t.Fatal(err)
	}
	for index, target := range targets {
		content := []byte("synthetic observer binary " + target.os + " " + target.arch + " " + strings.Repeat("x", index))
		if err := os.WriteFile(filepath.Join(binaries, target.binaryName), content, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"LICENSE":              "Synthetic test license\n",
		"START-HERE.md":        "# Start here\n",
		"run-observer.sh":      "#!/bin/sh\nexec \"$(dirname \"$0\")/observer\" \"$@\"\n",
		"Run-Observer.command": "#!/bin/sh\nexec \"$(dirname \"$0\")/observer\" \"$@\"\n",
		"Run-Observer.cmd":     "@echo off\r\n\"%~dp0observer.exe\" %*\r\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(resources, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return Config{Version: testVersion, Commit: testCommit, BinariesDir: binaries, ResourcesDir: resources, OutputDir: filepath.Join(root, "release output")}
}

func directoryFiles(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("unexpected output directory: %s", entry.Name())
		}
		result = append(result, entry.Name())
	}
	sort.Strings(result)
	if len(result) != 8 {
		t.Fatalf("expected six archives, manifest and checksums; got %v", result)
	}
	return result
}

func assertChecksumFile(t *testing.T, directory string, manifest Manifest) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(directory, ChecksumsName))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	if len(lines) != 7 {
		t.Fatalf("unexpected checksum lines: %v", lines)
	}
	for _, line := range lines {
		parts := strings.Split(line, "  ")
		if len(parts) != 2 || parts[1] == ChecksumsName {
			t.Fatalf("invalid checksum line: %q", line)
		}
		bytes, err := os.ReadFile(filepath.Join(directory, parts[1]))
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(bytes)
		if parts[0] != hex.EncodeToString(hash[:]) {
			t.Fatalf("checksum mismatch: %q", line)
		}
	}
}

func assertTarArchive(t *testing.T, directory string, asset Asset, helper string) {
	t.Helper()
	file, err := os.Open(filepath.Join(directory, asset.Filename))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	root := strings.TrimSuffix(asset.Filename, ".tar.gz")
	want := map[string]int64{root + "/": 0o755, root + "/observer": 0o755, root + "/LICENSE": 0o644, root + "/START-HERE.md": 0o644, root + "/" + helper: 0o755}
	seen := make(map[string]bool)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		mode, ok := want[header.Name]
		if !ok || seen[header.Name] || header.Mode != mode || !header.ModTime.Equal(tarTime) || header.Uid != 0 || header.Gid != 0 {
			t.Fatalf("unsafe or unexpected tar member: %+v", header)
		}
		seen[header.Name] = true
		if header.Typeflag == tar.TypeReg {
			if _, err := io.ReadAll(reader); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("tar members = %v; want %v", seen, want)
	}
}

func assertZipArchive(t *testing.T, directory string, asset Asset) {
	t.Helper()
	reader, err := zip.OpenReader(filepath.Join(directory, asset.Filename))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	root := strings.TrimSuffix(asset.Filename, ".zip")
	want := map[string]bool{root + "/": true, root + "/observer.exe": true, root + "/LICENSE": true, root + "/START-HERE.md": true, root + "/Run-Observer.cmd": true}
	seen := make(map[string]bool)
	for _, file := range reader.File {
		if !want[file.Name] || seen[file.Name] {
			t.Fatalf("unsafe or unexpected zip member: %s", file.Name)
		}
		seen[file.Name] = true
		member, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.ReadAll(member); err != nil {
			t.Fatal(err)
		}
		if err := member.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("zip members = %v; want %v", seen, want)
	}
}
