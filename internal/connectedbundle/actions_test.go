package connectedbundle

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestActionBundleExactMembership(t *testing.T) {
	dir, raw, digest := buildNamedRichFixture(t, actionBundleSpec, CreateActionManifest)
	m, err := VerifyActionDirectory(dir, digest)
	if err != nil || len(m.Files) != 5 {
		t.Fatalf("exact bundle refused: %v", err)
	}
	if _, err := DecodeRichProcessesDocker(raw); err == nil {
		t.Fatal("observation profile accepted an action bundle")
	}
	var first, second bytes.Buffer
	if WriteActionArchive(&first, dir, digest) != nil || WriteActionArchive(&second, dir, digest) != nil ||
		!bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("action archive not deterministic")
	}
	if err := os.WriteFile(filepath.Join(dir, "home-lab-observer-action-worker.service.tmpl"), []byte("foreign"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyActionDirectory(dir, digest); err == nil {
		t.Fatal("altered service resource accepted")
	}
}
