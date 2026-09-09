package projection

import (
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/braidenm/home-lab-observer/internal/observation"
)

const CurrentSnapshotVersion = "observer-current-snapshot/v1"
const MaxProjectedFilesystems = 16
const MaxProjectedProcesses = 200

type SystemInfo struct {
	OS           string
	Architecture string
}

func NativeSystemInfo() SystemInfo {
	name := runtime.GOOS
	if name != "linux" && name != "windows" && name != "darwin" {
		name = "other"
	}
	return SystemInfo{OS: name, Architecture: runtime.GOARCH}
}

type Privacy struct {
	Profile                     string   `json:"profile"`
	RemoteProjection            string   `json:"remote_projection"`
	MessageBodiesUploadEligible bool     `json:"message_bodies_upload_eligible"`
	RedactionCount              int      `json:"redaction_count"`
	DroppedCount                int      `json:"dropped_count"`
	ExcludedFields              []string `json:"excluded_fields"`
}

type OverviewData struct {
	HostAlias        string `json:"host_alias"`
	OS               string `json:"os"`
	Architecture     string `json:"architecture"`
	UptimeSeconds    uint64 `json:"uptime_seconds"`
	CPULogicalCount  int    `json:"cpu_logical_count"`
	MemoryTotalBytes uint64 `json:"memory_total_bytes"`
	MemoryUsedBytes  uint64 `json:"memory_used_bytes"`
}

type SectionStatus struct {
	SupportState    string     `json:"support_state"`
	CollectionState string     `json:"collection_state"`
	Freshness       string     `json:"freshness"`
	ObservedAt      *time.Time `json:"observed_at"`
	ReasonCode      *string    `json:"reason_code"`
}

type OverviewSection struct {
	SectionStatus
	Data *OverviewData `json:"data"`
}

type ListStatus struct {
	SectionStatus
	TotalCount    int  `json:"total_count"`
	ReturnedCount int  `json:"returned_count"`
	Truncated     bool `json:"truncated"`
}

type Filesystem struct {
	MountAlias     string `json:"mount_alias"`
	FilesystemType string `json:"filesystem_type"`
	TotalBytes     uint64 `json:"total_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
}

type Process struct {
	PID         int32   `json:"pid"`
	Name        string  `json:"name"`
	State       string  `json:"state"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryBytes uint64  `json:"memory_bytes"`
}

type FilesystemSection struct {
	ListStatus
	Items []Filesystem `json:"items"`
}

type ProcessSection struct {
	ListStatus
	Items []Process `json:"items"`
}

type EmptySection struct {
	ListStatus
	Items []any `json:"items"`
}

type ObserverSignal struct {
	Name  string  `json:"name"`
	State string  `json:"state"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type ObserverSection struct {
	ListStatus
	Items []ObserverSignal `json:"items"`
}

type Sections struct {
	Overview    OverviewSection   `json:"overview"`
	Filesystems FilesystemSection `json:"filesystems"`
	Processes   ProcessSection    `json:"processes"`
	Services    EmptySection      `json:"services"`
	Containers  EmptySection      `json:"containers"`
	Logs        EmptySection      `json:"logs"`
	Observer    ObserverSection   `json:"observer"`
}

type CurrentSnapshot struct {
	SchemaVersion   string    `json:"schema_version"`
	SnapshotID      string    `json:"snapshot_id"`
	Sequence        uint64    `json:"sequence"`
	ObservedAt      time.Time `json:"observed_at"`
	DurationMS      int64     `json:"duration_ms"`
	CollectionState string    `json:"collection_state"`
	Privacy         Privacy   `json:"privacy"`
	Sections        Sections  `json:"sections"`
}

func Current(raw observation.Snapshot, system SystemInfo) CurrentSnapshot {
	observedAt := raw.ObservedAt.UTC()
	unsupported := unsupportedSection("COLLECTOR_NOT_IMPLEMENTED")
	sections := Sections{
		Overview: projectOverview(raw, system), Filesystems: projectFilesystems(raw), Processes: projectProcesses(raw),
		Services: unsupported, Containers: unsupported, Logs: unsupported, Observer: unsupportedObserverSection("COLLECTOR_NOT_IMPLEMENTED"),
	}
	return CurrentSnapshot{
		SchemaVersion:   CurrentSnapshotVersion,
		SnapshotID:      "snapshot_" + strconv.FormatInt(observedAt.UnixNano(), 10),
		Sequence:        0,
		ObservedAt:      observedAt,
		DurationMS:      clamp(raw.Quality.DurationMS, 0, 60000),
		CollectionState: projectedCollectionState(sections.Overview.SectionStatus, sections.Filesystems.SectionStatus, sections.Processes.SectionStatus),
		Privacy: Privacy{
			Profile: "SAFE_DEFAULT", RemoteProjection: "home-lab-server-snapshot/v1",
			MessageBodiesUploadEligible: false, RedactionCount: 0, DroppedCount: 0,
			ExcludedFields: []string{"process_arguments", "environment_variables", "container_commands", "container_mounts", "container_labels", "raw_log_bodies", "credentials", "tokens"},
		},
		Sections: sections,
	}
}

func WithObserverSignals(current CurrentSnapshot, observedAt time.Time, signals []ObserverSignal, degradedReason string) CurrentSnapshot {
	if len(signals) > 32 {
		signals = signals[:32]
	}
	state := SectionStatus{SupportState: "SUPPORTED", CollectionState: "OK", Freshness: "CURRENT", ObservedAt: timePtr(observedAt.UTC())}
	if degradedReason != "" {
		state.CollectionState = "PARTIAL"
		state.ReasonCode = stringPtr(safeToken(degradedReason, "OBSERVER_DEGRADED", 64))
	}
	items := append([]ObserverSignal(nil), signals...)
	current.Sections.Observer = ObserverSection{ListStatus: ListStatus{SectionStatus: state, TotalCount: len(items), ReturnedCount: len(items)}, Items: items}
	return current
}

func projectOverview(raw observation.Snapshot, system SystemInfo) OverviewSection {
	states := []observation.SupportState{raw.CPU.State, raw.Memory.State, raw.Uptime.State}
	if raw.CPU.Data == nil || raw.Memory.Data == nil || raw.Uptime.Data == nil {
		state, reason := unavailableStatus(states, raw.CPU.ReasonCode, raw.Memory.ReasonCode, raw.Uptime.ReasonCode)
		return OverviewSection{SectionStatus: stateWithReason(state, reason), Data: nil}
	}
	state := observation.Available
	reason := observation.ReasonCode("")
	for index, candidate := range states {
		if candidate == observation.Degraded {
			state = observation.Degraded
			reason = []observation.ReasonCode{raw.CPU.ReasonCode, raw.Memory.ReasonCode, raw.Uptime.ReasonCode}[index]
		}
	}
	status := statusFor(state, reason, raw.ObservedAt)
	data := &OverviewData{HostAlias: "local-host", OS: normalizedOS(system.OS), Architecture: safeToken(system.Architecture, "unknown", 32), UptimeSeconds: raw.Uptime.Data.Seconds, CPULogicalCount: raw.CPU.Data.LogicalCPUs, MemoryTotalBytes: raw.Memory.Data.TotalBytes, MemoryUsedBytes: raw.Memory.Data.UsedBytes}
	return OverviewSection{SectionStatus: status, Data: data}
}

func projectFilesystems(raw observation.Snapshot) FilesystemSection {
	status := statusFor(raw.Filesystems.State, raw.Filesystems.ReasonCode, raw.ObservedAt)
	result := FilesystemSection{ListStatus: listStatus(status, raw.Filesystems.Quality), Items: []Filesystem{}}
	if raw.Filesystems.Data == nil || status.SupportState != "SUPPORTED" {
		return result
	}
	items := *raw.Filesystems.Data
	total := max(raw.Filesystems.Quality.Total, len(items))
	if len(items) > MaxProjectedFilesystems {
		items = items[:MaxProjectedFilesystems]
	}
	for _, item := range items {
		result.Items = append(result.Items, Filesystem{MountAlias: item.ID, FilesystemType: safeToken(item.Type, "unknown", 32), TotalBytes: item.TotalBytes, UsedBytes: item.UsedBytes, AvailableBytes: item.FreeBytes})
	}
	result.TotalCount, result.ReturnedCount = total, len(result.Items)
	result.Truncated = raw.Filesystems.Quality.Truncated || total > len(result.Items)
	return result
}

func projectProcesses(raw observation.Snapshot) ProcessSection {
	status := statusFor(raw.Processes.State, raw.Processes.ReasonCode, raw.ObservedAt)
	result := ProcessSection{ListStatus: listStatus(status, raw.Processes.Quality), Items: []Process{}}
	if raw.Processes.Data == nil || status.SupportState != "SUPPORTED" {
		return result
	}
	items := *raw.Processes.Data
	total := max(raw.Processes.Quality.Total, len(items))
	if len(items) > MaxProjectedProcesses {
		items = items[:MaxProjectedProcesses]
	}
	for _, item := range items {
		result.Items = append(result.Items, Process{PID: item.PID, Name: safeProcessName(item.Name), State: safeToken(item.State, "unknown", 32), CPUPercent: clampFloat(item.CPUPercent, 0, 100), MemoryBytes: item.MemoryBytes})
	}
	result.TotalCount, result.ReturnedCount = total, len(result.Items)
	result.Truncated = raw.Processes.Quality.Truncated || total > len(result.Items)
	return result
}

func listStatus(status SectionStatus, quality observation.SectionQuality) ListStatus {
	if status.SupportState != "SUPPORTED" {
		return ListStatus{SectionStatus: status}
	}
	total := quality.Total
	if total < quality.Samples {
		total = quality.Samples
	}
	return ListStatus{SectionStatus: status, TotalCount: total, ReturnedCount: quality.Samples, Truncated: quality.Truncated || total > quality.Samples}
}

func unsupportedSection(reason string) EmptySection {
	return EmptySection{ListStatus: ListStatus{SectionStatus: stateWithReason("UNSUPPORTED", reason)}, Items: []any{}}
}

func unsupportedObserverSection(reason string) ObserverSection {
	return ObserverSection{ListStatus: ListStatus{SectionStatus: stateWithReason("UNSUPPORTED", reason)}, Items: []ObserverSignal{}}
}

func statusFor(state observation.SupportState, reason observation.ReasonCode, observed time.Time) SectionStatus {
	switch state {
	case observation.Available:
		return SectionStatus{SupportState: "SUPPORTED", CollectionState: "OK", Freshness: "CURRENT", ObservedAt: timePtr(observed.UTC())}
	case observation.Degraded:
		return SectionStatus{SupportState: "SUPPORTED", CollectionState: "PARTIAL", Freshness: "CURRENT", ObservedAt: timePtr(observed.UTC()), ReasonCode: reasonPtr(reason, "PARTIAL_COLLECTION")}
	case observation.Disabled:
		return stateWithReason("DISABLED", reasonString(reason, "DISABLED_BY_CONFIGURATION"))
	case observation.PermissionDenied:
		return stateWithReason("PERMISSION_DENIED", reasonString(reason, "PERMISSION_DENIED"))
	case observation.Unavailable:
		return stateWithReason("UNAVAILABLE", reasonString(reason, "COLLECTION_FAILED"))
	case observation.Unsupported:
		return stateWithReason("UNSUPPORTED", reasonString(reason, "UNSUPPORTED"))
	default:
		return stateWithReason("UNSUPPORTED", "UNKNOWN_SUPPORT_STATE")
	}
}

func unavailableStatus(states []observation.SupportState, reasons ...observation.ReasonCode) (string, string) {
	for index, state := range states {
		if state == observation.PermissionDenied {
			return "PERMISSION_DENIED", reasonString(reasons[index], "PERMISSION_DENIED")
		}
	}
	for index, state := range states {
		if state == observation.Disabled {
			return "DISABLED", reasonString(reasons[index], "DISABLED_BY_CONFIGURATION")
		}
	}
	for index, state := range states {
		if state == observation.Unavailable {
			return "UNAVAILABLE", reasonString(reasons[index], "COLLECTION_FAILED")
		}
	}
	for index, state := range states {
		if state == observation.Unsupported {
			return "UNSUPPORTED", reasonString(reasons[index], "UNSUPPORTED")
		}
	}
	return "UNSUPPORTED", "OVERVIEW_INCOMPLETE"
}

func stateWithReason(state, reason string) SectionStatus {
	return SectionStatus{SupportState: state, CollectionState: "NOT_RUN", Freshness: "UNKNOWN", ReasonCode: &reason}
}
func projectedCollectionState(statuses ...SectionStatus) string {
	successes, failures, partial := 0, 0, false
	for _, status := range statuses {
		if status.SupportState == "DISABLED" || status.SupportState == "UNSUPPORTED" {
			continue
		}
		switch status.CollectionState {
		case "OK":
			successes++
		case "PARTIAL":
			successes++
			partial = true
		default:
			failures++
		}
	}
	if successes == 0 {
		return "FAILED"
	}
	if partial || failures > 0 {
		return "PARTIAL"
	}
	return "OK"
}
func reasonPtr(reason observation.ReasonCode, fallback string) *string {
	v := reasonString(reason, fallback)
	return &v
}
func reasonString(reason observation.ReasonCode, fallback string) string {
	if reason == "" {
		return fallback
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToUpper(r)
		}
		return '_'
	}, string(reason))
}
func stringPtr(value string) *string     { return &value }
func timePtr(value time.Time) *time.Time { return &value }
func normalizedOS(value string) string {
	if value == "linux" || value == "windows" || value == "darwin" {
		return value
	}
	return "other"
}
func safeToken(value, fallback string, limit int) string {
	value = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(value, "")))
	if value == "" {
		return fallback
	}
	for len(value) > limit {
		_, size := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-size]
	}
	return value
}
func safeProcessName(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	if index := strings.LastIndexByte(value, '/'); index >= 0 {
		value = value[index+1:]
	}
	value = safeToken(value, "process", 128)
	for len(value) > 128 {
		_, size := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-size]
	}
	return value
}
func clamp(value, low, high int64) int64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
func clampFloat(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
