package releasepack

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/buildidentity"
)

func TestInstalledSchemaIsCodeOwnedAndLegacyV1RemainsValid(t *testing.T) {
	config := testConfig(t)
	manifest, err := Build(config)
	if err != nil {
		t.Fatal(err)
	}
	// Non-Linux case is essential: absence of a helper digest does not identify v1.
	asset := manifest.Assets[4]
	directory := t.TempDir()
	for name, source := range map[string]string{"observer.exe": filepath.Join(config.BinariesDir, targets[4].binaryName), "LICENSE": filepath.Join(config.ResourcesDir, "LICENSE"), "START-HERE.md": filepath.Join(config.ResourcesDir, "START-HERE.md"), "Run-Observer.cmd": filepath.Join(config.ResourcesDir, "Run-Observer.cmd")} {
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, name), data, 0700); err != nil {
			t.Fatal(err)
		}
	}
	input := Installed{ExpectedSchemaVersion: SchemaVersion, ManifestPath: filepath.Join(config.OutputDir, ManifestName), ExecutablePath: filepath.Join(directory, "observer.exe"), Identity: buildidentity.Identity{Role: "observer", Version: config.Version, Commit: config.Commit, OS: "windows", Arch: "amd64"}, ArchiveSHA256: asset.SHA256, ArchiveSize: asset.SizeBytes}
	if err := VerifyInstalled(input); err != nil {
		t.Fatal("legacy v1 failed")
	}
	for _, expected := range []string{SchemaVersionV2, "", "unknown"} {
		input.ExpectedSchemaVersion = expected
		if VerifyInstalled(input) == nil {
			t.Fatal("schema downgrade or unspecified contract accepted")
		}
	}
}
