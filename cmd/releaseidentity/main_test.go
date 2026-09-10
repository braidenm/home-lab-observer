package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/buildidentity"
)

func TestFormatterUsesCanonicalClosedIdentity(t *testing.T) {
	args := []string{"--role", "journal-helper", "--version", "0.1.0-preview.99", "--commit", strings.Repeat("a", 40), "--os", "linux", "--arch", "amd64"}
	var output bytes.Buffer
	if run(args, &output, io.Discard) != 0 {
		t.Fatal("canonical identity rejected")
	}
	if _, err := buildidentity.Parse(strings.TrimSuffix(output.String(), "\n")); err != nil {
		t.Fatal("formatter differs from runtime")
	}
	for _, invalid := range [][]string{nil, append(append([]string{}, args...), "extra"), {"--role", "arbitrary-command"}} {
		output.Reset()
		if run(invalid, &output, io.Discard) == 0 || output.Len() != 0 {
			t.Fatal("invalid identity produced linker data")
		}
	}
}

func TestFormatterScansOneRegularBoundedBinary(t *testing.T) {
	identity := buildidentity.Identity{Role: "observer", Version: "0.1.0-preview.99", Commit: strings.Repeat("a", 40), OS: "linux", Arch: "amd64", HelperSHA256: strings.Repeat("b", 64)}
	record, err := buildidentity.Encode(identity)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "observer")
	if err := os.WriteFile(path, []byte("binary-prefix\x00"+record+"\x00binary-suffix"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output, diagnostics bytes.Buffer
	if code := run([]string{"--scan", path}, &output, &diagnostics); code != 0 || strings.TrimSpace(output.String()) != record || diagnostics.Len() != 0 {
		t.Fatalf("scan code/output/diagnostics = %d %q %q", code, output.String(), diagnostics.String())
	}
	invalid := []struct {
		name       string
		args       []string
		fixedError bool
	}{
		{name: "extra argument", args: []string{"--scan", path, "extra"}},
		{name: "mixed formatter mode", args: []string{"--scan", path, "--role", "observer"}, fixedError: true},
		{name: "missing input", args: []string{"--scan", filepath.Join(t.TempDir(), "missing")}, fixedError: true},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			output.Reset()
			diagnostics.Reset()
			code := run(test.args, &output, &diagnostics)
			if code == 0 || output.Len() != 0 {
				t.Fatalf("invalid scan accepted: %#v code=%d out=%q diagnostics=%q", test.args, code, output.String(), diagnostics.String())
			}
			if test.fixedError && diagnostics.String() != "BUILD_IDENTITY_INVALID\n" {
				t.Fatalf("scan diagnostic = %q, want fixed code", diagnostics.String())
			}
		})
	}
}
