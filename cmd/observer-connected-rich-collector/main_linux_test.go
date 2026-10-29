//go:build linux

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/observation"
)

func TestRichCollectorProducesOnlyApprovedGroups(t *testing.T) {
	server := "srv_0123456789abcdef0123456789abcdef"
	items := []observation.Process{{PID: 123, Name: "fixture-app", CPUPercent: 125, State: "running", MemoryBytes: 4096, UptimeSeconds: 60}}
	raw := observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC), Processes: observation.Section[[]observation.Process]{State: observation.Available, Data: &items, Quality: observation.SectionQuality{Samples: 1, Total: 1}}}
	body, err := encodeProcesses(raw, server, "0.2.0-canary.1")
	if err != nil || combinedprojection.Validate(body, server) != nil {
		t.Fatal("rich collector composition refused")
	}
	for _, required := range []string{`"processes_enabled":true`, `"containers_enabled":false`, `"cpu_percent":125`, `"name":"fixture-app"`, `"cpu_measurement_basis":"LIFETIME_SINGLE_CORE"`} {
		if !bytes.Contains(body, []byte(required)) {
			t.Fatal("approved process semantics lost")
		}
	}
	for _, forbidden := range []string{`"account"`, `"argv"`, `"environment"`, `"full_id"`, `"docker_socket"`} {
		if bytes.Contains(body, []byte(forbidden)) {
			t.Fatal("extra collection field escaped")
		}
	}
}

func TestRichCollectorLaunchRequiresExactRoleAndDefinition(t *testing.T) {
	id := connectedidentity.Identity{Role: "rich-collector", Version: "0.2.0-canary.1", Commit: strings.Repeat("a", 40), OS: "linux", Arch: "amd64", ContractSHA256: connectedprofile.RichProcessesDefinitionSHA256}
	encoded, err := connectedidentity.Encode(id)
	if err != nil {
		t.Fatal("identity fixture refused")
	}
	env := []string{"GODEBUG=netdns=go", "INVOCATION_ID=synthetic"}
	args := []string{"observer-connected-rich-collector"}
	if _, err := validateLaunch(env, encoded, args); err != nil {
		t.Fatal("valid rich launch refused")
	}
	for _, role := range []string{"rich-uploader", "rich-install"} {
		other := id
		other.Role = role
		record, err := connectedidentity.Encode(other)
		if err != nil {
			t.Fatal("other role fixture refused")
		}
		if _, err := validateLaunch(env, record, args); err == nil {
			t.Fatal("other rich role launched collector")
		}
	}
	if _, err := validateLaunch(env, encoded, append(args, "--profile=numeric")); err == nil {
		t.Fatal("caller-selected profile admitted")
	}
	if _, err := validateLaunch(append(env, "HTTPS_PROXY=https://proxy.example"), encoded, args); err == nil {
		t.Fatal("proxy override admitted")
	}
}

func TestRichCollectorHasNoCredentialOrDockerAdapterDependencies(t *testing.T) {
	allowed := map[string]bool{}
	for _, name := range strings.Fields("collector combinedprojection connectedcompat connectedidentity connectedprocess connectedprofile connectedruntime connectedstatus containerobs/model numerichost observation ownerfs remoteprojection sharedhandoff workloadprojection") {
		allowed[name] = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-f", "{{.ImportPath}}", ".")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	data, err := cmd.Output()
	if err != nil {
		t.Fatal("dependency inspection failed")
	}
	for _, path := range strings.Fields(string(data)) {
		if name, ok := strings.CutPrefix(path, "github.com/braidenm/home-lab-observer/internal/"); ok && !allowed[name] {
			t.Fatalf("unexpected collector dependency: %s", name)
		}
		if path == "net/http" {
			t.Fatal("collector acquired HTTP transport")
		}
	}
}
