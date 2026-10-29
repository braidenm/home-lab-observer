//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/containerobs/model"
	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/sharedhandoff"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

type frameResult struct {
	section workloadprojection.ContainerSection
	err     error
	now     time.Time
}

func (f *frameResult) Read(_ context.Context, _ string, now time.Time) (workloadprojection.ContainerSection, error) {
	f.now = now
	return f.section, f.err
}

func TestDockerCollectorComposesTypedFramesAndNeverInventsHealthyEmpty(t *testing.T) {
	server := "srv_" + strings.Repeat("a", 32)
	at := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	processes := []observation.Process{{PID: 7, Name: "fixture-process", State: "running", CPUPercent: 125, MemoryBytes: 4096}}
	raw := observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: at, Processes: observation.Section[[]observation.Process]{State: observation.Available, Data: &processes, Quality: observation.SectionQuality{Samples: 1, Total: 1}}}
	section, err := workloadprojection.ProjectContainers(model.Inventory{SchemaVersion: model.SchemaVersion, ObservedAt: &at, SupportState: model.SupportSupported, CollectionState: model.CollectionOK, Freshness: model.FreshnessCurrent, TotalCount: 1, ReturnedCount: 1, Items: []model.Container{{IDAlias: "ctr_0123456789abcdef", Name: "fixture-container", Image: "fixture:1", State: model.StateExited, MetricsState: model.MetricsNotRunning}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range []error{nil, sharedhandoff.ErrUnavailable, sharedhandoff.ErrInvalid, sharedhandoff.ErrUnsafe, errors.New("synthetic-private-error")} {
		reader := &frameResult{section: section, err: failure}
		selected := readContainers(context.Background(), reader, server, at)
		body, err := encodeDocker(raw, server, "0.3.0-canary.1", selected)
		if err != nil || combinedprojection.Validate(body, server) != nil || !reader.now.Equal(at) {
			t.Fatal("typed Docker composition failed")
		}
		for _, required := range []string{`"processes_enabled":true`, `"containers_enabled":true`, `"cpu_percent":125`, `"cpu_measurement_basis":"LIFETIME_SINGLE_CORE"`} {
			if !bytes.Contains(body, []byte(required)) {
				t.Fatal("approved semantics missing")
			}
		}
		if failure == nil {
			if !bytes.Contains(body, []byte(`"name":"fixture-container"`)) {
				t.Fatal("fresh frame missing")
			}
		} else {
			if bytes.Contains(body, []byte("fixture-container")) || !bytes.Contains(body, []byte(`"reason_code":"COLLECTION_FAILED"`)) || bytes.Contains(body, []byte("synthetic-private-error")) {
				t.Fatal("failed frame became stale cached/healthy data or leaked an error")
			}
		}
	}
}

func TestDockerCollectorLaunchAndDependencyAuthorityAreClosed(t *testing.T) {
	id := connectedidentity.Identity{Role: "rich-docker-collector", Version: "0.3.0-canary.1", Commit: strings.Repeat("a", 40), OS: "linux", Arch: "amd64", ContractSHA256: connectedprofile.RichProcessesDockerDefinitionSHA256}
	encoded, err := connectedidentity.Encode(id)
	if err != nil {
		t.Fatal(err)
	}
	env := []string{"GODEBUG=netdns=go", "INVOCATION_ID=synthetic"}
	if _, err := validateLaunch(env, encoded, []string{"collector"}); err != nil {
		t.Fatal(err)
	}
	if _, err := validateLaunch(env, encoded, []string{"collector", "--profile=rich"}); err == nil {
		t.Fatal("caller selected profile")
	}
	for _, role := range []string{"rich-collector", "rich-docker-reader", "rich-docker-uploader", "rich-docker-install"} {
		other := id
		other.Role = role
		if role == "rich-collector" {
			other.ContractSHA256 = connectedprofile.RichProcessesDefinitionSHA256
		}
		b, err := connectedidentity.Encode(other)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := validateLaunch(env, b, []string{"collector"}); err == nil {
			t.Fatal("foreign role accepted")
		}
	}
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
			t.Fatal("unexpected collector authority", name)
		}
		if path == "net/http" {
			t.Fatal("collector acquired HTTP/socket adapter")
		}
	}
}
