package workloadprojection

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/containerobs"
	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
)

func inputs() (observation.Snapshot, containerobs.Inventory, Identity) {
	at := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	ps := []observation.Process{{PID: 42, Name: "sample-worker", State: "sleep", CPUPercent: 0, MemoryBytes: 2048, UptimeSeconds: 12}}
	raw := observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: at,
		Processes: observation.Section[[]observation.Process]{State: observation.Available,
			Quality: observation.SectionQuality{Samples: 1, Total: 1}, Data: &ps}}
	memory, reason := uint64(4096), "METRIC_COUNTER_INVALID"
	docker := containerobs.Inventory{SchemaVersion: containerobs.SchemaVersion, ObservedAt: &at,
		SupportState: containerobs.SupportSupported, CollectionState: containerobs.CollectionPartial,
		Freshness: containerobs.FreshnessCurrent, TotalCount: 2, ReturnedCount: 2,
		Items: []containerobs.Container{
			{IDAlias: "ctr_0000000000000001", Name: "sample-service", Image: "example.invalid/sample:1", State: "running", MemoryBytes: &memory, MetricsState: containerobs.MetricsPartial, ReasonCode: &reason},
			{IDAlias: "ctr_0000000000000002", Name: "sample-stopped", Image: "example.invalid/sample:1", State: "exited", MetricsState: containerobs.MetricsNotRunning},
		}}
	return raw, docker, Identity{SourceID: "srv_0123456789abcdef0123456789abcdef", Version: "0.2.0-canary.1"}
}

func encoded(t *testing.T, consent Consent) ([]byte, document) {
	t.Helper()
	raw, docker, id := inputs()
	return encodeInput(t, raw, docker, id, consent)
}

func encodeInput(t *testing.T, raw observation.Snapshot, docker containerobs.Inventory, id Identity, consent Consent) ([]byte, document) {
	t.Helper()
	b, err := Encode(raw, docker, id, consent)
	if err != nil || Validate(b, id.SourceID) != nil {
		t.Fatal("workload projection or roundtrip refused", err)
	}
	var d document
	if json.Unmarshal(b, &d) != nil {
		t.Fatal("workload document decode")
	}
	return b, d
}

func TestSharedFixturesAndNoUploadAdmission(t *testing.T) {
	for name, consent := range map[string]Consent{"workloads-disabled": {}, "workloads-partial": {Processes: true, Containers: true}} {
		t.Run(name, func(t *testing.T) {
			b, _ := encoded(t, consent)
			want, err := os.ReadFile("../../schemas/remote/fixtures/" + name + ".json")
			if err != nil || !bytes.Equal(b, bytes.TrimSpace(want)) {
				t.Fatal("producer differs from shared fixture")
			}
			if remoteprojection.ValidateUpload(b, "srv_0123456789abcdef0123456789abcdef") == nil {
				t.Fatal("unused workload profile acquired upload admission")
			}
		})
	}
}

func TestConsentSuppressesRetainedInvalidDataIndependently(t *testing.T) {
	raw, docker, id := inputs()
	(*raw.Processes.Data)[0].Name = "secret@example.invalid"
	(*raw.Processes.Data)[0].CPUPercent = math.NaN()
	docker.Items[0].IDAlias = "private-local-id"
	for _, consent := range []Consent{{}, {Processes: false, Containers: false}} {
		b, d := encodeInput(t, raw, docker, id, consent)
		if bytes.Contains(b, []byte("secret")) || d.Processes.ObservedAt != nil || d.Containers.Total != nil || len(d.Processes.Items)+len(d.Containers.Items) != 0 {
			t.Fatal("disabled data escaped")
		}
	}
	raw, docker, id = inputs()
	_, d := encodeInput(t, raw, docker, id, Consent{Processes: true})
	if len(d.Processes.Items) != 1 || d.Containers.Quality.State != observation.Disabled {
		t.Fatal("process opt-in widened container consent")
	}
	_, d = encodeInput(t, raw, docker, id, Consent{Containers: true})
	if len(d.Containers.Items) != 2 || d.Processes.Quality.State != observation.Disabled {
		t.Fatal("container opt-in widened process consent")
	}
}

func TestPrivacyAndDetachedOutput(t *testing.T) {
	raw, docker, id := inputs()
	raw.Source.ID, raw.CollectorVersion = "local-only-source", "local-only-version"
	raw.Processes.ReasonCode = "private-failure"
	(*raw.Processes.Data)[0].Name = "/private/fixture/sample-worker"
	(*raw.Processes.Data)[0].State = "private-state"
	(*raw.Processes.Data)[0].UptimeSeconds = 0
	docker.Items[0].Name = "https://sample:fixture@example.invalid/private"
	docker.Items[0].Image = "fixture@example.invalid"
	docker.Items[0].ReasonCode = p("private-failure")
	b, d := encodeInput(t, raw, docker, id, Consent{Processes: true, Containers: true})
	for _, marker := range []string{"local-only", "private", "fixture@example.invalid", "https://", "sample:fixture"} {
		if bytes.Contains(b, []byte(marker)) {
			t.Fatal("private retained field escaped")
		}
	}
	if d.Processes.Items[0].Name != "sample-worker" || d.Processes.Items[0].State != "unknown" || d.Processes.Items[0].Elapsed != nil || d.Containers.Items[0].CPU != nil {
		t.Fatal("normalization or nullable metric semantics")
	}
	saved := bytes.Clone(b)
	docker.Items[0].Name = "changed"
	*docker.Items[0].MemoryBytes = 99
	if !bytes.Equal(saved, b) {
		t.Fatal("output aliases caller data")
	}
}

func TestQualityAndHealthyEmpty(t *testing.T) {
	for _, state := range []observation.SupportState{observation.Disabled, observation.Unavailable, observation.PermissionDenied, observation.Unsupported, observation.Unknown, "untrusted-state"} {
		t.Run(string(state), func(t *testing.T) {
			raw, docker, id := inputs()
			raw.Processes.State, raw.Processes.ReasonCode = state, "private-reason"
			_, d := encodeInput(t, raw, docker, id, Consent{Processes: true})
			if len(d.Processes.Items) != 0 || d.Processes.Total != nil || d.Processes.Truncated {
				t.Fatal("unavailable inventory became observed rows")
			}
		})
	}
	raw, docker, id := inputs()
	empty := []observation.Process{}
	raw.Processes.Data, raw.Processes.Quality = &empty, observation.SectionQuality{}
	docker.Items, docker.TotalCount, docker.ReturnedCount, docker.CollectionState = []containerobs.Container{}, 0, 0, containerobs.CollectionOK
	_, d := encodeInput(t, raw, docker, id, Consent{Processes: true, Containers: true})
	if d.Processes.Total == nil || *d.Processes.Total != 0 || d.Containers.Total == nil || *d.Containers.Total != 0 || d.Processes.Quality.State != observation.Available {
		t.Fatal("healthy empty is not a measured zero")
	}
	raw, docker, id = inputs()
	raw.Processes.State, raw.Processes.Quality.Errors = observation.Degraded, 1
	_, d = encodeInput(t, raw, docker, id, Consent{Processes: true, Containers: true})
	if d.Processes.Total != nil || d.Containers.Total == nil || d.Containers.Items[1].CPU != nil || d.Containers.Items[1].Memory != nil {
		t.Fatal("coverage or stopped metrics fabricated")
	}
}

func TestInvalidEligibleDataRejectsWithoutValues(t *testing.T) {
	for name, mutate := range map[string]func(*observation.Snapshot, *containerobs.Inventory, *Identity){
		"source":  func(_ *observation.Snapshot, _ *containerobs.Inventory, id *Identity) { id.SourceID = "private-source" },
		"version": func(_ *observation.Snapshot, _ *containerobs.Inventory, id *Identity) { id.Version = "private-version" },
		"time":    func(r *observation.Snapshot, _ *containerobs.Inventory, _ *Identity) { r.ObservedAt = time.Time{} },
		"PID":     func(r *observation.Snapshot, _ *containerobs.Inventory, _ *Identity) { (*r.Processes.Data)[0].PID = 0 },
		"duplicate PID": func(r *observation.Snapshot, _ *containerobs.Inventory, _ *Identity) {
			*r.Processes.Data = append(*r.Processes.Data, (*r.Processes.Data)[0])
			r.Processes.Quality.Samples = 2
			r.Processes.Quality.Total = 2
		},
		"CPU NaN": func(r *observation.Snapshot, _ *containerobs.Inventory, _ *Identity) {
			(*r.Processes.Data)[0].CPUPercent = math.NaN()
		},
		"CPU infinity": func(_ *observation.Snapshot, d *containerobs.Inventory, _ *Identity) {
			d.Items[0].CPUPercent = p(math.Inf(1))
		},
		"memory": func(_ *observation.Snapshot, d *containerobs.Inventory, _ *Identity) {
			d.Items[0].MemoryBytes = p(uint64(1 << 53))
		},
		"count":           func(r *observation.Snapshot, _ *containerobs.Inventory, _ *Identity) { r.Processes.Quality.Samples = 3 },
		"container count": func(_ *observation.Snapshot, d *containerobs.Inventory, _ *Identity) { d.ReturnedCount = 0 },
		"alias": func(_ *observation.Snapshot, d *containerobs.Inventory, _ *Identity) {
			d.Items[0].IDAlias = "private-id"
		},
		"duplicate alias": func(_ *observation.Snapshot, d *containerobs.Inventory, _ *Identity) {
			d.Items[1].IDAlias = d.Items[0].IDAlias
		},
		"invalid UTF8": func(r *observation.Snapshot, _ *containerobs.Inventory, _ *Identity) {
			(*r.Processes.Data)[0].Name = string([]byte{0xff})
		},
		"oversized text": func(_ *observation.Snapshot, d *containerobs.Inventory, _ *Identity) {
			d.Items[0].Name = strings.Repeat("x", 1025)
		},
		"forged available": func(_ *observation.Snapshot, d *containerobs.Inventory, _ *Identity) {
			d.Items[0].MetricsState = containerobs.MetricsAvailable
		},
		"stopped live metrics": func(_ *observation.Snapshot, d *containerobs.Inventory, _ *Identity) {
			d.Items[1].MemoryBytes = p(uint64(0))
		},
	} {
		t.Run(name, func(t *testing.T) {
			raw, docker, id := inputs()
			mutate(&raw, &docker, &id)
			b, err := Encode(raw, docker, id, Consent{Processes: true, Containers: true})
			if err == nil || b != nil || (err != ErrEnvelope && err != ErrData) {
				t.Fatal("invalid eligible data did not fail closed")
			}
		})
	}
}

func TestRowAndByteBudgetsAreDeterministic(t *testing.T) {
	raw, docker, id := inputs()
	ps := make([]observation.Process, 201)
	for i := range ps {
		ps[i] = observation.Process{PID: int32(i + 1), Name: "sample", State: "running", CPUPercent: 0, MemoryBytes: uint64(i)}
	}
	raw.Processes.Data, raw.Processes.Quality = &ps, observation.SectionQuality{Samples: len(ps), Total: len(ps)}
	docker.Items = make([]containerobs.Container, 501)
	for i := range docker.Items {
		docker.Items[i] = containerobs.Container{IDAlias: fmt.Sprintf("ctr_%016x", i), Name: "sample", Image: "example.invalid/sample", State: "exited", MetricsState: containerobs.MetricsNotRunning}
	}
	docker.ReturnedCount, docker.TotalCount, docker.CollectionState = 501, 501, containerobs.CollectionOK
	b, d := encodeInput(t, raw, docker, id, Consent{Processes: true, Containers: true})
	if len(d.Processes.Items) != MaxProcesses || len(d.Containers.Items) != MaxContainers || !d.Processes.Truncated || !d.Containers.Truncated || *d.Processes.Total != 201 {
		t.Fatal("row budget or known total lost")
	}
	if d.Processes.Items[0].PID != 201 {
		t.Fatal("process ranking lost")
	}
	for i := range docker.Items {
		docker.Items[i].Name, docker.Items[i].Image = strings.Repeat("界", 128), strings.Repeat("&", 240)
	}
	b, d = encodeInput(t, raw, docker, id, Consent{Processes: true, Containers: true})
	section, _ := json.Marshal(d.Containers)
	if len(b) > MaxBytes || len(section) > MaxSectionBytes || !d.Containers.Truncated || len(d.Containers.Items) >= MaxContainers || *d.Containers.Total != 501 {
		t.Fatal("encoded byte budget or truncation metadata")
	}
	b2, _ := encodeInput(t, raw, docker, id, Consent{Processes: true, Containers: true})
	if !bytes.Equal(b, b2) {
		t.Fatal("projection is nondeterministic")
	}
}

func TestProcessCPUHasExplicitLifetimeSingleCoreBasis(t *testing.T) {
	for _, value := range []float64{101, 409600} {
		raw, docker, id := inputs()
		(*raw.Processes.Data)[0].CPUPercent = value
		_, d := encodeInput(t, raw, docker, id, Consent{Processes: true})
		if *d.Processes.Items[0].CPU != value || d.Processes.CPUBasis != "LIFETIME_SINGLE_CORE" || d.Containers.CPUBasis != "ENGINE_HOST_CAPACITY" {
			t.Fatal("CPU basis or multi-core value misrepresented")
		}
	}
	raw, docker, id := inputs()
	(*raw.Processes.Data)[0].CPUPercent = 409601
	if _, err := Encode(raw, docker, id, Consent{Processes: true}); err != ErrData {
		t.Fatal("unbounded process CPU accepted")
	}
}

func TestValidatorRejectsNoncanonicalUnknownAndCrossSource(t *testing.T) {
	b, _ := encoded(t, Consent{Processes: true, Containers: true})
	for _, mutated := range [][]byte{
		append(bytes.Clone(b), '\n'),
		bytes.Replace(b, []byte(`"schema_version":`), []byte(`"unexpected":true,"schema_version":`), 1),
		bytes.Replace(b, []byte(`"schema_version":`), []byte(`"schema_version":"duplicate","schema_version":`), 1),
		bytes.Replace(b, []byte(`"cpu_percent":null`), []byte(`"cpu_percent":0`), 1),
		bytes.Replace(b, []byte(`"processes_enabled":true`), []byte(`"processes_enabled":false`), 1),
		bytes.Repeat([]byte(" "), MaxBytes+1),
	} {
		if Validate(mutated, "srv_0123456789abcdef0123456789abcdef") == nil {
			t.Fatal("validator accepted invalid producer bytes")
		}
	}
	if Validate(b, "srv_ffffffffffffffffffffffffffffffff") == nil {
		t.Fatal("cross-source projection accepted")
	}
}

func TestDisplayRedactionAndUTF8Bounds(t *testing.T) {
	for _, value := range []string{
		"sample@example.invalid", "password=fixture", "Bearer fixture", "https://sample:fixture@example.invalid",
		"gh" + "p_" + strings.Repeat("a", 24), "github" + "_pat_" + strings.Repeat("a", 24),
		"AK" + "IA" + strings.Repeat("A", 16), "10." + "1.2.3", "[fd00::1234]",
	} {
		got, err := display(value, 128, false)
		if err != nil || got != "[REDACTED]" {
			t.Fatal("common sensitive metadata not redacted")
		}
	}
	got, err := display(strings.Repeat("界", 128), 128, false)
	if err != nil || len(got) != 126 || !textValid(got, 128) {
		t.Fatal("UTF-8 byte boundary broken")
	}
	got, err = display(" C:\\private\\sample\x00-worker ", 128, true)
	if err != nil || got != "sample -worker" {
		t.Fatal("process basename/control normalization")
	}
	for _, value := range []string{"sample\u202eservice", "sample\u200bservice", "sample\u2066service\u2069"} {
		got, err := display(value, 128, false)
		if err != nil || got != "sampleservice" {
			t.Fatal("invisible Unicode formatting survived normalization")
		}
		if textValid(value, 128) {
			t.Fatal("invisible Unicode formatting accepted by validator")
		}
	}
	got, err = display("pass\u200bword=fixture", 128, false)
	if err != nil || got != "[REDACTED]" {
		t.Fatal("Unicode formatting bypassed secret filtering")
	}
}

func TestKnownConnectorSecretsAndJWTNeverEscape(t *testing.T) {
	for i, value := range []string{
		"hl" + "e_" + strings.Repeat("x", 43),
		"hl" + "c_" + strings.Repeat("_", 42) + "-",
		"eyJhbGciOiJub25lIn0" + "." + "eyJzdWIiOiJzeW50aGV0aWMifQ" + "." + "synthetic-signature",
	} {
		t.Run(fmt.Sprintf("encode-%d", i), func(t *testing.T) {
			raw, docker, id := inputs()
			(*raw.Processes.Data)[0].Name = value
			docker.Items[0].Name, docker.Items[0].Image = value, value
			b, d := encodeInput(t, raw, docker, id, Consent{Processes: true, Containers: true})
			if bytes.Contains(b, []byte(value)) || d.Processes.Items[0].Name != "[REDACTED]" || d.Containers.Items[0].Name != "[REDACTED]" || d.Containers.Items[0].Image != "[REDACTED]" {
				t.Fatal("known credential-shaped metadata escaped encoder")
			}
		})
		for j, mutate := range []func(*document){
			func(d *document) { d.Processes.Items[0].Name = value },
			func(d *document) { d.Containers.Items[0].Name = value },
			func(d *document) { d.Containers.Items[0].Image = value },
		} {
			t.Run(fmt.Sprintf("validate-%d-%d", i, j), func(t *testing.T) {
				_, clean := encoded(t, Consent{Processes: true, Containers: true})
				mutate(&clean)
				unsafe, _ := json.Marshal(clean)
				if Validate(unsafe, clean.Source) == nil {
					t.Fatal("known credential-shaped metadata accepted by validator")
				}
			})
		}
	}
}

func TestDockerSupportAndMetricReasonsStayClosed(t *testing.T) {
	for input, want := range map[string]observation.SupportState{
		containerobs.SupportDisabled: observation.Disabled, containerobs.SupportPermissionDenied: observation.PermissionDenied,
		containerobs.SupportUnsupported: observation.Unsupported, containerobs.SupportUnavailable: observation.Unavailable,
		"private-status": observation.Unknown,
	} {
		raw, docker, id := inputs()
		docker.SupportState, docker.ReasonCode = input, p("private-error")
		b, d := encodeInput(t, raw, docker, id, Consent{Containers: true})
		if bytes.Contains(b, []byte("private")) || d.Containers.Quality.State != want || len(d.Containers.Items) != 0 || d.Containers.Total != nil {
			t.Fatal("Docker failure shape leaked or fabricated rows")
		}
	}
	for input, want := range map[string]string{
		"PERMISSION_DENIED": "PERMISSION_DENIED", "ENGINE_OS_UNSUPPORTED": "UNSUPPORTED",
		"DEADLINE_EXCEEDED": "DEADLINE_EXCEEDED", "COLLECTION_CANCELED": "COLLECTION_CANCELED",
		"METRIC_COUNTER_INVALID": "INVALID_COUNTER", "private-error": "METRIC_UNAVAILABLE",
	} {
		raw, docker, id := inputs()
		docker.Items[0].MemoryBytes, docker.Items[0].MetricsState, docker.Items[0].ReasonCode = nil, containerobs.MetricsUnavailable, &input
		_, d := encodeInput(t, raw, docker, id, Consent{Containers: true})
		if *d.Containers.Items[0].Reason != want {
			t.Fatal("metric reason not mapped")
		}
	}
}

func TestCanonicalSemanticMutationsReject(t *testing.T) {
	for name, mutate := range map[string]func(*document){
		"count mismatch":            func(d *document) { d.Processes.Returned++ },
		"total below rows":          func(d *document) { d.Containers.Total = p(uint64(0)) },
		"false complete stats":      func(d *document) { d.Containers.Quality = q(observation.Available, "") },
		"wrong process CPU basis":   func(d *document) { d.Processes.CPUBasis = "ENGINE_HOST_CAPACITY" },
		"wrong container CPU basis": func(d *document) { d.Containers.CPUBasis = "LIFETIME_SINGLE_CORE" },
		"process path":              func(d *document) { d.Processes.Items[0].Name = "/private/sample" },
		"duplicate alias":           func(d *document) { d.Containers.Items[1].Alias = d.Containers.Items[0].Alias },
		"unsorted alias": func(d *document) {
			d.Containers.Items[0], d.Containers.Items[1] = d.Containers.Items[1], d.Containers.Items[0]
		},
		"private metric reason":       func(d *document) { d.Containers.Items[0].Reason = p("private-error") },
		"process bidi formatting":     func(d *document) { d.Processes.Items[0].Name = "sample\u202eservice" },
		"container zero width":        func(d *document) { d.Containers.Items[0].Name = "sample\u200bservice" },
		"image directional isolation": func(d *document) { d.Containers.Items[0].Image = "example.invalid/\u2066sample\u2069" },
		"disabled observation timestamp": func(d *document) {
			d.Policy.Processes = false
			d.Processes = empty[processRow](observation.Disabled, "")
			d.Processes.ObservedAt = &d.At
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, d := encoded(t, Consent{Processes: true, Containers: true})
			mutate(&d)
			b, _ := json.Marshal(d)
			if Validate(b, d.Source) == nil {
				t.Fatal("invalid canonical semantics accepted")
			}
		})
	}
}

func TestInputCapsAndDiscardedRowsAreValidated(t *testing.T) {
	raw, docker, id := inputs()
	ps := make([]observation.Process, maxInputRows+1)
	raw.Processes.Data, raw.Processes.Quality = &ps, observation.SectionQuality{Samples: len(ps), Total: len(ps)}
	if _, err := Encode(raw, docker, id, Consent{Processes: true}); err != ErrData {
		t.Fatal("unbounded process input accepted")
	}
	raw, docker, id = inputs()
	docker.Items = make([]containerobs.Container, maxInputRows+1)
	docker.ReturnedCount, docker.TotalCount = len(docker.Items), len(docker.Items)
	if _, err := Encode(raw, docker, id, Consent{Containers: true}); err != ErrData {
		t.Fatal("unbounded Docker input accepted")
	}
	raw, docker, id = inputs()
	ps = make([]observation.Process, MaxProcesses+1)
	for i := range ps {
		ps[i] = observation.Process{PID: int32(i + 1), Name: "sample", State: "running"}
	}
	ps[MaxProcesses].PID = ps[0].PID
	raw.Processes.Data, raw.Processes.Quality = &ps, observation.SectionQuality{Samples: len(ps), Total: len(ps)}
	if _, err := Encode(raw, docker, id, Consent{Processes: true}); err != ErrData {
		t.Fatal("duplicate hidden past output cap accepted")
	}
}
