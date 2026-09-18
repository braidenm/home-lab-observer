package remoteprojection

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/observation"
)

func TestNativeSharedReceiverFixtures(t *testing.T) {
	for _, name := range []string{"native-host-complete", "native-host-partial-cap"} {
		t.Run(name, func(t *testing.T) {
			raw, id := fixture()
			if name == "native-host-partial-cap" {
				raw.Memory.State = observation.Degraded
				raw.Memory.ReasonCode = observation.ReasonPartialCollection
				raw.Filesystems.State = observation.Degraded
				raw.Filesystems.ReasonCode = observation.ReasonPartialCollection
				raw.Filesystems.Quality.Total = 2
				raw.Filesystems.Quality.Truncated = true
			}
			got, _ := nativeFixture(t, raw, id)
			want, err := os.ReadFile("../../schemas/remote/fixtures/" + name + ".json")
			if err != nil || !bytes.Equal(got, bytes.TrimSpace(want)) {
				t.Fatal("producer differs from shared receiver fixture")
			}
		})
	}
}

func nativeFixture(t *testing.T, raw observation.Snapshot, id Identity) ([]byte, nativeDocument) {
	t.Helper()
	data, err := EncodeNative(raw, id)
	if err != nil {
		t.Fatal("native encode refused", err)
	}
	if ValidateNative(data, id.SourceID) != nil {
		t.Fatal("native roundtrip refused")
	}
	var d nativeDocument
	if json.Unmarshal(data, &d) != nil {
		t.Fatal("native fixture decode")
	}
	return data, d
}

func TestNativeIndependentQualityAndPrivacy(t *testing.T) {
	raw, id := fixture()
	marker := "private-secret@example.invalid/path?token=synthetic"
	raw.Source = observation.Source{Kind: marker, ID: marker}
	raw.CollectorVersion = marker
	raw.CPU.ReasonCode = observation.ReasonCode(marker)
	(*raw.Filesystems.Data)[0].ID = marker
	(*raw.Filesystems.Data)[0].DisplayName = marker
	raw.Filesystems.State = observation.Degraded
	raw.Filesystems.ReasonCode = observation.ReasonPartialCollection
	raw.Filesystems.Quality.Total = 900
	raw.Memory.State = observation.Degraded
	raw.Memory.ReasonCode = observation.ReasonPartialCollection
	raw.Memory.Data.SwapTotalBytes = math.MaxUint64 // Failed retained swap must not leak.
	data, d := nativeFixture(t, raw, id)
	if bytes.Contains(data, []byte(marker)) || bytes.Contains(data, []byte("900")) {
		t.Fatal("local-only data escaped")
	}
	if d.Host.CPU.Count == nil || d.Host.Uptime.Seconds == nil || d.Host.Memory.Total == nil || d.Host.Memory.SwapTotal != nil || d.Host.Filesystems.Total != nil || d.Host.Filesystems.Quality.State != observation.Degraded {
		t.Fatal("partial metric suppression or false coverage")
	}
	copyData := append([]byte{}, data...)
	raw.CPU.Data.LogicalCPUs = 1
	if !bytes.Equal(copyData, data) {
		t.Fatal("output aliases input")
	}
}

func TestNativeQualityMappings(t *testing.T) {
	for _, state := range []observation.SupportState{observation.Unavailable, observation.Disabled, observation.PermissionDenied, observation.Unsupported, observation.Unknown, "private-state"} {
		t.Run(string(state), func(t *testing.T) {
			raw, id := fixture()
			raw.CPU.State = state
			raw.CPU.ReasonCode = "private-reason"
			raw.CPU.Data.UsagePercent = math.NaN()
			raw.Memory.State = state
			raw.Memory.Data.UsedBytes = math.MaxUint64
			raw.Uptime.State = state
			raw.Uptime.Data.Seconds = math.MaxUint64
			raw.Filesystems.State = state
			(*raw.Filesystems.Data)[0].UsedBytes = math.MaxUint64
			data, d := nativeFixture(t, raw, id)
			if bytes.Contains(data, []byte("private-")) || d.Host.CPU.Count != nil || d.Host.Memory.Total != nil || d.Host.Uptime.Seconds != nil || d.Host.Filesystems.Returned != 0 {
				t.Fatal("unavailable values escaped")
			}
		})
	}
	for reason, want := range map[observation.ReasonCode]string{observation.ReasonDeadlineExceeded: "DEADLINE_EXCEEDED", observation.ReasonCollectionStopped: "COLLECTION_CANCELED", observation.ReasonNoData: "NO_DATA", observation.ReasonCollectionFailed: "COLLECTION_FAILED"} {
		raw, id := fixture()
		raw.CPU.State = observation.Unavailable
		raw.CPU.ReasonCode = reason
		_, d := nativeFixture(t, raw, id)
		if d.Host.CPU.Quality.Reason == nil || *d.Host.CPU.Quality.Reason != want {
			t.Fatal("reason mapping")
		}
	}
}

func TestNativeFilesystemCoverage(t *testing.T) {
	for _, tc := range []struct {
		name          string
		state         observation.SupportState
		total, errors int
		truncated     bool
		wantState     observation.SupportState
		wantTotal     bool
	}{
		{"complete", observation.Available, 1, 0, false, observation.Available, true},
		{"unverified", observation.Degraded, 1, 0, false, observation.Degraded, false},
		{"row-failure", observation.Degraded, 2, 1, false, observation.Degraded, false},
		{"configured-cap-one", observation.Degraded, 2, 0, true, observation.Degraded, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, id := fixture()
			raw.Filesystems.State = tc.state
			raw.Filesystems.Quality.Total = tc.total
			raw.Filesystems.Quality.Errors = tc.errors
			raw.Filesystems.Quality.Truncated = tc.truncated
			_, d := nativeFixture(t, raw, id)
			f := d.Host.Filesystems
			if f.Quality.State != tc.wantState || (f.Total != nil) != tc.wantTotal || f.Truncated != tc.truncated {
				t.Fatal("coverage overstated or cap lost")
			}
		})
	}
	raw, id := fixture()
	empty := []observation.Filesystem{}
	raw.Filesystems.Data = &empty
	raw.Filesystems.State = observation.Degraded
	raw.Filesystems.Quality = observation.SectionQuality{}
	_, d := nativeFixture(t, raw, id)
	if d.Host.Filesystems.Quality.State != observation.Unavailable {
		t.Fatal("empty degraded data presented")
	}
}

func TestNativeEligibleInvalidNumbersRefuse(t *testing.T) {
	for name, mutate := range map[string]func(*observation.Snapshot){
		"cpu-zero":   func(s *observation.Snapshot) { s.CPU.Data.LogicalCPUs = 0 },
		"cpu-max":    func(s *observation.Snapshot) { s.CPU.Data.LogicalCPUs = 4097 },
		"nan":        func(s *observation.Snapshot) { s.CPU.Data.UsagePercent = math.NaN() },
		"inf":        func(s *observation.Snapshot) { s.CPU.Data.UsagePercent = math.Inf(1) },
		"percent":    func(s *observation.Snapshot) { s.CPU.Data.UsagePercent = 101 },
		"ram":        func(s *observation.Snapshot) { s.Memory.Data.TotalBytes = MaxExactInteger + 1 },
		"used":       func(s *observation.Snapshot) { s.Memory.Data.UsedBytes = s.Memory.Data.TotalBytes + 1 },
		"swap":       func(s *observation.Snapshot) { s.Memory.Data.SwapUsedBytes = s.Memory.Data.SwapTotalBytes + 1 },
		"uptime":     func(s *observation.Snapshot) { s.Uptime.Data.Seconds = MaxExactInteger + 1 },
		"filesystem": func(s *observation.Snapshot) { (*s.Filesystems.Data)[0].UsedBytes = MaxExactInteger + 1 },
		"over-cap": func(s *observation.Snapshot) {
			f := make([]observation.Filesystem, 17)
			s.Filesystems.Data = &f
			s.Filesystems.Quality.Samples = 17
			s.Filesystems.Quality.Total = 17
		},
		"false-truncation": func(s *observation.Snapshot) { s.Filesystems.Quality.Truncated = true },
		"sample-mismatch":  func(s *observation.Snapshot) { s.Filesystems.Quality.Samples = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			raw, id := fixture()
			mutate(&raw)
			if b, e := EncodeNative(raw, id); e != ErrHostData || b != nil {
				t.Fatal("invalid eligible data accepted")
			}
		})
	}
}

func TestNativeCanonicalValidation(t *testing.T) {
	raw, id := fixture()
	data, _ := nativeFixture(t, raw, id)
	for _, bad := range [][]byte{nil, bytes.Repeat([]byte("x"), MaxBytes+1), append(append([]byte{}, data...), '\n'), bytes.Replace(data, []byte(`{"schema_version":`), []byte(`{"extra":0,"schema_version":`), 1), bytes.Replace(data, []byte(`{"schema_version":`), []byte(`{"schema_version":"wrong","schema_version":`), 1), bytes.Replace(data, []byte(`"duration_ms":125`), []byte(`"duration_ms":null`), 1)} {
		if ValidateNative(bad, id.SourceID) != ErrValidation {
			t.Fatal("noncanonical document accepted")
		}
	}
	if ValidateNative(data, "srv_"+strings.Repeat("f", 32)) != ErrValidation {
		t.Fatal("foreign server accepted")
	}
	for name, mutate := range map[string]func(*nativeDocument){
		"schema":         func(d *nativeDocument) { d.Schema = "other" },
		"missing-reason": func(d *nativeDocument) { d.Host.CPU.Quality.State = observation.Unavailable },
		"cpu-degraded":   func(d *nativeDocument) { d.Host.CPU.Quality = nativeState(observation.Degraded, "") },
		"half-memory":    func(d *nativeDocument) { d.Host.Memory.Used = nil },
		"degraded-swap":  func(d *nativeDocument) { d.Host.Memory.Quality = nativeState(observation.Degraded, "") },
		"fs-null":        func(d *nativeDocument) { d.Host.Filesystems.Items = nil },
		"alias":          func(d *nativeDocument) { d.Host.Filesystems.Items[0].Alias = "private" },
		"coverage":       func(d *nativeDocument) { d.Host.Filesystems.Total = nil },
		"degraded-total": func(d *nativeDocument) { d.Host.Filesystems.Quality = nativeState(observation.Degraded, "") },
		"time":           func(d *nativeDocument) { d.At = "0001-01-01T00:00:00Z" },
	} {
		t.Run(name, func(t *testing.T) {
			var d nativeDocument
			_ = json.Unmarshal(data, &d)
			mutate(&d)
			b, _ := json.Marshal(d)
			if ValidateNative(b, id.SourceID) != ErrValidation {
				t.Fatal("invalid wire combination accepted")
			}
		})
	}
}

func TestNativeZeroAndOperatingSystems(t *testing.T) {
	for _, os := range []string{"linux", "windows", "darwin"} {
		raw, id := fixture()
		id.OS = os
		raw.CPU.Data.UsagePercent = 0
		raw.Memory.Data = &observation.Memory{}
		raw.Uptime.Data.Seconds = 0
		_, d := nativeFixture(t, raw, id)
		if d.Host.Memory.Total == nil || *d.Host.Memory.Total != 0 || d.Host.Uptime.Seconds == nil || *d.Host.Uptime.Seconds != 0 {
			t.Fatal("zero confused with missing")
		}
	}
}
