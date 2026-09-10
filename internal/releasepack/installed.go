package releasepack

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/braidenm/home-lab-observer/internal/buildidentity"
)

// Installed describes an already checksum-verified, privately extracted archive.
// The entrypoint supplies its own executable and authoritative runtime identity,
// never a caller-selected program to run. Verification executes nothing.
type Installed struct {
	// Code-owned: derived from the running binary's identity envelope presence.
	ExpectedSchemaVersion string
	ManifestPath          string
	ExecutablePath        string
	Identity              buildidentity.Identity
	ArchiveSHA256         string
	ArchiveSize           int64
}

func VerifyInstalled(input Installed) error {
	invalid := errors.New("INSTALLED_RELEASE_INVALID")
	if input.ExpectedSchemaVersion != SchemaVersion && input.ExpectedSchemaVersion != SchemaVersionV2 {
		return invalid
	}
	if !shaPattern(input.ArchiveSHA256) || input.ArchiveSize <= 0 || input.ArchiveSize > maxArchiveSize || !filepath.IsAbs(input.ExecutablePath) {
		return invalid
	}
	payload, err := readRegularBounded(input.ManifestPath, 1<<20)
	if err != nil {
		return invalid
	}
	manifest, err := DecodeManifest(payload)
	if err != nil {
		return invalid
	}
	if manifest.SchemaVersion != input.ExpectedSchemaVersion {
		return invalid
	}
	if manifest.Version != input.Identity.Version || manifest.CommitSHA != input.Identity.Commit || input.Identity.Role != "observer" {
		return invalid
	}
	index := -1
	for i, t := range targets {
		if t.os == input.Identity.OS && t.arch == input.Identity.Arch {
			index = i
			break
		}
	}
	if index < 0 {
		return invalid
	}
	target, asset := targets[index], manifest.Assets[index]
	if asset.SHA256 != input.ArchiveSHA256 || asset.SizeBytes != input.ArchiveSize {
		return invalid
	}
	directory := filepath.Dir(input.ExecutablePath)
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return invalid
	}
	binary := "observer"
	if target.os == "windows" {
		binary = "observer.exe"
	}
	if filepath.Base(input.ExecutablePath) != binary {
		return invalid
	}
	expected := map[string]int64{binary: maxBinarySize, "LICENSE": 1 << 20, "START-HERE.md": 1 << 20, target.helperName: 1 << 20}
	if asset.JournalHelper != nil {
		expected["observer-journal-helper"] = maxBinarySize
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != len(expected) {
		return invalid
	}
	files := map[string]sourceFile{}
	var expanded int64
	for _, entry := range entries {
		limit, ok := expected[entry.Name()]
		if !ok {
			return invalid
		}
		path := filepath.Join(directory, entry.Name())
		hash, size, err := hashFileBounded(path, limit)
		if err != nil || size <= 0 {
			return invalid
		}
		if expanded > maxArchiveSize-size {
			return invalid
		}
		expanded += size
		info, err := os.Lstat(path)
		if err != nil {
			return invalid
		}
		files[entry.Name()] = sourceFile{path: path, info: info}
		if entry.Name() == "observer-journal-helper" && (asset.JournalHelper == nil || hash != asset.JournalHelper.SHA256 || size != asset.JournalHelper.SizeBytes) {
			return invalid
		}
	}
	if manifest.SchemaVersion == SchemaVersionV2 {
		if input.Identity.Validate() != nil {
			return invalid
		}
		digest := ""
		if asset.JournalHelper != nil {
			digest = asset.JournalHelper.SHA256
		}
		if input.Identity.HelperSHA256 != digest {
			return invalid
		}
		config := Config{Version: manifest.Version, Commit: manifest.CommitSHA}
		if verifyBuildFile(files[binary], config, target, digest, false) != nil {
			return invalid
		}
		if asset.JournalHelper != nil && verifyBuildFile(files["observer-journal-helper"], config, target, "", true) != nil {
			return invalid
		}
	} else if input.Identity.HelperSHA256 != "" {
		return invalid
	}
	return nil
}
