//go:build linux

package main

import (
	"runtime"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedcompat"
	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
)

func TestCollectorLaunchAdmission(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("the first connected worker profile is Linux/amd64")
	}
	identity := connectedidentity.Identity{
		Role: "collector", Version: "0.1.0-canary.1", Commit: strings.Repeat("a", 40),
		OS: "linux", Arch: "amd64", ContractSHA256: connectedcompat.Digest(),
	}
	record, err := connectedidentity.Encode(identity)
	if err != nil {
		t.Fatal(err)
	}
	environ := []string{"GODEBUG=netdns=go", "INVOCATION_ID=synthetic"}
	args := []string{"observer-connected-collector"}
	if got, err := validateLaunch(environ, record, args); err != nil || got != identity {
		t.Fatalf("valid launch refused: %v", err)
	}
	wrongRole := identity
	wrongRole.Role = "uploader"
	wrongRecord, err := connectedidentity.Encode(wrongRole)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		environ  []string
		identity string
		args     []string
	}{
		{"missing build identity", environ, "", args},
		{"wrong worker role", environ, wrongRecord, args},
		{"extra argument", environ, record, []string{args[0], "unexpected"}},
		{"missing DNS policy", []string{"INVOCATION_ID=synthetic"}, record, args},
		{"proxy override", append(append([]string{}, environ...), "HTTPS_PROXY=https://proxy.example"), record, args},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateLaunch(tc.environ, tc.identity, tc.args); err == nil {
				t.Fatal("unsafe launch admitted")
			}
		})
	}
}
