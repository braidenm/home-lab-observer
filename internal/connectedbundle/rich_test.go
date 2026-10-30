package connectedbundle

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
)

func buildRichProcessesFixture(t *testing.T) (string, []byte, string) {
	return buildNamedRichFixture(t, richProcessesBundleSpec, CreateRichProcessesManifest)
}

func buildNamedRichFixture(t *testing.T, spec bundleSpec, create func(string, string, string) ([]byte, error)) (string, []byte, string) {
	t.Helper()
	goExe, err := exec.LookPath("go")
	if err != nil {
		t.Fatal("Go compiler required")
	}
	source := t.TempDir()
	bundle := canonicalTempDir(t)
	const version = "0.1.0-canary.1"
	commit := strings.Repeat("a", 40)
	if os.WriteFile(filepath.Join(source, "go.mod"), []byte("module github.com/braidenm/home-lab-observer\n\ngo 1.27.1\n"), 0o600) != nil {
		t.Fatal("fixture module")
	}
	for name, role := range spec.binaries {
		directory := filepath.Join(source, "cmd", name)
		if os.MkdirAll(directory, 0o700) != nil || os.WriteFile(filepath.Join(directory, "main.go"), []byte("package main\nimport \"fmt\"\nvar releaseIdentity string\nfunc main(){fmt.Print(releaseIdentity)}\n"), 0o600) != nil {
			t.Fatal("fixture source")
		}
		record, err := connectedidentity.Encode(connectedidentity.Identity{Role: role, Version: version, Commit: commit, OS: "linux", Arch: "amd64", ContractSHA256: spec.contract})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		command := exec.CommandContext(ctx, goExe, "build", "-trimpath", "-ldflags=-s -w -X main.releaseIdentity="+record, "-o", filepath.Join(bundle, name), "./cmd/"+name)
		command.Dir = source
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "GOOS=") && !strings.HasPrefix(entry, "GOARCH=") && !strings.HasPrefix(entry, "CGO_ENABLED=") && !strings.HasPrefix(entry, "GOTOOLCHAIN=") {
				command.Env = append(command.Env, entry)
			}
		}
		command.Env = append(command.Env, "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0", "GOTOOLCHAIN=local")
		if output, err := command.CombinedOutput(); err != nil {
			cancel()
			t.Fatalf("fixture build: %v %s", err, output)
		}
		cancel()
	}
	for name, data := range spec.resources() {
		if os.WriteFile(filepath.Join(bundle, name), data, 0o644) != nil {
			t.Fatal("fixture resource")
		}
	}
	manifest, err := create(bundle, version, commit)
	if err != nil || os.WriteFile(filepath.Join(bundle, ManifestName), manifest, 0o644) != nil {
		t.Fatalf("manifest: %v", err)
	}
	return bundle, manifest, digest(manifest)
}

func TestRichProcessesBundleIsClosedAndSeparate(t *testing.T) {
	bundle, manifest, manifestSHA := buildRichProcessesFixture(t)
	m, err := DecodeRichProcesses(manifest)
	if err != nil || !m.HasKnownRichProcessesContract() || m.HasKnownContract() {
		t.Fatalf("rich contract = %#v, %v", m, err)
	}
	if _, err := Decode(manifest); err != ErrInvalid {
		t.Fatal("numeric decoder accepted rich package")
	}
	if _, err := VerifyRichProcessesDirectory(bundle, manifestSHA); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyDirectory(bundle, manifestSHA); err != ErrInvalid {
		t.Fatal("numeric verifier accepted rich package")
	}
	var first, second bytes.Buffer
	if WriteRichProcessesArchive(&first, bundle, manifestSHA) != nil || WriteRichProcessesArchive(&second, bundle, manifestSHA) != nil || !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("rich archive is not deterministic")
	}
	changed := m
	changed.ContractSHA256 = strings.Repeat("f", 64)
	if _, err := EncodeRichProcesses(changed); err != ErrInvalid {
		t.Fatal("unknown rich definition admitted")
	}
}
