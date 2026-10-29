// Package connectedbundle verifies a separate exact-content connected release.
// It neither installs services nor changes the native-v2 release contract.
package connectedbundle

import (
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"

	"github.com/braidenm/home-lab-observer/internal/connectedcompat"
	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
	"github.com/braidenm/home-lab-observer/internal/ownerfs"
)

const ManifestName = "connected-manifest.json"
const Version = "observer-connected-bundle/v1"
const VersionV2 = "observer-connected-bundle/v2"
const Profile = "ubuntu24.04-systemd255-amd64-canary"
const MaxManifestBytes = 16384
const MaxResourceBytes = 16384

var ErrInvalid = errors.New("connected_bundle_invalid")
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var binaries = map[string]string{
	"observer-connected-collector": "collector",
	"observer-connected-uploader":  "uploader",
	"observer-connected-install":   "install",
}
var richProcessesBinaries = map[string]string{
	"observer-connected-rich-collector": "rich-collector",
	"observer-connected-rich-uploader":  "rich-uploader",
	"observer-connected-rich-install":   "rich-install",
}

type bundleSpec struct {
	profile, contract, installRole string
	binaries                       map[string]string
	resources                      func() map[string][]byte
}

var numericBundleSpec = bundleSpec{Profile, connectedcompat.Digest(), "install", binaries, connectedunits.Resources}
var richProcessesBundleSpec = bundleSpec{"ubuntu24.04-systemd255-amd64-rich-processes-canary", connectedprofile.RichProcessesDefinitionSHA256, "rich-install", richProcessesBinaries, connectedunits.RichProcessesResources}

type File struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
}
type Manifest struct {
	Schema         string `json:"schema"`
	Version        string `json:"version"`
	Commit         string `json:"commit"`
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	Profile        string `json:"profile"`
	ContractSHA256 string `json:"contract_sha256,omitempty"`
	Files          []File `json:"files"`
}

func Names() []string {
	return names(numericBundleSpec)
}

func RichProcessesNames() []string {
	return names(richProcessesBundleSpec)
}

func names(spec bundleSpec) []string {
	names := make([]string, 0, len(spec.binaries)+len(spec.resources()))
	for name := range spec.binaries {
		names = append(names, name)
	}
	for name := range spec.resources() {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m Manifest) Validate() error {
	if m.Schema == VersionV2 && !connectedcompat.MatchesResources(connectedunits.Resources()) {
		return ErrInvalid
	}
	return validateManifest(m, numericBundleSpec, true)
}

func (m Manifest) ValidateRichProcesses() error {
	return validateManifest(m, richProcessesBundleSpec, false)
}

func validateManifest(m Manifest, spec bundleSpec, allowV1 bool) error {
	schemaOK := m.Schema == VersionV2 || allowV1 && m.Schema == Version
	identity := connectedidentity.Identity{Role: spec.installRole, Version: m.Version, Commit: m.Commit, OS: m.OS, Arch: m.Arch, ContractSHA256: m.ContractSHA256}
	if !schemaOK || (m.Schema == Version && m.ContractSHA256 != "") || (m.Schema == VersionV2 && m.ContractSHA256 != spec.contract) || m.Profile != spec.profile || identity.Validate() != nil {
		return ErrInvalid
	}
	wantNames := names(spec)
	if len(m.Files) != len(wantNames) {
		return ErrInvalid
	}
	for i, f := range m.Files {
		limit, mode := int64(MaxResourceBytes), uint32(0o644)
		if _, ok := spec.binaries[f.Name]; ok {
			limit = connectedidentity.MaxFileBytes
			mode = 0o755
		}
		if f.Name != wantNames[i] || f.Size <= 0 || f.Size > limit || f.Mode != mode || !digestPattern.MatchString(f.SHA256) {
			return ErrInvalid
		}
	}
	return nil
}

// HasKnownContract recognizes a declared code-owned contract. It does not verify
// actual files, distribution authenticity, transitions or activation readiness.
func (m Manifest) HasKnownContract() bool {
	return m.Validate() == nil && m.Schema == VersionV2 && connectedcompat.Known(m.ContractSHA256)
}

func (m Manifest) HasKnownRichProcessesContract() bool {
	return m.ValidateRichProcesses() == nil && m.Schema == VersionV2 && m.ContractSHA256 == connectedprofile.RichProcessesDefinitionSHA256
}

func Encode(m Manifest) ([]byte, error) {
	return encode(m, m.Validate)
}

func EncodeRichProcesses(m Manifest) ([]byte, error) {
	return encode(m, m.ValidateRichProcesses)
}

func encode(m Manifest, validate func() error) ([]byte, error) {
	if validate() != nil {
		return nil, ErrInvalid
	}
	b, err := json.Marshal(m)
	if err != nil || len(b) > MaxManifestBytes {
		return nil, ErrInvalid
	}
	return b, nil
}

func Decode(data []byte) (Manifest, error) {
	return decode(data, Encode)
}

func DecodeRichProcesses(data []byte) (Manifest, error) {
	return decode(data, EncodeRichProcesses)
}

func decode(data []byte, encode func(Manifest) ([]byte, error)) (Manifest, error) {
	var m Manifest
	if len(data) == 0 || len(data) > MaxManifestBytes || json.Unmarshal(data, &m) != nil {
		return Manifest{}, ErrInvalid
	}
	canonical, err := encode(m)
	if err != nil || !bytes.Equal(data, canonical) {
		return Manifest{}, ErrInvalid
	}
	return m, nil
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func openRoot(directory string) (*os.Root, error) {
	if ownerfs.ValidateDedicatedDirectory(directory) != nil {
		return nil, ErrInvalid
	}
	r, err := os.OpenRoot(directory)
	if err != nil {
		return nil, ErrInvalid
	}
	return r, nil
}

func exactEntries(root *os.Root, names []string) error {
	f, err := root.Open(".")
	if err != nil {
		return ErrInvalid
	}
	entries, readErr := f.ReadDir(len(names) + 1)
	closeErr := f.Close()
	if (readErr != nil && readErr != io.EOF) || closeErr != nil || len(entries) != len(names) {
		return ErrInvalid
	}
	found := make([]string, 0, len(entries))
	for _, entry := range entries {
		found = append(found, entry.Name())
	}
	sort.Strings(found)
	want := append([]string(nil), names...)
	sort.Strings(want)
	for i := range want {
		if found[i] != want[i] {
			return ErrInvalid
		}
	}
	return nil
}

func openFile(root *os.Root, directory, name string, limit int64) (*os.File, error) {
	before, err := ownerfs.ValidateRegular(filepath.Join(directory, name), limit)
	if err != nil {
		return nil, ErrInvalid
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, ErrInvalid
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) || !after.Mode().IsRegular() || after.Size() != before.Size() {
		f.Close()
		return nil, ErrInvalid
	}
	return f, nil
}

func inspectBinary(f *os.File, size int64, want connectedidentity.Identity) error {
	elfFile, err := elf.NewFile(f)
	if err != nil || elfFile.Class != elf.ELFCLASS64 || elfFile.Data != elf.ELFDATA2LSB || elfFile.Machine != elf.EM_X86_64 || (elfFile.Type != elf.ET_EXEC && elfFile.Type != elf.ET_DYN) {
		return ErrInvalid
	}
	for _, p := range elfFile.Progs {
		if p.Type == elf.PT_INTERP {
			return ErrInvalid
		}
	}
	libs, err := elfFile.ImportedLibraries()
	if err != nil || len(libs) != 0 {
		return ErrInvalid
	}
	info, err := buildinfo.Read(f)
	if err != nil || info.Path != "github.com/braidenm/home-lab-observer/cmd/observer-connected-"+want.Role {
		return ErrInvalid
	}
	settings := make(map[string]string, 3)
	for _, setting := range info.Settings {
		if setting.Key != "GOOS" && setting.Key != "GOARCH" && setting.Key != "CGO_ENABLED" {
			continue
		}
		if _, exists := settings[setting.Key]; exists {
			return ErrInvalid
		}
		settings[setting.Key] = setting.Value
	}
	if settings["GOOS"] != "linux" || settings["GOARCH"] != "amd64" || settings["CGO_ENABLED"] != "0" {
		return ErrInvalid
	}
	i, err := connectedidentity.Scan(io.NewSectionReader(f, 0, size), size)
	if err != nil || i != want {
		return ErrInvalid
	}
	return nil
}

func inspectFile(root *os.Root, directory string, entry File, m Manifest, spec bundleSpec) error {
	f, err := openFile(root, directory, entry.Name, entry.Size)
	if err != nil {
		return ErrInvalid
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() != entry.Size {
		return ErrInvalid
	}
	// Windows extraction does not preserve Unix modes; archive generation fixes
	// modes from the closed manifest. Linux runtime install validates final modes.
	if info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return ErrInvalid
	}
	if runtime.GOOS != "windows" && uint32(info.Mode().Perm()) != entry.Mode {
		return ErrInvalid
	}
	h := sha256.New()
	n, err := io.Copy(h, io.NewSectionReader(f, 0, entry.Size+1))
	if err != nil || n != entry.Size || hex.EncodeToString(h.Sum(nil)) != entry.SHA256 {
		return ErrInvalid
	}
	if role, ok := spec.binaries[entry.Name]; ok {
		if inspectBinary(f, entry.Size, connectedidentity.Identity{Role: role, Version: m.Version, Commit: m.Commit, OS: m.OS, Arch: m.Arch, ContractSHA256: m.ContractSHA256}) != nil {
			return ErrInvalid
		}
	} else {
		data, err := io.ReadAll(io.NewSectionReader(f, 0, entry.Size))
		if err != nil || !bytes.Equal(data, spec.resources()[entry.Name]) {
			return ErrInvalid
		}
	}
	final, err := ownerfs.ValidateRegular(filepath.Join(directory, entry.Name), entry.Size)
	if err != nil || !os.SameFile(info, final) || final.Size() != entry.Size {
		return ErrInvalid
	}
	return nil
}

// VerifyDirectory is read-only and needs an independently trusted expected
// canonical-manifest digest. It is not a long-lived capability: a privileged
// installer must verify its own freshly staged final copy before publishing it.
func VerifyDirectory(directory, expectedManifestSHA256 string) (Manifest, error) {
	return verifyDirectory(directory, expectedManifestSHA256, numericBundleSpec, Decode)
}

func VerifyRichProcessesDirectory(directory, expectedManifestSHA256 string) (Manifest, error) {
	return verifyDirectory(directory, expectedManifestSHA256, richProcessesBundleSpec, DecodeRichProcesses)
}

func verifyDirectory(directory, expectedManifestSHA256 string, spec bundleSpec, decode func([]byte) (Manifest, error)) (Manifest, error) {
	if !digestPattern.MatchString(expectedManifestSHA256) {
		return Manifest{}, ErrInvalid
	}
	r, err := openRoot(directory)
	if err != nil {
		return Manifest{}, err
	}
	defer r.Close()
	wantNames := names(spec)
	if exactEntries(r, append(wantNames, ManifestName)) != nil {
		return Manifest{}, ErrInvalid
	}
	f, err := openFile(r, directory, ManifestName, MaxManifestBytes)
	if err != nil {
		return Manifest{}, ErrInvalid
	}
	info, statErr := f.Stat()
	if statErr != nil || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 ||
		(runtime.GOOS != "windows" && info.Mode().Perm() != 0o644) {
		f.Close()
		return Manifest{}, ErrInvalid
	}
	data, readErr := io.ReadAll(io.LimitReader(f, MaxManifestBytes+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || digest(data) != expectedManifestSHA256 {
		return Manifest{}, ErrInvalid
	}
	m, err := decode(data)
	if err != nil {
		return Manifest{}, err
	}
	for _, entry := range m.Files {
		if inspectFile(r, directory, entry, m, spec) != nil {
			return Manifest{}, ErrInvalid
		}
	}
	if exactEntries(r, append(wantNames, ManifestName)) != nil {
		return Manifest{}, ErrInvalid
	}
	return m, nil
}

// CreateManifest verifies already staged exact resources and binaries. The build
// tool alone adds the new manifest; this never repairs an existing installation.
func CreateManifest(directory, version, commit string) ([]byte, error) {
	return createManifest(directory, version, commit, VersionV2, connectedcompat.Digest())
}

func CreateRichProcessesManifest(directory, version, commit string) ([]byte, error) {
	return createManifestForSpec(directory, version, commit, VersionV2, richProcessesBundleSpec.contract, richProcessesBundleSpec, EncodeRichProcesses)
}

func createManifest(directory, version, commit, schema, contract string) ([]byte, error) {
	return createManifestForSpec(directory, version, commit, schema, contract, numericBundleSpec, Encode)
}

func createManifestForSpec(directory, version, commit, schema, contract string, spec bundleSpec, encode func(Manifest) ([]byte, error)) ([]byte, error) {
	m := Manifest{Schema: schema, Version: version, Commit: commit, OS: "linux", Arch: "amd64", Profile: spec.profile, ContractSHA256: contract}
	if (connectedidentity.Identity{Role: spec.installRole, Version: version, Commit: commit, OS: "linux", Arch: "amd64", ContractSHA256: contract}).Validate() != nil {
		return nil, ErrInvalid
	}
	r, err := openRoot(directory)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	wantNames := names(spec)
	if exactEntries(r, wantNames) != nil {
		return nil, ErrInvalid
	}
	for _, name := range wantNames {
		limit, mode := int64(MaxResourceBytes), uint32(0o644)
		if _, ok := spec.binaries[name]; ok {
			limit = connectedidentity.MaxFileBytes
			mode = 0o755
		}
		f, err := openFile(r, directory, name, limit)
		if err != nil {
			return nil, ErrInvalid
		}
		h := sha256.New()
		n, copyErr := io.Copy(h, io.LimitReader(f, limit+1))
		closeErr := f.Close()
		if copyErr != nil || closeErr != nil || n <= 0 || n > limit {
			return nil, ErrInvalid
		}
		m.Files = append(m.Files, File{Name: name, SHA256: hex.EncodeToString(h.Sum(nil)), Size: n, Mode: mode})
	}
	for _, entry := range m.Files {
		if inspectFile(r, directory, entry, m, spec) != nil {
			return nil, ErrInvalid
		}
	}
	return encode(m)
}
