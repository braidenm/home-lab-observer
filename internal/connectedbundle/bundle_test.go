package connectedbundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedcompat"
	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
)

func fakeManifest() Manifest {
	m := Manifest{Schema: Version, Version: "0.1.0-canary.1", Commit: strings.Repeat("a", 40), OS: "linux", Arch: "amd64", Profile: Profile}
	for _, name := range Names() {
		mode := uint32(0o644)
		if _, ok := binaries[name]; ok {
			mode = 0o755
		}
		m.Files = append(m.Files, File{name, strings.Repeat("b", 64), 1, mode})
	}
	return m
}

func TestClosedManifest(t *testing.T) {
	m := fakeManifest()
	data, err := Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{append(append([]byte(nil), data...), '\n'), []byte(strings.Replace(string(data), `"schema":`, `"other":0,"schema":`, 1)), []byte(strings.Replace(string(data), `"schema":`, `"schema":"old","schema":`, 1)), []byte(strings.Replace(string(data), `"schema":`, `"Schema":`, 1)), make([]byte, MaxManifestBytes+1), nil} {
		if _, err := Decode(bad); err != ErrInvalid {
			t.Fatal("noncanonical accepted")
		}
	}
	for _, change := range []func(*Manifest){func(m *Manifest) { m.Schema = "observer-release/v2" }, func(m *Manifest) { m.Arch = "arm64" }, func(m *Manifest) { m.Files = m.Files[:len(m.Files)-1] }, func(m *Manifest) { m.Files[0].Name = "../outside" }, func(m *Manifest) { m.Files[0].Mode = 0o4755 }, func(m *Manifest) { m.Files[0].Size = connectedidentity.MaxFileBytes + 1 }, func(m *Manifest) { m.Files[0].SHA256 = "bad" }, func(m *Manifest) { m.Files[0], m.Files[1] = m.Files[1], m.Files[0] }} {
		copy := fakeManifest()
		change(&copy)
		if _, err := Encode(copy); err != ErrInvalid {
			t.Fatal("invalid manifest")
		}
	}
}

// Three tiny Go command fixtures are cross-built and never executed. This tests
// actual ELF/buildinfo/identity integration, not the final workers' behavior.
func TestRealGoBundleVerification(t *testing.T) {
	for _, schema := range []string{Version, VersionV2} {
		t.Run(schema, func(t *testing.T) { testRealGoBundleVerification(t, schema) })
	}
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	// macOS reports its temporary root through /var -> /private/var. The
	// production verifier must reject symlinked ancestors, so point this
	// fixture at the same directory through its actual, link-free path.
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func testRealGoBundleVerification(t *testing.T, schema string) {
	goExe, err := exec.LookPath("go")
	if err != nil {
		t.Fatal("Go compiler required")
	}
	source := t.TempDir()
	bundle := canonicalTempDir(t)
	m := fakeManifest()
	m.Schema = schema
	if schema == VersionV2 {
		m.ContractSHA256 = connectedcompat.Digest()
	}
	if os.WriteFile(filepath.Join(source, "go.mod"), []byte("module github.com/braidenm/home-lab-observer\n\ngo 1.27.1\n"), 0o644) != nil {
		t.Fatal("fixture module")
	}
	for name, role := range binaries {
		dir := filepath.Join(source, "cmd", name)
		if os.MkdirAll(dir, 0o755) != nil {
			t.Fatal("fixture directory")
		}
		code := []byte("package main\nimport \"fmt\"\nvar releaseIdentity string\nfunc main(){fmt.Print(releaseIdentity)}\n")
		if os.WriteFile(filepath.Join(dir, "main.go"), code, 0o644) != nil {
			t.Fatal("fixture source")
		}
		record, _ := connectedidentity.Encode(connectedidentity.Identity{Role: role, Version: m.Version, Commit: m.Commit, OS: "linux", Arch: "amd64", ContractSHA256: m.ContractSHA256})
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		cmd := exec.CommandContext(ctx, goExe, "build", "-trimpath", "-ldflags=-s -w -X main.releaseIdentity="+record, "-o", filepath.Join(bundle, name), "./cmd/"+name)
		cmd.Dir = source
		for _, env := range os.Environ() {
			if !strings.HasPrefix(env, "GOOS=") && !strings.HasPrefix(env, "GOARCH=") && !strings.HasPrefix(env, "CGO_ENABLED=") && !strings.HasPrefix(env, "GOTOOLCHAIN=") {
				cmd.Env = append(cmd.Env, env)
			}
		}
		cmd.Env = append(cmd.Env, "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0", "GOTOOLCHAIN=local")
		buildErr := cmd.Run()
		cancel()
		if buildErr != nil {
			t.Fatal("synthetic Go build failed")
		}
	}
	for name, data := range connectedunits.Resources() {
		if os.WriteFile(filepath.Join(bundle, name), data, 0o644) != nil {
			t.Fatal("fixture resource")
		}
	}
	var manifest []byte
	if schema == VersionV2 {
		manifest, err = CreateManifest(bundle, m.Version, m.Commit)
	} else {
		manifest, err = createManifest(bundle, m.Version, m.Commit, m.Schema, m.ContractSHA256)
	}
	if err != nil {
		t.Fatal(err)
	}
	if os.WriteFile(filepath.Join(bundle, ManifestName), manifest, 0o644) != nil {
		t.Fatal("manifest write")
	}
	manifestSHA := digest(manifest)
	if _, err := VerifyDirectory(bundle, manifestSHA); err != nil {
		t.Fatal(err)
	}
	if schema == VersionV2 {
		built, err := CreateManifest(bundle, m.Version, m.Commit)
		// The completed bundle contains a manifest, so build-only exact-input
		// validation must refuse it rather than overwrite/repackage implicitly.
		if err != ErrInvalid || built != nil {
			t.Fatal("completed bundle accepted as build input")
		}
	}
	var first, second bytes.Buffer
	if WriteArchive(&first, bundle, manifestSHA) != nil || WriteArchive(&second, bundle, manifestSHA) != nil || !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("archive nondeterministic")
	}
	gz, err := gzip.NewReader(bytes.NewReader(first.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	count := 0
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		count++
		if header.Typeflag != tar.TypeReg || header.Uid != 0 || header.Gid != 0 || strings.Contains(header.Name, "/") {
			t.Fatal("archive metadata")
		}
	}
	if count != len(Names())+1 {
		t.Fatal("archive file count")
	}
	for _, mutation := range []string{"missing", "extra", "linked", "symlink", "resource", "mixed-role", "mixed-commit", "malformed-manifest", "manifest-mode", "actual-manifest-mode", "wrong-digest", "mixed-contract", "unknown-contract"} {
		t.Run(mutation, func(t *testing.T) {
			dir := canonicalTempDir(t)
			for _, name := range append(Names(), ManifestName) {
				data, err := os.ReadFile(filepath.Join(bundle, name))
				if err != nil {
					t.Fatal(err)
				}
				mode := os.FileMode(0o644)
				if _, ok := binaries[name]; ok {
					mode = 0o755
				}
				if os.WriteFile(filepath.Join(dir, name), data, mode) != nil {
					t.Fatal("copy")
				}
			}
			expected := manifestSHA
			switch mutation {
			case "mixed-contract", "unknown-contract":
				changed, _ := Decode(manifest)
				if changed.Schema == Version {
					changed.Schema = VersionV2
					changed.ContractSHA256 = connectedcompat.Digest()
				} else {
					changed.Schema = Version
					changed.ContractSHA256 = ""
				}
				if mutation == "unknown-contract" {
					changed.Schema = VersionV2
					changed.ContractSHA256 = strings.Repeat("f", 64)
				}
				data, _ := json.Marshal(changed)
				if os.WriteFile(filepath.Join(dir, ManifestName), data, 0o644) != nil {
					t.Fatal("changed contract fixture")
				}
				expected = digest(data)
			case "missing":
				os.Remove(filepath.Join(dir, "observer-connected-collector"))
			case "extra":
				os.WriteFile(filepath.Join(dir, "extra"), nil, 0o644)
			case "linked":
				if os.Link(filepath.Join(dir, "observer-connected-collector"), filepath.Join(t.TempDir(), "link")) != nil {
					t.Fatal("hardlink fixture")
				}
			case "symlink":
				if runtime.GOOS == "windows" {
					t.Skip("symlink requires additional Windows privilege")
				}
				os.Remove(filepath.Join(dir, "observer-connected-collector"))
				if os.Symlink("observer-connected-uploader", filepath.Join(dir, "observer-connected-collector")) != nil {
					t.Fatal("symlink fixture")
				}
			case "resource":
				os.WriteFile(filepath.Join(dir, "enrollment.properties.tmpl"), []byte("ExecStart=/bin/sh\n"), 0o644)
			case "mixed-role":
				data, _ := os.ReadFile(filepath.Join(dir, "observer-connected-uploader"))
				os.WriteFile(filepath.Join(dir, "observer-connected-collector"), data, 0o755)
			case "mixed-commit", "manifest-mode":
				changed, _ := Decode(manifest)
				if mutation == "mixed-commit" {
					changed.Commit = strings.Repeat("c", 40)
				} else {
					changed.Files[0].Mode = 0o777
				}
				data, _ := json.Marshal(changed)
				os.WriteFile(filepath.Join(dir, ManifestName), data, 0o644)
				expected = digest(data)
			case "malformed-manifest":
				os.WriteFile(filepath.Join(dir, ManifestName), []byte("{}"), 0o644)
				expected = digest([]byte("{}"))
			case "wrong-digest":
				expected = strings.Repeat("0", 64)
			case "actual-manifest-mode":
				if runtime.GOOS == "windows" {
					t.Skip("Windows does not preserve POSIX modes")
				}
				if os.Chmod(filepath.Join(dir, ManifestName), 0o600) != nil {
					t.Fatal("manifest mode fixture")
				}
			}
			// Preserve byte/hash consistency so these cases specifically exercise
			// binary role and reviewed-template authority, not just digest mismatch.
			if mutation == "mixed-role" || mutation == "resource" {
				changed, _ := Decode(manifest)
				name := "observer-connected-collector"
				if mutation == "resource" {
					name = "enrollment.properties.tmpl"
				}
				data, _ := os.ReadFile(filepath.Join(dir, name))
				for i := range changed.Files {
					if changed.Files[i].Name == name {
						changed.Files[i].SHA256 = digest(data)
						changed.Files[i].Size = int64(len(data))
					}
				}
				data, _ = Encode(changed)
				os.WriteFile(filepath.Join(dir, ManifestName), data, 0o644)
				expected = digest(data)
			}
			if _, err := VerifyDirectory(dir, expected); err != ErrInvalid {
				t.Fatal("hostile bundle accepted")
			}
		})
	}
}
