package combinedprojection_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
	"github.com/braidenm/home-lab-observer/internal/containerobs"
	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

const server = "srv_0123456789abcdef0123456789abcdef"

func component(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../schemas/remote/fixtures/" + name + ".json")
	if err != nil {
		t.Fatal("synthetic fixture unavailable")
	}
	return bytes.TrimSpace(b)
}

func components(t *testing.T, workload string) ([]byte, []byte) {
	t.Helper()
	host := component(t, "native-host-complete")
	host = bytes.ReplaceAll(host, []byte("0.1.0-preview.3"), []byte("0.2.0-canary.1"))
	host = bytes.ReplaceAll(host, []byte("2026-09-10T12:00:00Z"), []byte("2026-10-29T12:00:00Z"))
	return host, component(t, workload)
}

func TestCombinedPreservesBothComponentsWithoutAdmittingUpload(t *testing.T) {
	for _, fixture := range []string{"workloads-disabled", "workloads-partial"} {
		t.Run(fixture, func(t *testing.T) {
			host, workloads := components(t, fixture)
			body, err := combinedprojection.Encode(host, workloads, server)
			if err != nil || combinedprojection.Validate(body, server) != nil {
				t.Fatal("combined roundtrip refused")
			}
			var d struct {
				Schema    string          `json:"schema_version"`
				Profile   string          `json:"projection_profile"`
				Host      json.RawMessage `json:"host_metrics"`
				Workloads json.RawMessage `json:"workload_inventory"`
			}
			if json.Unmarshal(body, &d) != nil || d.Schema != "home-lab-connected-observation/v1" || d.Profile != "numeric-host-workloads/v1" || !bytes.Equal(d.Host, host) || !bytes.Equal(d.Workloads, workloads) {
				t.Fatal("component bytes or closed envelope identity changed")
			}
			at, err := combinedprojection.CollectionTime(body, server)
			if err != nil || !at.Equal(time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)) {
				t.Fatal("wrong validated collection time")
			}
			if remoteprojection.ValidateUpload(body, server) == nil {
				t.Fatal("contract acquired current upload admission")
			}
		})
	}
}

func TestCombinedRejectsInvalidOrMismatchedComponents(t *testing.T) {
	host, workloads := components(t, "workloads-partial")
	for name, mutate := range map[string]func([]byte) []byte{
		"source": func(b []byte) []byte {
			return bytes.ReplaceAll(b, []byte(server), []byte("srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
		},
		"version": func(b []byte) []byte { return bytes.ReplaceAll(b, []byte("0.2.0-canary.1"), []byte("0.2.1-canary.1")) },
		"cycle": func(b []byte) []byte {
			return bytes.Replace(b, []byte("2026-10-29T12:00:00Z"), []byte("2026-10-29T12:00:01Z"), 1)
		},
		"missing":      func([]byte) []byte { return nil },
		"trailing":     func(b []byte) []byte { return append(b, ' ') },
		"unknown":      func(b []byte) []byte { return append([]byte(`{"extra":true,`), b[1:]...) },
		"oversized":    func([]byte) []byte { return bytes.Repeat([]byte("x"), 262145) },
		"invalid UTF8": func(b []byte) []byte { return append(b[:len(b):len(b)], 0xff) },
	} {
		t.Run(name, func(t *testing.T) {
			for _, changeHost := range []bool{true, false} {
				h, w := bytes.Clone(host), bytes.Clone(workloads)
				if changeHost {
					h = mutate(h)
				} else {
					w = mutate(w)
				}
				body, err := combinedprojection.Encode(h, w, server)
				if err != combinedprojection.ErrValidation || body != nil {
					t.Fatal("invalid component encoded or returned partial bytes")
				}
			}
		})
	}
	if body, err := combinedprojection.Encode(host, workloads, "invalid"); err != combinedprojection.ErrValidation || body != nil {
		t.Fatal("invalid expected identity admitted")
	}
}

func TestCombinedAllowsSectionSpecificTimes(t *testing.T) {
	host, workloads := components(t, "workloads-partial")
	// Only the root cycle must match. Per-section measurements retain their own times.
	workloads = bytes.Replace(workloads, []byte(`"observed_at":"2026-10-29T12:00:00Z"`), []byte(`"observed_at":"2026-10-29T11:59:59Z"`), 1)
	body, err := combinedprojection.Encode(host, workloads, server)
	if err != nil || !bytes.Contains(body, []byte(`"observed_at":"2026-10-29T11:59:59Z"`)) {
		t.Fatal("section time rewritten or incorrectly bound to root cycle")
	}
}

func TestLargeBoundedBodyDoesNotWidenInstalledAdmission(t *testing.T) {
	host, _ := components(t, "workloads-disabled")
	at := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	raw := observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: at}
	docker := containerobs.Inventory{SchemaVersion: containerobs.SchemaVersion, ObservedAt: &at,
		SupportState: containerobs.SupportSupported, CollectionState: containerobs.CollectionOK,
		Freshness: containerobs.FreshnessCurrent, TotalCount: 500, ReturnedCount: 500}
	for i := range 500 {
		docker.Items = append(docker.Items, containerobs.Container{IDAlias: fmt.Sprintf("ctr_%016x", i),
			Name: strings.Repeat("n", 128), Image: strings.Repeat("i", 240),
			State: containerobs.StateExited, MetricsState: containerobs.MetricsNotRunning})
	}
	workloads, err := workloadprojection.Encode(raw, docker, workloadprojection.Identity{SourceID: server, Version: "0.2.0-canary.1"}, workloadprojection.Consent{Containers: true})
	if err != nil {
		t.Fatal("large synthetic workload refused")
	}
	body, err := combinedprojection.Encode(host, workloads, server)
	if err != nil || len(body) <= remoteprojection.MaxBytes || len(body) > combinedprojection.MaxBytes || combinedprojection.Validate(body, server) != nil {
		t.Fatal("large bounded envelope failed")
	}
	if remoteprojection.ValidateUpload(body, server) == nil {
		t.Fatal("larger contract silently widened installed admission")
	}
	for _, pair := range [][2][]byte{
		{bytes.Repeat([]byte(" "), remoteprojection.MaxBytes+1), workloads},
		{host, bytes.Repeat([]byte(" "), workloadprojection.MaxBytes+1)},
	} {
		if out, err := combinedprojection.Encode(pair[0], pair[1], server); out != nil || err != combinedprojection.ErrValidation {
			t.Fatal("individual component byte budget ignored")
		}
	}
}

func TestCombinedValidatorRejectsNoncanonicalOrInconsistentEnvelope(t *testing.T) {
	host, workloads := components(t, "workloads-disabled")
	body, err := combinedprojection.Encode(host, workloads, server)
	if err != nil {
		t.Fatal("fixture encode refused")
	}
	for name, mutate := range map[string]func([]byte) []byte{
		"whitespace":        func(b []byte) []byte { return append(b, '\n') },
		"unknown":           func(b []byte) []byte { return append([]byte(`{"extra":0,`), b[1:]...) },
		"duplicate":         func(b []byte) []byte { return append([]byte(`{"source_id":"`+server+`",`), b[1:]...) },
		"escaped duplicate": func(b []byte) []byte { return append([]byte(`{"source_\u0069d":"`+server+`",`), b[1:]...) },
		"root source": func(b []byte) []byte {
			return bytes.Replace(b, []byte(server), []byte("srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), 1)
		},
		"root cycle": func(b []byte) []byte {
			return bytes.Replace(b, []byte("2026-10-29T12:00:00Z"), []byte("2026-10-29T12:00:01Z"), 1)
		},
		"root version": func(b []byte) []byte { return bytes.Replace(b, []byte("0.2.0-canary.1"), []byte("0.2.1-canary.1"), 1) },
		"profile": func(b []byte) []byte {
			return bytes.Replace(b, []byte("numeric-host-workloads/v1"), []byte("all-data/v1"), 1)
		},
		"schema": func(b []byte) []byte {
			return bytes.Replace(b, []byte("home-lab-connected-observation/v1"), []byte("home-lab-connected-observation/v2"), 1)
		},
		"host missing":      func(b []byte) []byte { return bytes.Replace(b, host, []byte("null"), 1) },
		"workloads missing": func(b []byte) []byte { return bytes.Replace(b, workloads, []byte("null"), 1) },
		"host string": func(b []byte) []byte {
			quoted, _ := json.Marshal(string(host))
			return bytes.Replace(b, host, quoted, 1)
		},
		"workloads string": func(b []byte) []byte {
			quoted, _ := json.Marshal(string(workloads))
			return bytes.Replace(b, workloads, quoted, 1)
		},
		"oversized":         func([]byte) []byte { return bytes.Repeat([]byte("x"), combinedprojection.MaxBytes+1) },
		"invalid UTF8":      func(b []byte) []byte { return append(b, 0xff) },
		"trailing document": func(b []byte) []byte { return append(b, []byte(`{}`)...) },
	} {
		t.Run(name, func(t *testing.T) {
			at, err := combinedprojection.CollectionTime(mutate(bytes.Clone(body)), server)
			if err != combinedprojection.ErrValidation || !at.IsZero() || strings.Contains(err.Error(), server) {
				t.Fatal("invalid envelope produced a freshness time or sensitive error")
			}
		})
	}
}
