package releasepack

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func Verify(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return errors.New("OUTPUT_READ_FAILED")
	}
	// The manifest supplies the authoritative version below. At this point only reject unsafe entry types;
	// the exact filename set is checked after decoding that version.
	for _, entry := range entries {
		info, err := os.Lstat(filepath.Join(directory, entry.Name()))
		if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxArchiveSize || !singleLink(filepath.Join(directory, entry.Name()), info) {
			return errors.New("OUTPUT_SET_INVALID")
		}
	}
	manifestBytes, err := readRegularBounded(filepath.Join(directory, ManifestName), 1<<20)
	if err != nil {
		return errors.New("MANIFEST_READ_FAILED")
	}
	manifest, err := DecodeManifest(manifestBytes)
	if err != nil {
		return err
	}
	if err := validateManifest(manifest, directory); err != nil {
		return err
	}
	expectedFiles := map[string]bool{ManifestName: true, ChecksumsName: true}
	for _, asset := range manifest.Assets {
		expectedFiles[asset.Filename] = true
	}
	if len(entries) != len(expectedFiles) {
		return errors.New("OUTPUT_SET_INVALID")
	}
	for _, entry := range entries {
		if !expectedFiles[entry.Name()] {
			return errors.New("OUTPUT_SET_INVALID")
		}
	}
	checksumBytes, err := readRegularBounded(filepath.Join(directory, ChecksumsName), 64<<10)
	if err != nil {
		return errors.New("CHECKSUM_READ_FAILED")
	}
	wantNames := make([]string, 0, len(manifest.Assets)+1)
	for _, asset := range manifest.Assets {
		wantNames = append(wantNames, asset.Filename)
	}
	wantNames = append(wantNames, ManifestName)
	sort.Strings(wantNames)
	lines := strings.Split(strings.TrimSuffix(string(checksumBytes), "\n"), "\n")
	if len(lines) != len(wantNames) {
		return errors.New("CHECKSUM_SET_INVALID")
	}
	for index, name := range wantNames {
		parts := strings.Split(lines[index], "  ")
		if len(parts) != 2 || parts[1] != name || !shaPattern(parts[0]) {
			return errors.New("CHECKSUM_SET_INVALID")
		}
		hash, _, err := hashFileBounded(filepath.Join(directory, name), outputFileLimit(name))
		if err != nil || hash != parts[0] {
			return errors.New("CHECKSUM_MISMATCH")
		}
	}
	return nil
}

func validateManifest(manifest Manifest, directory string) error {
	if err := validateManifestShape(manifest); err != nil {
		return err
	}
	for index, target := range targets {
		asset := manifest.Assets[index]
		root := "home-lab-observer_" + manifest.Version + "_" + target.os + "_" + target.arch
		filename := root + "." + target.format
		url := "https://github.com/" + Repository + "/releases/download/" + manifest.Tag + "/" + filename
		if asset.OS != target.os || asset.Arch != target.arch || asset.Format != target.format || asset.Filename != filename || asset.DownloadURL != url || !shaPattern(asset.SHA256) || asset.SizeBytes <= 0 || asset.SizeBytes > maxArchiveSize {
			return errors.New("MANIFEST_INVALID")
		}
		hash, size, err := hashFileBounded(filepath.Join(directory, filename), maxArchiveSize)
		if err != nil || hash != asset.SHA256 || size != asset.SizeBytes {
			return errors.New("ASSET_MISMATCH")
		}
		if err := verifyArchive(filepath.Join(directory, filename), asset); err != nil {
			return err
		}
	}
	return nil
}

func outputFileLimit(name string) int64 {
	switch name {
	case ManifestName:
		return 1 << 20
	case ChecksumsName:
		return 64 << 10
	default:
		return maxArchiveSize
	}
}

func shaPattern(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
