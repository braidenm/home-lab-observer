package projection

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
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

func TestOverviewDoesNotExposeDeniedData(t *testing.T) {
	raw := observation.Snapshot{
		CPU:    observation.Section[observation.CPU]{State: observation.Available, Data: &observation.CPU{LogicalCPUs: 4}},
		Memory: observation.Section[observation.Memory]{State: observation.PermissionDenied, Data: &observation.Memory{TotalBytes: 100}},
		Uptime: observation.Section[observation.Uptime]{State: observation.Available, Data: &observation.Uptime{}},
	}
	result := Current(raw, SystemInfo{})
	if result.Sections.Overview.Data != nil || result.Sections.Overview.SupportState != "PERMISSION_DENIED" {
		t.Fatalf("denied data exposed: %+v", result.Sections.Overview)
	}
}

func TestRecomputeSelectedCollectionState(t *testing.T) {
	current := CurrentSnapshot{CollectionState: "PARTIAL"}
	current.Sections.Overview.SectionStatus = SectionStatus{SupportState: "SUPPORTED", CollectionState: "OK"}
	current.Sections.Processes.SectionStatus = SectionStatus{SupportState: "DISABLED", CollectionState: "NOT_RUN"}
	current.Sections.Filesystems.SectionStatus = SectionStatus{SupportState: "DISABLED", CollectionState: "NOT_RUN"}
	current.Sections.Services.SectionStatus = SectionStatus{SupportState: "UNSUPPORTED", CollectionState: "NOT_RUN"}
	current.Sections.Containers.SectionStatus = current.Sections.Services.SectionStatus
	current.Sections.Logs.SectionStatus = current.Sections.Services.SectionStatus
	current.Sections.Observer.SectionStatus = current.Sections.Filesystems.SectionStatus
	if got := RecomputeCollectionState(current).CollectionState; got != "OK" {
		t.Fatalf("selected healthy state=%s", got)
	}
}

func TestUnsupportedSectionsAreEmptyAndHonest(t *testing.T) {
	snapshot := Current(observation.Snapshot{ObservedAt: time.Unix(0, 0), Quality: observation.Quality{State: observation.Failed}}, SystemInfo{})
	for name, section := range map[string]EmptySection{"services": snapshot.Sections.Services, "containers": snapshot.Sections.Containers} {
		if section.SupportState != "UNSUPPORTED" || section.CollectionState != "NOT_RUN" || section.Items == nil || len(section.Items) != 0 {
			t.Fatalf("%s=%+v", name, section)
		}
	}
	logs := snapshot.Sections.Logs
	if logs.SupportState != "UNSUPPORTED" || logs.CollectionState != "NOT_RUN" || logs.Items == nil || len(logs.Items) != 0 {
		t.Fatalf("logs=%+v", logs)
	}
	observer := snapshot.Sections.Observer
	if observer.SupportState != "UNSUPPORTED" || observer.CollectionState != "NOT_RUN" || observer.Items == nil || len(observer.Items) != 0 {
		t.Fatalf("observer=%+v", observer)
	}
}

func TestWithLogsProjectsBoundedMetadataAndOmittedBodiesWithoutMutatingSource(t *testing.T) {
	at := time.Date(2026, 9, 9, 19, 0, 0, 0, time.UTC)
	observed, attempted := at, at
	snapshot := logobs.Snapshot{
		Status:     logobs.Status{SupportState: logobs.SupportSupported, CollectionState: logobs.CollectionOK, Freshness: logobs.FreshnessCurrent, ObservedAt: &observed, AttemptedAt: &attempted},
		TotalCount: 2,
		Events: []logobs.Event{
			{ObservedAt: at, Source: logobs.SourceSystem, Severity: logobs.SeverityError, EventCode: "SYSTEMD_PRIORITY_3"},
			{ObservedAt: at.Add(-time.Second), Source: logobs.SourceApplication, Severity: logobs.SeverityInfo, EventCode: "WIN_100"},
		},
	}
	before := snapshot.Clone()
	current := WithLogs(CurrentSnapshot{}, snapshot, 1)
	section := current.Sections.Logs
	if section.TotalCount != 2 || section.ReturnedCount != 1 || !section.Truncated || len(section.Items) != 1 {
		t.Fatalf("bounds=%+v", section.ListStatus)
	}
	item := section.Items[0]
	if item.Metadata.Source != "system" || item.Metadata.Severity != "ERROR" || item.Metadata.EventCode != "SYSTEMD_PRIORITY_3" || item.Body.State != "OMITTED" {
		t.Fatalf("item=%+v", item)
	}
	if !reflect.DeepEqual(snapshot, before) {
		t.Fatal("projection mutated source snapshot")
	}
}

func TestWithLogsRetainsStaleRingWithTruthfulLatestStatus(t *testing.T) {
	attempted := time.Date(2026, 9, 9, 19, 0, 0, 0, time.UTC)
	eventAt := attempted.Add(-time.Minute)
	reason := logobs.ReasonPermissionDenied
	snapshot := logobs.Snapshot{
		Status:     logobs.Status{SupportState: logobs.SupportPermissionDenied, CollectionState: logobs.CollectionNotRun, Freshness: logobs.FreshnessUnknown, AttemptedAt: &attempted, ReasonCode: &reason},
		TotalCount: 1,
		Events:     []logobs.Event{{ObservedAt: eventAt, Source: logobs.SourceSystem, Severity: logobs.SeverityWarn, EventCode: "WIN_5"}},
	}
	section := WithLogs(CurrentSnapshot{}, snapshot, 1).Sections.Logs
	if section.SupportState != "PERMISSION_DENIED" || section.CollectionState != "NOT_RUN" || section.Freshness != "STALE" || section.ObservedAt == nil || !section.ObservedAt.Equal(eventAt) || section.ReasonCode == nil || *section.ReasonCode != "PERMISSION_DENIED" || len(section.Items) != 1 {
		t.Fatalf("retained section=%+v", section)
	}
}

func TestWithLogsRejectsInvalidSnapshotAndLimit(t *testing.T) {
	for _, snapshot := range []logobs.Snapshot{
		{TotalCount: 1, Events: []logobs.Event{}},
		{},
	} {
		limit := 1
		if snapshot.TotalCount == 0 {
			limit = MaxProjectedLogs + 1
		}
		section := WithLogs(CurrentSnapshot{}, snapshot, limit).Sections.Logs
		if section.SupportState != "UNAVAILABLE" || section.CollectionState != "NOT_RUN" || section.Freshness != "UNKNOWN" || section.ObservedAt != nil || section.ReasonCode == nil || *section.ReasonCode != "INVALID_RESPONSE" || len(section.Items) != 0 {
			t.Fatalf("invalid section=%+v", section)
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

func TestProjectionAppliesContractCapsAndPreservesTotals(t *testing.T) {
	filesystems := make([]observation.Filesystem, MaxProjectedFilesystems+1)
	for index := range filesystems {
		filesystems[index] = observation.Filesystem{ID: "filesystem-001", Type: "ext4"}
	}
	processes := make([]observation.Process, MaxProjectedProcesses+1)
	for index := range processes {
		processes[index] = observation.Process{PID: int32(index + 1), Name: "worker", State: "running"}
	}
	processes[0].Name = `/private/users/example/` + strings.Repeat("x", 200)
	raw := observation.Snapshot{
		ObservedAt:  time.Unix(0, 0),
		Filesystems: observation.Section[[]observation.Filesystem]{State: observation.Available, Quality: observation.SectionQuality{Total: 40}, Data: &filesystems},
		Processes:   observation.Section[[]observation.Process]{State: observation.Available, Quality: observation.SectionQuality{Total: 500}, Data: &processes},
	}
	current := Current(raw, SystemInfo{})
	if got := current.Sections.Filesystems; len(got.Items) != 16 || got.TotalCount != 40 || !got.Truncated {
		t.Fatalf("filesystems=%+v", got.ListStatus)
	}
	if got := current.Sections.Processes; len(got.Items) != 200 || got.TotalCount != 500 || !got.Truncated {
		t.Fatalf("processes=%+v", got.ListStatus)
	}
	if name := current.Sections.Processes.Items[0].Name; strings.Contains(name, "/") || len(name) > 128 {
		t.Fatalf("unsafe name %q", name)
	}
}

func TestCollectionStateUsesOnlyProjectedSections(t *testing.T) {
	cpu := observation.CPU{LogicalCPUs: 4}
	memory := observation.Memory{}
	uptime := observation.Uptime{}
	filesystems := []observation.Filesystem{}
	processes := []observation.Process{}
	raw := observation.Snapshot{
		ObservedAt: time.Unix(0, 0), Quality: observation.Quality{State: observation.Partial},
		CPU:         observation.Section[observation.CPU]{State: observation.Available, Data: &cpu},
		Memory:      observation.Section[observation.Memory]{State: observation.Available, Data: &memory},
		Uptime:      observation.Section[observation.Uptime]{State: observation.Available, Data: &uptime},
		Filesystems: observation.Section[[]observation.Filesystem]{State: observation.Available, Data: &filesystems},
		Processes:   observation.Section[[]observation.Process]{State: observation.Available, Data: &processes},
		Network:     observation.Section[observation.Network]{State: observation.Unavailable, ReasonCode: observation.ReasonCollectionFailed},
	}
	if got := Current(raw, SystemInfo{}).CollectionState; got != "OK" {
		t.Fatalf("collection_state=%s", got)
	}
}

func TestFailedProjectionMatchesSchemaValidatedFixture(t *testing.T) {
	at := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	raw := observation.Snapshot{
		ObservedAt:  at,
		CPU:         observation.Section[observation.CPU]{State: observation.PermissionDenied, ReasonCode: observation.ReasonPermissionDenied},
		Memory:      observation.Section[observation.Memory]{State: observation.PermissionDenied, ReasonCode: observation.ReasonPermissionDenied},
		Uptime:      observation.Section[observation.Uptime]{State: observation.PermissionDenied, ReasonCode: observation.ReasonPermissionDenied},
		Filesystems: observation.Section[[]observation.Filesystem]{State: observation.Unsupported, ReasonCode: observation.ReasonUnsupported},
		Processes:   observation.Section[[]observation.Process]{State: observation.PermissionDenied, ReasonCode: observation.ReasonPermissionDenied},
	}
	actual, err := json.Marshal(Current(raw, SystemInfo{}))
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile("../../schemas/v1/fixtures/valid/current-snapshot-native-failed.json")
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
		t.Fatalf("failed projection differs from fixture\nactual: %s\nexpected: %s", actual, expected)
	}
}
