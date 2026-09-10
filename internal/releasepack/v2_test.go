package releasepack

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func v2Fixture(t *testing.T) (inputs, Manifest, string) {
	t.Helper()
	config := testConfig(t)
	in, err := validateAndLoad(config)
	if err != nil {
		t.Fatal(err)
	}
	in.config.SchemaVersion = SchemaVersionV2
	for _, arch := range []string{"amd64", "arm64"} {
		path := filepath.Join(config.BinariesDir, journalInputName(arch))
		if err := os.WriteFile(path, []byte("synthetic-private-helper-"+arch), 0700); err != nil {
			t.Fatal(err)
		}
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		in.binaries[journalInputName(arch)] = sourceFile{path: path, info: info}
	}
	stage := t.TempDir()
	manifest := Manifest{SchemaVersion: SchemaVersionV2, Version: testVersion, Tag: "v" + testVersion, Repository: Repository, CommitSHA: testCommit, SigningPolicy: SigningPolicy}
	for _, target := range targets {
		asset, err := buildArchive(stage, in, target)
		if err != nil {
			t.Fatal(err)
		}
		manifest.Assets = append(manifest.Assets, asset)
	}
	return in, manifest, stage
}

func TestV2ProfilesAndExactHelperArchive(t *testing.T) {
	_, manifest, stage := v2Fixture(t)
	payload, err := marshalManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(payload, []byte(`"journal_helper": null`)) != 4 {
		t.Fatal("non-Linux null profiles missing")
	}
	decoded, err := DecodeManifest(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateManifest(decoded, stage); err != nil {
		t.Fatal(err)
	}
	for _, asset := range manifest.Assets {
		if asset.OS == "linux" && (asset.ContentProfile != "linux-journal-helper-v1" || asset.JournalHelper == nil) {
			t.Fatal("helper profile missing")
		}
	}
	manifest.Assets[0].JournalHelper.SHA256 = strings.Repeat("a", 64)
	if validateManifest(manifest, stage) == nil {
		t.Fatal("wrong helper digest accepted")
	}
}

func TestClosedManifestRejectsAmbiguity(t *testing.T) {
	_, manifest, _ := v2Fixture(t)
	valid, _ := marshalManifest(manifest)
	for name, payload := range map[string][]byte{
		"duplicate":    bytes.Replace(valid, []byte(`"schema_version":`), []byte(`"version":"canary", "schema_version":`), 1),
		"case":         bytes.Replace(valid, []byte(`"schema_version"`), []byte(`"Schema_Version"`), 1),
		"unknown":      bytes.Replace(valid, []byte(`"schema_version":`), []byte(`"canary":0,"schema_version":`), 1),
		"missing-null": bytes.Replace(valid, []byte(`"journal_helper": null`), []byte(`"ignored": null`), 1),
		"null-linux":   bytes.Replace(valid, []byte(`"linux-journal-helper-v1"`), []byte(`null`), 1),
		"trailing":     append(bytes.Clone(valid), []byte(` {}`)...),
		"bom":          append([]byte{0xef, 0xbb, 0xbf}, valid...),
		"utf8":         append(bytes.Clone(valid), 0xff),
		"oversize":     bytes.Repeat([]byte(" "), (1<<20)+1),
		"fraction":     bytes.Replace(valid, []byte(`"size_bytes": `), []byte(`"size_bytes": 0.5e`), 1),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeManifest(payload)
			if err == nil || err.Error() != "MANIFEST_INVALID" {
				t.Fatal("invalid manifest accepted or leaked")
			}
		})
	}
	config := testConfig(t)
	v1, err := Build(config)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(v1)
	if bytes.Contains(payload, []byte("journal_helper")) || bytes.Contains(payload, []byte("content_profile")) {
		t.Fatal("v1 wire changed")
	}
	if _, err := DecodeManifest(payload); err != nil {
		t.Fatal(err)
	}
}

func TestV2InputSetRejectsMissingExtraLinkedAndSyntheticIdentity(t *testing.T) {
	in, _, _ := v2Fixture(t)
	config := in.config
	if _, err := Build(config); err == nil {
		t.Fatal("synthetic files accepted as release binaries")
	}
	path := filepath.Join(config.BinariesDir, journalInputName("amd64"))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := loadBinaries(config.BinariesDir, SchemaVersionV2); err == nil {
		t.Fatal("missing helper accepted")
	}
	if err := os.Link(filepath.Join(config.BinariesDir, journalInputName("arm64")), path); err != nil {
		t.Skip("hardlink unsupported")
	}
	if _, err := loadBinaries(config.BinariesDir, SchemaVersionV2); err == nil {
		t.Fatal("linked helper accepted")
	}
	if _, err := loadBinaries(config.BinariesDir, SchemaVersion); err == nil {
		t.Fatal("v1 accepted extra helpers")
	}
}

func TestArchiveVerificationRejectsUnexpectedOrLinkedEntries(t *testing.T) {
	in, manifest, _ := v2Fixture(t)
	asset := manifest.Assets[0]
	for _, mode := range []string{"missing", "extra", "wrong-helper", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			root := strings.TrimSuffix(asset.Filename, ".tar.gz")
			entries := []archiveEntry{{name: "observer", mode: 0755, data: []byte("binary")}, {name: "LICENSE", mode: 0644, data: in.license}, {name: "START-HERE.md", mode: 0644, data: in.start}, {name: "run-observer.sh", mode: 0755, data: in.helpers["run-observer.sh"]}}
			if mode != "missing" {
				entries = append(entries, archiveEntry{name: "observer-journal-helper", mode: 0755, data: []byte("wrong helper")})
			}
			if mode == "extra" {
				entries = append(entries, archiveEntry{name: "private-canary", mode: 0644, data: []byte("canary")})
			}
			if mode == "oversize" {
				entries[1].data = make([]byte, (1<<20)+1)
			}
			path := filepath.Join(t.TempDir(), asset.Filename)
			if err := writeTarGzip(path, root, entries); err != nil {
				t.Fatal(err)
			}
			if verifyArchive(path, asset) == nil {
				t.Fatal("invalid contents accepted")
			}
		})
	}
}
