package connectedbundle

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDockerBundleHasFourExactRolesAndSeparateContract(t *testing.T) {
	dir, b, sha := buildNamedRichFixture(t, richProcessesDockerBundleSpec, CreateRichProcessesDockerManifest)
	m, err := DecodeRichProcessesDocker(b)
	if err != nil || !m.HasKnownRichProcessesDockerContract() || m.HasKnownContract() || m.HasKnownRichProcessesContract() {
		t.Fatal("Docker bundle contract")
	}
	if len(m.Files) != 10 {
		t.Fatal("Docker bundle membership")
	}
	if _, err := VerifyRichProcessesDockerDirectory(dir, sha); err != nil {
		t.Fatal("Docker exact bundle refused", err)
	}
	for _, decode := range []func([]byte) (Manifest, error){Decode, DecodeRichProcesses} {
		if _, err := decode(b); err != ErrInvalid {
			t.Fatal("old decoder widened")
		}
	}
	var first, second bytes.Buffer
	if WriteRichProcessesDockerArchive(&first, dir, sha) != nil || WriteRichProcessesDockerArchive(&second, dir, sha) != nil || !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("Docker archive determinism")
	}
	if err := os.WriteFile(filepath.Join(dir, "foreign"), []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyRichProcessesDockerDirectory(dir, sha); err != ErrInvalid {
		t.Fatal("extra bundle member admitted")
	}
	if err := os.Remove(filepath.Join(dir, "foreign")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "observer-connected-rich-docker-reader")); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyRichProcessesDockerDirectory(dir, sha); err != ErrInvalid {
		t.Fatal("missing socket reader admitted")
	}
}
