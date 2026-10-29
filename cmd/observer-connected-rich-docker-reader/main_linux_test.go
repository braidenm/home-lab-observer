//go:build linux

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func TestReaderLaunchRequiresExactRoleAndResourceEnvironment(t *testing.T) {
	i := connectedidentity.Identity{Role: "rich-docker-reader", Version: "0.1.0-canary.3", Commit: strings.Repeat("a", 40), OS: "linux", Arch: "amd64", ContractSHA256: connectedprofile.RichProcessesDockerDefinitionSHA256}
	marker, err := connectedidentity.Encode(i)
	if err != nil {
		t.Fatal(err)
	}
	env := []string{"GODEBUG=netdns=go", "GOMAXPROCS=2", "GOMEMLIMIT=64MiB"}
	if validateLaunch(env, marker, []string{"reader"}) != nil {
		t.Fatal("valid launch refused")
	}
	for _, role := range []string{"rich-docker-uploader", "rich-docker-collector", "rich-collector", "collector"} {
		wrong := i
		wrong.Role = role
		if role == "rich-collector" {
			wrong.ContractSHA256 = connectedprofile.RichProcessesDefinitionSHA256
		}
		if role == "collector" {
			wrong.ContractSHA256 = ""
		}
		m, err := connectedidentity.Encode(wrong)
		if err != nil {
			t.Fatal(err)
		}
		if validateLaunch(env, m, []string{"reader"}) == nil {
			t.Fatal("foreign binary role accepted")
		}
	}
	if validateLaunch(env, marker, []string{"reader", "--endpoint=private"}) == nil || validateLaunch(nil, marker, []string{"reader"}) == nil || validateLaunch(env, "", []string{"reader"}) == nil {
		t.Fatal("unsafe launch accepted")
	}
	// The actual test process has no installed release identity: refuse before
	// reading any fixed host path or opening an Engine connection.
	if run(context.Background()) != 22 {
		t.Fatal("unpackaged launch admitted")
	}
}
