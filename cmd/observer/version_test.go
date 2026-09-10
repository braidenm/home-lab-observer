package main

import (
	"encoding/json"
	"io"
	"runtime"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/buildidentity"
)

func TestVersionDoesNotStartCollectionOrService(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"version", "--json"}, {"--version"}} {
		var out, errOut strings.Builder
		got := runCommand(args, &out, &errOut, nil, func([]string, io.Writer, io.Writer) int {
			t.Fatal("version started service")
			return 1
		})
		if got != 0 || errOut.Len() != 0 {
			t.Fatalf("version failed: code=%d error=%s", got, errOut.String())
		}
		if len(args) > 1 {
			var info buildVersion
			if json.Unmarshal([]byte(out.String()), &info) != nil || info.SchemaVersion != "observer-build/v1" || info.Version != version || info.Commit != commit || info.OS != runtime.GOOS || info.Arch != runtime.GOARCH {
				t.Fatalf("invalid build description: %s", out.String())
			}
		} else if !strings.Contains(out.String(), "Home Lab Observer "+version) {
			t.Fatalf("missing readable version: %s", out.String())
		}
	}
}

func TestVersionUsesAuthoritativeRecordWithoutChangingWire(t *testing.T) {
	previous := releaseIdentity
	defer func() { releaseIdentity = previous }()
	i := buildidentity.Identity{Role: "observer", Version: "0.1.0-preview.99", Commit: strings.Repeat("a", 40), OS: runtime.GOOS, Arch: runtime.GOARCH}
	if runtime.GOOS == "linux" {
		i.HelperSHA256 = strings.Repeat("b", 64)
	}
	var err error
	releaseIdentity, err = buildidentity.Encode(i)
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut strings.Builder
	if runVersion([]string{"--json"}, &out, &errOut) != 0 {
		t.Fatal("record version failed")
	}
	var decoded map[string]any
	if json.Unmarshal([]byte(out.String()), &decoded) != nil || len(decoded) != 6 || decoded["version"] != i.Version || decoded["commit"] != i.Commit {
		t.Fatal("record identity or closed wire changed")
	}
	out.Reset()
	errOut.Reset()
	if runVersion([]string{"--release-schema"}, &out, &errOut) != 0 || out.String() != "observer-release/v2\n" {
		t.Fatal("v2 schema probe failed")
	}
	if runVersion([]string{"--release-schema", "--json"}, io.Discard, io.Discard) != 2 {
		t.Fatal("conflicting schema mode accepted")
	}
	releaseIdentity = "private-canary-invalid"
	out.Reset()
	errOut.Reset()
	if runVersion(nil, &out, &errOut) != 1 || out.Len() != 0 || errOut.String() != "BUILD_IDENTITY_INVALID\n" {
		t.Fatal("invalid record fell back or leaked")
	}
}

func TestNoArgumentsUsesForegroundServe(t *testing.T) {
	called := false
	got := runCommand(nil, io.Discard, io.Discard, nil, func(args []string, _, _ io.Writer) int {
		called = true
		if len(args) != 0 {
			t.Fatalf("default serve received implicit options: %v", args)
		}
		return 19
	})
	if !called || got != 19 {
		t.Fatal("no-argument start did not preserve foreground service status")
	}
}

func TestVersionRejectsUnexpectedOptions(t *testing.T) {
	for _, args := range [][]string{{"version", "private-extra"}, {"version", "--unknown"}, {"version", "--release-manifest", "test"}, {"version", "--archive-size", "1"}, {"version", "--release-manifest", "test", "--archive-sha256", strings.Repeat("a", 64), "--archive-size", "1", "--json"}} {
		if run(args, io.Discard, io.Discard, nil) != 2 {
			t.Fatal("invalid version options were accepted")
		}
	}
	var errorsOut strings.Builder
	if runVersion([]string{"--archive-size", "private-path-canary"}, io.Discard, &errorsOut) != 2 || errorsOut.String() != "invalid version options\n" {
		t.Fatal("argument parser leaked input")
	}
}
