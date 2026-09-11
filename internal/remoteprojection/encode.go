// Package remoteprojection defines the explicit numeric-host egress boundary.
// It does not enroll, persist, transmit, or authorize observations.
package remoteprojection

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
)

const (
	MaxBytes               = 16 * 1024
	MaxFilesystems         = 16
	MaxExactInteger uint64 = 1<<53 - 1
)

var (
	ErrEnvelope    = errors.New("remote_projection_invalid_envelope")
	ErrHostData    = errors.New("remote_projection_invalid_host_data")
	sourcePattern  = regexp.MustCompile(`^srv_[a-f0-9]{32}$`)
	versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?$`)
)

// Identity contains the exchanged server_id (not connector_instance_id) and verified release metadata.
// Syntax checks here do not authenticate an installation or grant upload consent.
type Identity struct {
	SourceID string
	Version  string
	OS       string
}

// Encode returns only the reviewed producer profile, or nil bytes and a fixed error.
// Incomplete observations become unavailable; malformed eligible data is rejected.
func Encode(raw observation.Snapshot, identity Identity) ([]byte, error) {
	osName, ok := map[string]string{"linux": "Linux", "windows": "Windows", "darwin": "macOS"}[identity.OS]
	if !ok || !sourcePattern.MatchString(identity.SourceID) || len(identity.Version) > 40 || !versionPattern.MatchString(identity.Version) ||
		raw.SchemaVersion != observation.SchemaVersion || raw.ObservedAt.IsZero() || raw.ObservedAt.UTC().Year() < 1 || raw.ObservedAt.UTC().Year() > 9999 ||
		raw.Quality.DurationMS < 0 || raw.Quality.DurationMS > 10000 {
		return nil, ErrEnvelope
	}
	overview, err := projectOverview(raw, osName)
	if err != nil {
		return nil, err
	}
	result := document{
		SchemaVersion: "home-lab-server-snapshot/v1", ProjectionProfile: "numeric-host/v1",
		SourceID: identity.SourceID, CollectorVersion: identity.Version,
		CollectedAt: raw.ObservedAt.UTC().Format(time.RFC3339Nano), DurationMS: raw.Quality.DurationMS,
		Sections: sections{Overview: overview, Containers: excluded(), Processes: excluded(), Services: excluded()},
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > MaxBytes {
		return nil, ErrHostData
	}
	return encoded, nil
}

func projectOverview(raw observation.Snapshot, osName string) (overview, error) {
	if raw.CPU.State != observation.Available || raw.CPU.Data == nil ||
		raw.Memory.State != observation.Available || raw.Memory.Data == nil ||
		raw.Uptime.State != observation.Available || raw.Uptime.Data == nil ||
		raw.Filesystems.State != observation.Available || raw.Filesystems.Data == nil ||
		incomplete(raw.CPU.Quality) || incomplete(raw.Memory.Quality) || incomplete(raw.Uptime.Quality) || incomplete(raw.Filesystems.Quality) {
		return overview{ReasonCode: "HOST_DATA_UNAVAILABLE"}, nil
	}
	cpu, memory, uptime := *raw.CPU.Data, *raw.Memory.Data, *raw.Uptime.Data
	fs := *raw.Filesystems.Data
	if cpu.LogicalCPUs < 1 || cpu.LogicalCPUs > 4096 || math.IsNaN(cpu.UsagePercent) || math.IsInf(cpu.UsagePercent, 0) || cpu.UsagePercent < 0 || cpu.UsagePercent > 100 ||
		memory.TotalBytes > MaxExactInteger || memory.UsedBytes > memory.TotalBytes || memory.SwapTotalBytes > MaxExactInteger || memory.SwapUsedBytes > memory.SwapTotalBytes ||
		uptime.Seconds > MaxExactInteger || len(fs) > MaxFilesystems || raw.Filesystems.Quality.Total != len(fs) || raw.Filesystems.Quality.Samples != len(fs) {
		return overview{}, ErrHostData
	}
	items := make([]filesystem, 0, len(fs))
	for index, item := range fs {
		if item.TotalBytes > MaxExactInteger || item.UsedBytes > item.TotalBytes {
			return overview{}, ErrHostData
		}
		items = append(items, filesystem{Key: fmt.Sprintf("filesystem-%02d", index+1), Title: fmt.Sprintf("Filesystem %d", index+1), TotalBytes: item.TotalBytes, UsedBytes: item.UsedBytes})
	}
	return overview{Available: true, host: &host{
		OperatingSystem: osName, Kernel: "not collected", UptimeSeconds: uptime.Seconds,
		LogicalCPUCount: cpu.LogicalCPUs, CPUUsagePercent: cpu.UsagePercent,
		MemoryTotalBytes: memory.TotalBytes, MemoryUsedBytes: memory.UsedBytes,
		SwapTotalBytes: memory.SwapTotalBytes, SwapUsedBytes: memory.SwapUsedBytes, Filesystems: items,
	}}, nil
}

func incomplete(quality observation.SectionQuality) bool {
	return quality.Truncated || quality.Errors != 0
}

type document struct {
	SchemaVersion     string   `json:"schema_version"`
	ProjectionProfile string   `json:"projection_profile"`
	SourceID          string   `json:"source_id"`
	CollectorVersion  string   `json:"collector_version"`
	CollectedAt       string   `json:"collected_at"`
	DurationMS        int64    `json:"duration_ms"`
	Sections          sections `json:"sections"`
}

type sections struct {
	Overview   overview        `json:"overview"`
	Containers excludedSection `json:"containers"`
	Processes  excludedSection `json:"processes"`
	Services   excludedSection `json:"services"`
}

// Embedding preserves the legacy flat overview while omitting all data on failure.
type overview struct {
	Available  bool   `json:"available"`
	ReasonCode string `json:"reason_code,omitempty"`
	*host
}

type host struct {
	OperatingSystem  string       `json:"operating_system"`
	Kernel           string       `json:"kernel"`
	UptimeSeconds    uint64       `json:"uptime_seconds"`
	LogicalCPUCount  int          `json:"logical_cpu_count"`
	CPUUsagePercent  float64      `json:"cpu_usage_percent"`
	Load1            *float64     `json:"load_1"`
	Load5            *float64     `json:"load_5"`
	Load15           *float64     `json:"load_15"`
	MemoryTotalBytes uint64       `json:"memory_total_bytes"`
	MemoryUsedBytes  uint64       `json:"memory_used_bytes"`
	SwapTotalBytes   uint64       `json:"swap_total_bytes"`
	SwapUsedBytes    uint64       `json:"swap_used_bytes"`
	Filesystems      []filesystem `json:"filesystems"`
}

type filesystem struct {
	Key        string `json:"key"`
	Title      string `json:"title"`
	TotalBytes uint64 `json:"total_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
}

type excludedSection struct {
	Available  bool       `json:"available"`
	ReasonCode string     `json:"reason_code"`
	Items      []struct{} `json:"items"`
}

func excluded() excludedSection {
	return excludedSection{ReasonCode: "NOT_UPLOAD_ELIGIBLE", Items: []struct{}{}}
}
