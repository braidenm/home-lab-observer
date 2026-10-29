package collector

import (
	"context"
	"math"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/projection"
)

func TestProcessCPURangePreservesMulticoreRows(t *testing.T) {
	for _, value := range []float64{0, 100, 101, 250.5, 409600} {
		result := ProcessResult{Processes: []ProcessStat{{PID: 7, Name: "worker", State: "running", CPUPercent: value, MemoryBytes: 1}}, Discovered: 1, Scanned: 1}
		snapshot := New(nil, fakeProvider{processResult: &result}, DefaultConfig()).Collect(context.Background())
		section := snapshot.Processes
		if section.State != observation.Available || section.Quality.Errors != 0 || section.Data == nil || len(*section.Data) != 1 || (*section.Data)[0].CPUPercent != value {
			t.Fatal("a legitimate multicore process was dropped or its internal measurement changed")
		}
		// Current-v1 has an established saturated display contract; the new
		// full-range workload profile is separate, not a silent schema change.
		legacy := projection.Current(snapshot, projection.SystemInfo{}).Sections.Processes
		if len(legacy.Items) != 1 || legacy.Items[0].CPUPercent != min(value, 100) {
			t.Fatal("legacy projected process CPU range changed")
		}
	}
}

func TestProcessCPURejectsUnboundedAndNonfiniteValues(t *testing.T) {
	for _, value := range []float64{-1, 409601, math.NaN(), math.Inf(1), math.Inf(-1)} {
		result := ProcessResult{Processes: []ProcessStat{{PID: 7, Name: "worker", CPUPercent: value}}, Discovered: 1, Scanned: 1}
		section := New(nil, fakeProvider{processResult: &result}, DefaultConfig()).Collect(context.Background()).Processes
		if section.State != observation.Degraded || section.Quality.Errors != 1 || section.Data == nil || len(*section.Data) != 0 {
			t.Fatal("invalid CPU measurement became a valid process row")
		}
	}
	if percent(101) || percent(math.Inf(1)) {
		t.Fatal("process range widened aggregate host CPU")
	}
}
