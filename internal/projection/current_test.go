package projection

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
)

func TestCurrentMatchesSchemaValidatedFixture(t *testing.T) {
	at := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	cpu := observation.CPU{LogicalCPUs: 8, UsagePercent: 12.5}
	memory := observation.Memory{TotalBytes: 17179869184, UsedBytes: 6442450944, AvailableBytes: 10737418240, UsagePercent: 37.5}
	uptime := observation.Uptime{Seconds: 86400}
	filesystems := []observation.Filesystem{{ID: "filesystem-001", DisplayName: "Filesystem 1", Type: "ext4", TotalBytes: 536870912000, UsedBytes: 214748364800, FreeBytes: 322122547200}}
	processes := []observation.Process{{PID: 101, Name: "sample-worker", State: "running", CPUPercent: 2.5, MemoryBytes: 67108864}}
	raw := observation.Snapshot{
		ObservedAt: at, Quality: observation.Quality{State: observation.Partial, DurationMS: 184},
		CPU:         observation.Section[observation.CPU]{State: observation.Available, Data: &cpu},
		Memory:      observation.Section[observation.Memory]{State: observation.Available, Data: &memory},
		Uptime:      observation.Section[observation.Uptime]{State: observation.Available, Data: &uptime},
		Filesystems: observation.Section[[]observation.Filesystem]{State: observation.Available, Quality: observation.SectionQuality{Samples: 1, Total: 2, Truncated: true}, Data: &filesystems},
		Processes:   observation.Section[[]observation.Process]{State: observation.Degraded, ReasonCode: observation.ReasonPartialCollection, Quality: observation.SectionQuality{Samples: 1, Total: 3, Truncated: true, Errors: 1}, Data: &processes},
	}
	actual, err := json.Marshal(Current(raw, SystemInfo{OS: "linux", Architecture: "amd64"}))
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile("../../schemas/v1/fixtures/valid/current-snapshot-native-preview.json")
	if err != nil {
		t.Fatal(err)
	}
	var actualValue, expectedValue any
	if err := json.Unmarshal(actual, &actualValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(expected, &expectedValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actualValue, expectedValue) {
		t.Fatalf("projection differs from schema fixture\nactual: %s\nexpected: %s", actual, expected)
	}
}

func TestUnsupportedSectionsAreEmptyAndHonest(t *testing.T) {
	snapshot := Current(observation.Snapshot{ObservedAt: time.Unix(0, 0), Quality: observation.Quality{State: observation.Failed}}, SystemInfo{})
	for name, section := range map[string]EmptySection{"services": snapshot.Sections.Services, "containers": snapshot.Sections.Containers, "logs": snapshot.Sections.Logs, "observer": snapshot.Sections.Observer} {
		if section.SupportState != "UNSUPPORTED" || section.CollectionState != "NOT_RUN" || section.Items == nil || len(section.Items) != 0 {
			t.Fatalf("%s=%+v", name, section)
		}
	}
}

func TestUnavailableListCannotProjectItems(t *testing.T) {
	items := []observation.Process{{PID: 7, Name: "must-not-appear", State: "running"}}
	raw := observation.Snapshot{ObservedAt: time.Unix(0, 0), Quality: observation.Quality{State: observation.Failed}, Processes: observation.Section[[]observation.Process]{State: observation.PermissionDenied, ReasonCode: observation.ReasonPermissionDenied, Data: &items}}
	section := Current(raw, SystemInfo{}).Sections.Processes
	if len(section.Items) != 0 || section.TotalCount != 0 || section.ObservedAt != nil {
		t.Fatalf("unsafe unsupported projection: %+v", section)
	}
}

func TestProcessCPUIsDefensivelyBounded(t *testing.T) {
	items := []observation.Process{{PID: 7, Name: "worker", State: "running", CPUPercent: 240}}
	raw := observation.Snapshot{ObservedAt: time.Unix(0, 0), Quality: observation.Quality{State: observation.Partial}, Processes: observation.Section[[]observation.Process]{State: observation.Available, Quality: observation.SectionQuality{Samples: 1, Total: 1}, Data: &items}}
	if got := Current(raw, SystemInfo{}).Sections.Processes.Items[0].CPUPercent; got != 100 {
		t.Fatalf("cpu_percent=%v", got)
	}
}
