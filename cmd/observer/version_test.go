package main

import (
	"encoding/json"
	"io"
	"runtime"
	"strings"
	"testing"
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
	for _, args := range [][]string{{"version", "private-extra"}, {"version", "--unknown"}} {
		if run(args, io.Discard, io.Discard, nil) != 2 {
			t.Fatal("invalid version options were accepted")
		}
	}
}
