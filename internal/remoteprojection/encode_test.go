package remoteprojection

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
)

func fixture() (observation.Snapshot, Identity) {
	fs := []observation.Filesystem{{ID: "/private/device", DisplayName: "private-volume", Type: "private-type", TotalBytes: 10000, UsedBytes: 5000}}
	return observation.Snapshot{
		SchemaVersion: observation.SchemaVersion, ObservedAt: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC),
		Quality:     observation.Quality{DurationMS: 125},
		CPU:         observation.Section[observation.CPU]{State: observation.Available, Data: &observation.CPU{LogicalCPUs: 8, UsagePercent: 12.5}},
		Memory:      observation.Section[observation.Memory]{State: observation.Available, Data: &observation.Memory{TotalBytes: 8192, UsedBytes: 4096, SwapTotalBytes: 1024, SwapUsedBytes: 128}},
		Uptime:      observation.Section[observation.Uptime]{State: observation.Available, Data: &observation.Uptime{Seconds: 86400}},
		Filesystems: observation.Section[[]observation.Filesystem]{State: observation.Available, Data: &fs, Quality: observation.SectionQuality{Samples: 1, Total: 1}},
	}, Identity{SourceID: "srv_0123456789abcdef0123456789abcdef", Version: "0.1.0-preview.3", OS: "linux"}
}

func TestFixtureMatchesIndependentSchemaFixture(t *testing.T) {
	raw, identity := fixture()
	got, err := Encode(raw, identity)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../schemas/remote/fixtures/numeric-host.json")
	if err != nil {
		t.Fatal(err)
	}
	var gotJSON, wantJSON any
	if json.Unmarshal(got, &gotJSON) != nil || json.Unmarshal(want, &wantJSON) != nil || !reflect.DeepEqual(gotJSON, wantJSON) {
		t.Fatal("projection differs from independently validated fixture")
	}
}

func TestExcludedLocalTextNeverCrossesBoundary(t *testing.T) {
	raw, identity := fixture()
	marker := "private-secret@example.invalid/path?credential=synthetic"
	raw.Source = observation.Source{Kind: marker, ID: marker}
	raw.CollectorVersion = marker
	raw.Capabilities = []observation.Capability{{Name: observation.CapabilityName(marker), ReasonCode: observation.ReasonCode(marker)}}
	raw.CPU.ReasonCode = observation.ReasonCode(marker)
	(*raw.Filesystems.Data)[0].ID = marker
	(*raw.Filesystems.Data)[0].DisplayName = marker
	(*raw.Filesystems.Data)[0].Type = marker
	processes := []observation.Process{{PID: 4321, Name: marker, State: marker, CPUPercent: math.NaN()}}
	raw.Processes = observation.Section[[]observation.Process]{State: observation.Available, Data: &processes}
	got, err := Encode(raw, identity)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte(marker)) || bytes.Contains(got, []byte("4321")) {
		t.Fatal("local-only metadata escaped")
	}
	before := append([]byte(nil), got...)
	(*raw.Filesystems.Data)[0].UsedBytes = 1
	if !bytes.Equal(got, before) {
		t.Fatal("encoded output aliases mutable input")
	}
}

func TestUnavailableDoesNotExposeEvenMalformedRetainedData(t *testing.T) {
	for _, state := range []observation.SupportState{observation.Degraded, observation.Unavailable, observation.Disabled, observation.PermissionDenied, observation.Unsupported, observation.Unknown, "unrecognized"} {
		for _, section := range []string{"cpu", "memory", "uptime", "filesystems"} {
			t.Run(string(state)+"/"+section, func(t *testing.T) {
				raw, identity := fixture()
				switch section {
				case "cpu":
					raw.CPU.State = state
					raw.CPU.Data.UsagePercent = math.NaN()
				case "memory":
					raw.Memory.State = state
					raw.Memory.Data.UsedBytes = math.MaxUint64
				case "uptime":
					raw.Uptime.State = state
					raw.Uptime.Data.Seconds = math.MaxUint64
				case "filesystems":
					raw.Filesystems.State = state
					(*raw.Filesystems.Data)[0].UsedBytes = math.MaxUint64
				}
				assertUnavailable(t, raw, identity)
			})
		}
	}
}

func assertUnavailable(t *testing.T, raw observation.Snapshot, identity Identity) {
	t.Helper()
	got, err := Encode(raw, identity)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Sections struct {
			Overview map[string]any `json:"overview"`
		} `json:"sections"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Sections.Overview, map[string]any{"available": false, "reason_code": "HOST_DATA_UNAVAILABLE"}) {
		t.Fatal("unavailable overview contains data")
	}
}

func TestIncompleteOverview(t *testing.T) {
	for name, mutate := range map[string]func(*observation.Snapshot){
		"cpu absent":         func(s *observation.Snapshot) { s.CPU.Data = nil },
		"memory absent":      func(s *observation.Snapshot) { s.Memory.Data = nil },
		"uptime absent":      func(s *observation.Snapshot) { s.Uptime.Data = nil },
		"filesystems absent": func(s *observation.Snapshot) { s.Filesystems.Data = nil },
		"truncated":          func(s *observation.Snapshot) { s.Filesystems.Quality.Truncated = true },
		"collection errors":  func(s *observation.Snapshot) { s.Filesystems.Quality.Errors = 1 },
		"cpu partial":        func(s *observation.Snapshot) { s.CPU.Quality.Errors = 1 },
		"cpu truncated":      func(s *observation.Snapshot) { s.CPU.Quality.Truncated = true },
		"memory partial":     func(s *observation.Snapshot) { s.Memory.Quality.Errors = 1 },
		"memory truncated":   func(s *observation.Snapshot) { s.Memory.Quality.Truncated = true },
		"uptime partial":     func(s *observation.Snapshot) { s.Uptime.Quality.Errors = 1 },
		"uptime truncated":   func(s *observation.Snapshot) { s.Uptime.Quality.Truncated = true },
	} {
		t.Run(name, func(t *testing.T) { raw, identity := fixture(); mutate(&raw); assertUnavailable(t, raw, identity) })
	}
}

func TestInvalidEligibleDataReturnsNoBytes(t *testing.T) {
	for name, mutate := range map[string]func(*observation.Snapshot){
		"cpu count low":      func(s *observation.Snapshot) { s.CPU.Data.LogicalCPUs = 0 },
		"cpu count high":     func(s *observation.Snapshot) { s.CPU.Data.LogicalCPUs = 4097 },
		"cpu nan":            func(s *observation.Snapshot) { s.CPU.Data.UsagePercent = math.NaN() },
		"cpu inf":            func(s *observation.Snapshot) { s.CPU.Data.UsagePercent = math.Inf(1) },
		"cpu low":            func(s *observation.Snapshot) { s.CPU.Data.UsagePercent = -1 },
		"cpu high":           func(s *observation.Snapshot) { s.CPU.Data.UsagePercent = 101 },
		"memory total":       func(s *observation.Snapshot) { s.Memory.Data.TotalBytes = MaxExactInteger + 1 },
		"memory used":        func(s *observation.Snapshot) { s.Memory.Data.UsedBytes = 8193 },
		"swap total":         func(s *observation.Snapshot) { s.Memory.Data.SwapTotalBytes = MaxExactInteger + 1 },
		"swap used":          func(s *observation.Snapshot) { s.Memory.Data.SwapUsedBytes = 1025 },
		"uptime":             func(s *observation.Snapshot) { s.Uptime.Data.Seconds = MaxExactInteger + 1 },
		"filesystem total":   func(s *observation.Snapshot) { (*s.Filesystems.Data)[0].TotalBytes = MaxExactInteger + 1 },
		"filesystem used":    func(s *observation.Snapshot) { (*s.Filesystems.Data)[0].UsedBytes = 10001 },
		"filesystem count":   func(s *observation.Snapshot) { s.Filesystems.Quality.Total = 2 },
		"filesystem samples": func(s *observation.Snapshot) { s.Filesystems.Quality.Samples = 0 },
		"filesystem cap":     func(s *observation.Snapshot) { fs := make([]observation.Filesystem, 17); s.Filesystems.Data = &fs },
	} {
		t.Run(name, func(t *testing.T) {
			raw, identity := fixture()
			mutate(&raw)
			got, err := Encode(raw, identity)
			if got != nil || !errors.Is(err, ErrHostData) {
				t.Fatal("invalid eligible data accepted")
			}
		})
	}
}

func TestEnvelopeAndOS(t *testing.T) {
	for _, id := range []Identity{
		{SourceID: "../source", Version: "1.0.0", OS: "linux"},
		{SourceID: strings.Repeat("a", 65), Version: "1.0.0", OS: "linux"},
		{SourceID: "srv_0123456789abcdef0123456789abcdef", Version: "1.0.0\nsecret", OS: "linux"},
		{SourceID: "srv_0123456789abcdef0123456789abcdef", Version: strings.Repeat("1", 41) + ".0.0", OS: "linux"},
		{SourceID: "srv_0123456789abcdef0123456789abcdef", Version: "1.0.0", OS: "private-os"},
		{SourceID: "agent_0123456789abcdef0123456789abcdef", Version: "1.0.0", OS: "linux"},
		{},
	} {
		raw, _ := fixture()
		got, err := Encode(raw, id)
		if got != nil || !errors.Is(err, ErrEnvelope) {
			t.Fatal("invalid identity accepted")
		}
	}
	for _, duration := range []int64{-1, 10001} {
		raw, id := fixture()
		raw.Quality.DurationMS = duration
		if got, err := Encode(raw, id); got != nil || !errors.Is(err, ErrEnvelope) {
			t.Fatal("invalid duration accepted")
		}
	}
	for _, at := range []time.Time{{}, time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(9999, 12, 31, 23, 0, 0, 0, time.FixedZone("synthetic", -3600))} {
		raw, id := fixture()
		raw.ObservedAt = at
		if got, err := Encode(raw, id); got != nil || !errors.Is(err, ErrEnvelope) {
			t.Fatal("invalid timestamp accepted")
		}
	}
	raw, id := fixture()
	raw.SchemaVersion = "unexpected"
	if got, err := Encode(raw, id); got != nil || !errors.Is(err, ErrEnvelope) {
		t.Fatal("invalid schema accepted")
	}
	for osName, want := range map[string]string{"linux": "Linux", "windows": "Windows", "darwin": "macOS"} {
		raw, id := fixture()
		id.OS = osName
		got, err := Encode(raw, id)
		if err != nil || !bytes.Contains(got, []byte(`"operating_system":"`+want+`"`)) {
			t.Fatal("OS mapping failed")
		}
	}
}

func TestExcludedFailuresDoNotSuppressHostAndEmptyFilesystemIsKnown(t *testing.T) {
	raw, id := fixture()
	raw.Quality.State = observation.Partial
	raw.Processes.State = observation.PermissionDenied
	raw.Network.State = observation.Unavailable
	empty := []observation.Filesystem{}
	raw.Filesystems.Data = &empty
	raw.Filesystems.Quality = observation.SectionQuality{}
	got, err := Encode(raw, id)
	if err != nil || !bytes.Contains(got, []byte(`"overview":{"available":true`)) || !bytes.Contains(got, []byte(`"filesystems":[]`)) {
		t.Fatal("excluded section failure or known empty inventory suppressed host data")
	}
}

func TestMaximumBoundAndDeterminism(t *testing.T) {
	raw, id := fixture()
	id.SourceID = "srv_" + strings.Repeat("a", 32)
	id.Version = "1.0.0-" + strings.Repeat("a", 34)
	raw.ObservedAt = time.Date(2026, 9, 10, 12, 0, 0, 123456789, time.FixedZone("synthetic", 3600))
	raw.Quality.DurationMS = 10000
	raw.CPU.Data = &observation.CPU{LogicalCPUs: 4096, UsagePercent: 100}
	raw.Memory.Data = &observation.Memory{TotalBytes: MaxExactInteger, UsedBytes: MaxExactInteger, SwapTotalBytes: MaxExactInteger, SwapUsedBytes: MaxExactInteger}
	raw.Uptime.Data.Seconds = MaxExactInteger
	fs := make([]observation.Filesystem, 16)
	for i := range fs {
		fs[i] = observation.Filesystem{TotalBytes: MaxExactInteger, UsedBytes: MaxExactInteger}
	}
	raw.Filesystems.Data = &fs
	raw.Filesystems.Quality = observation.SectionQuality{Samples: 16, Total: 16}
	a, err := Encode(raw, id)
	if err != nil || len(a) > MaxBytes {
		t.Fatal("maximum valid document rejected or oversized")
	}
	b, err := Encode(raw, id)
	if err != nil || !bytes.Equal(a, b) || !bytes.Contains(a, []byte("2026-09-10T11:00:00.123456789Z")) {
		t.Fatal("non-deterministic or non-UTC projection")
	}
}
