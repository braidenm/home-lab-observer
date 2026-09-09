// Package releasepack creates deterministic, fixed-content native preview archives.
package releasepack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	SchemaVersion  = "observer-release/v1"
	Repository     = "braidenm/home-lab-observer"
	SigningPolicy  = "UNSIGNED_PREVIEW_WITH_CHECKSUMS_AND_PROVENANCE"
	ManifestName   = "release-manifest.json"
	ChecksumsName  = "SHA256SUMS"
	maxBinarySize  = 200 << 20
	maxArchiveSize = 220 << 20
)

var (
	versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)$`)
	commitPattern  = regexp.MustCompile(`^[a-f0-9]{40}$`)
)

type Config struct {
	Version      string
	Commit       string
	BinariesDir  string
	OutputDir    string
	ResourcesDir string
}

type Manifest struct {
	SchemaVersion string  `json:"schema_version"`
	Version       string  `json:"version"`
	Tag           string  `json:"tag"`
	Repository    string  `json:"repository"`
	CommitSHA     string  `json:"commit_sha"`
	SigningPolicy string  `json:"signing_policy"`
	Assets        []Asset `json:"assets"`
}

type Asset struct {
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	Filename    string `json:"filename"`
	SHA256      string `json:"sha256"`
	SizeBytes   int64  `json:"size_bytes"`
	Format      string `json:"format"`
	DownloadURL string `json:"download_url"`
}

type target struct {
	os           string
	arch         string
	format       string
	binaryName   string
	helperName   string
	helperSource string
}

var targets = []target{
	{os: "linux", arch: "amd64", format: "tar.gz", binaryName: "observer_linux_amd64", helperName: "run-observer.sh", helperSource: "run-observer.sh"},
	{os: "linux", arch: "arm64", format: "tar.gz", binaryName: "observer_linux_arm64", helperName: "run-observer.sh", helperSource: "run-observer.sh"},
	{os: "darwin", arch: "amd64", format: "tar.gz", binaryName: "observer_darwin_amd64", helperName: "Run-Observer.command", helperSource: "Run-Observer.command"},
	{os: "darwin", arch: "arm64", format: "tar.gz", binaryName: "observer_darwin_arm64", helperName: "Run-Observer.command", helperSource: "Run-Observer.command"},
	{os: "windows", arch: "amd64", format: "zip", binaryName: "observer_windows_amd64.exe", helperName: "Run-Observer.cmd", helperSource: "Run-Observer.cmd"},
	{os: "windows", arch: "arm64", format: "zip", binaryName: "observer_windows_arm64.exe", helperName: "Run-Observer.cmd", helperSource: "Run-Observer.cmd"},
}

type inputs struct {
	config   Config
	binaries map[string]sourceFile
	license  []byte
	start    []byte
	helpers  map[string][]byte
}

type sourceFile struct {
	path string
	info fs.FileInfo
}

func Build(config Config) (Manifest, error) {
	in, err := validateAndLoad(config)
	if err != nil {
		return Manifest{}, err
	}
	parent := filepath.Dir(in.config.OutputDir)
	stage, err := os.MkdirTemp(parent, ".releasepack-")
	if err != nil {
		return Manifest{}, errors.New("OUTPUT_STAGING_FAILED")
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(stage)
		}
	}()

	manifest := Manifest{
		SchemaVersion: SchemaVersion,
		Version:       in.config.Version,
		Tag:           "v" + in.config.Version,
		Repository:    Repository,
		CommitSHA:     in.config.Commit,
		SigningPolicy: SigningPolicy,
		Assets:        make([]Asset, 0, len(targets)),
	}
	for _, target := range targets {
		asset, err := buildArchive(stage, in, target)
		if err != nil {
			return Manifest{}, err
		}
		manifest.Assets = append(manifest.Assets, asset)
	}
	manifestBytes, err := marshalManifest(manifest)
	if err != nil {
		return Manifest{}, errors.New("MANIFEST_ENCODING_FAILED")
	}
	if err := writePrivateFile(filepath.Join(stage, ManifestName), manifestBytes); err != nil {
		return Manifest{}, errors.New("MANIFEST_WRITE_FAILED")
	}

	checksumNames := make([]string, 0, len(targets)+1)
	for _, asset := range manifest.Assets {
		checksumNames = append(checksumNames, asset.Filename)
	}
	checksumNames = append(checksumNames, ManifestName)
	sort.Strings(checksumNames)
	var sums strings.Builder
	for _, name := range checksumNames {
		hash, _, err := hashFile(filepath.Join(stage, name))
		if err != nil {
			return Manifest{}, errors.New("CHECKSUM_READ_FAILED")
		}
		fmt.Fprintf(&sums, "%s  %s\n", hash, name)
	}
	if err := writePrivateFile(filepath.Join(stage, ChecksumsName), []byte(sums.String())); err != nil {
		return Manifest{}, errors.New("CHECKSUM_WRITE_FAILED")
	}
	if err := makePublicFiles(stage); err != nil {
		return Manifest{}, errors.New("OUTPUT_PERMISSION_FAILED")
	}
	if err := Verify(stage); err != nil {
		return Manifest{}, err
	}
	if err := os.Rename(stage, in.config.OutputDir); err != nil {
		return Manifest{}, errors.New("OUTPUT_FINALIZE_FAILED")
	}
	complete = true
	return manifest, nil
}

func validateAndLoad(config Config) (inputs, error) {
	if !validVersion(config.Version) {
		return inputs{}, errors.New("INVALID_VERSION")
	}
	if !commitPattern.MatchString(config.Commit) {
		return inputs{}, errors.New("INVALID_COMMIT_SHA")
	}
	resolved, err := resolvePaths(config)
	if err != nil {
		return inputs{}, err
	}
	if _, err := os.Lstat(resolved.OutputDir); err == nil {
		return inputs{}, errors.New("OUTPUT_ALREADY_EXISTS")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return inputs{}, errors.New("OUTPUT_PATH_UNAVAILABLE")
	}
	if pathsOverlap(resolved.OutputDir, resolved.BinariesDir) || pathsOverlap(resolved.OutputDir, resolved.ResourcesDir) {
		return inputs{}, errors.New("OUTPUT_OVERLAPS_INPUT")
	}
	binaries, err := loadBinaries(resolved.BinariesDir)
	if err != nil {
		return inputs{}, err
	}
	license, err := readRegularBounded(filepath.Join(resolved.ResourcesDir, "LICENSE"), 1<<20)
	if err != nil {
		return inputs{}, errors.New("INVALID_LICENSE_RESOURCE")
	}
	start, err := readRegularBounded(filepath.Join(resolved.ResourcesDir, "START-HERE.md"), 256<<10)
	if err != nil {
		return inputs{}, errors.New("INVALID_START_RESOURCE")
	}
	helpers := make(map[string][]byte, 3)
	for _, name := range []string{"run-observer.sh", "Run-Observer.command", "Run-Observer.cmd"} {
		content, err := readRegularBounded(filepath.Join(resolved.ResourcesDir, name), 64<<10)
		if err != nil {
			return inputs{}, errors.New("INVALID_LAUNCH_RESOURCE")
		}
		helpers[name] = content
	}
	return inputs{config: resolved, binaries: binaries, license: license, start: start, helpers: helpers}, nil
}

func validVersion(version string) bool {
	if len(version) < 7 || len(version) > 64 || strings.HasPrefix(version, "v") || !versionPattern.MatchString(version) {
		return false
	}
	prerelease := strings.SplitN(version, "-", 2)[1]
	for _, identifier := range strings.Split(prerelease, ".") {
		if len(identifier) > 1 && identifier[0] == '0' {
			allNumeric := true
			for _, character := range identifier {
				allNumeric = allNumeric && character >= '0' && character <= '9'
			}
			if allNumeric {
				return false
			}
		}
	}
	return true
}

func resolvePaths(config Config) (Config, error) {
	var err error
	if config.BinariesDir, err = canonicalInputDirectory(config.BinariesDir); err != nil {
		return Config{}, errors.New("BINARIES_DIRECTORY_UNAVAILABLE")
	}
	if config.ResourcesDir, err = canonicalInputDirectory(config.ResourcesDir); err != nil {
		return Config{}, errors.New("RESOURCES_DIRECTORY_UNAVAILABLE")
	}
	if config.OutputDir == "" || config.OutputDir != strings.TrimSpace(config.OutputDir) {
		return Config{}, errors.New("INVALID_PATH")
	}
	absolute, err := filepath.Abs(config.OutputDir)
	if err != nil || filepath.Base(absolute) == "." || filepath.Base(absolute) == string(filepath.Separator) {
		return Config{}, errors.New("INVALID_PATH")
	}
	parent := filepath.Dir(absolute)
	parentInfo, err := os.Lstat(parent)
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
		return Config{}, errors.New("OUTPUT_PARENT_UNAVAILABLE")
	}
	canonicalParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return Config{}, errors.New("OUTPUT_PARENT_UNAVAILABLE")
	}
	config.OutputDir = filepath.Join(canonicalParent, filepath.Base(absolute))
	return config, nil
}

func canonicalInputDirectory(raw string) (string, error) {
	if raw == "" || raw != strings.TrimSpace(raw) {
		return "", errors.New("invalid path")
	}
	absolute, err := filepath.Abs(raw)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(absolute)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("invalid directory")
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(canonical), nil
}

func pathsOverlap(left, right string) bool {
	return containsPath(left, right) || containsPath(right, left)
}

func containsPath(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func loadBinaries(directory string) (map[string]sourceFile, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, errors.New("BINARIES_DIRECTORY_UNAVAILABLE")
	}
	expected := make(map[string]bool, len(targets))
	for _, target := range targets {
		expected[target.binaryName] = true
	}
	if len(entries) != len(expected) {
		return nil, errors.New("BINARY_SET_INVALID")
	}
	result := make(map[string]sourceFile, len(expected))
	for _, entry := range entries {
		if !expected[entry.Name()] {
			return nil, errors.New("BINARY_SET_INVALID")
		}
		path := filepath.Join(directory, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxBinarySize {
			return nil, errors.New("BINARY_INPUT_INVALID")
		}
		result[entry.Name()] = sourceFile{path: path, info: info}
	}
	return result, nil
}

func readRegularBounded(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit {
		return nil, errors.New("invalid resource")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) || openedInfo.Size() != info.Size() {
		return nil, errors.New("resource changed")
	}
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(content)) > limit || len(content) == 0 {
		return nil, errors.New("invalid resource")
	}
	return content, nil
}

func marshalManifest(manifest Manifest) ([]byte, error) {
	var output strings.Builder
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		return nil, err
	}
	return []byte(output.String()), nil
}

func writePrivateFile(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	written, err := file.Write(content)
	if err != nil || written != len(content) {
		_ = file.Close()
		if err == nil {
			err = io.ErrShortWrite
		}
		return err
	}
	return file.Close()
}

func makePublicFiles(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return errors.New("unexpected staging entry")
		}
		if err := os.Chmod(filepath.Join(directory, entry.Name()), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func hashFile(path string) (string, int64, error) {
	return hashFileBounded(path, maxArchiveSize)
}

func hashFileBounded(path string, limit int64) (string, int64, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > limit {
		return "", 0, errors.New("invalid hash input")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) || openedInfo.Size() != info.Size() {
		return "", 0, errors.New("hash input changed")
	}
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(file, limit+1))
	if err != nil {
		return "", 0, err
	}
	if size > limit || size != info.Size() {
		return "", 0, errors.New("hash input changed")
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}
