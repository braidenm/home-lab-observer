package observation

import (
	"fmt"
	"math"
	"time"
)

const SchemaVersion = "home-lab-observer/host-snapshot/v1"

type SupportState string

const (
	Available        SupportState = "AVAILABLE"
	Degraded         SupportState = "DEGRADED"
	Unavailable      SupportState = "UNAVAILABLE"
	Disabled         SupportState = "DISABLED"
	PermissionDenied SupportState = "PERMISSION_DENIED"
	Unsupported      SupportState = "UNSUPPORTED"
	Unknown          SupportState = "UNKNOWN"
)

type QualityState string

const (
	Complete QualityState = "COMPLETE"
	Partial  QualityState = "PARTIAL"
	Failed   QualityState = "FAILED"
)

type CapabilityName string

const (
	HostCPU         CapabilityName = "HOST_CPU"
	HostMemory      CapabilityName = "HOST_MEMORY"
	HostFilesystems CapabilityName = "HOST_FILESYSTEMS"
	HostNetwork     CapabilityName = "HOST_NETWORK"
	HostUptime      CapabilityName = "HOST_UPTIME"
	HostProcesses   CapabilityName = "HOST_PROCESSES"
)

type ReasonCode string

const (
	ReasonCollectionFailed  ReasonCode = "collection_failed"
	ReasonPermissionDenied  ReasonCode = "permission_denied"
	ReasonDeadlineExceeded  ReasonCode = "deadline_exceeded"
	ReasonCollectionStopped ReasonCode = "collection_canceled"
	ReasonPartialCollection ReasonCode = "partial_collection"
	ReasonDisabled          ReasonCode = "disabled_by_configuration"
	ReasonNoData            ReasonCode = "no_data"
	ReasonUnsupported       ReasonCode = "unsupported"
)

type Source struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type Quality struct {
	State               QualityState `json:"state"`
	StartedAt           time.Time    `json:"started_at"`
	FinishedAt          time.Time    `json:"finished_at"`
	DurationMS          int64        `json:"duration_ms"`
	AvailableSections   int          `json:"available_sections"`
	DegradedSections    int          `json:"degraded_sections"`
	UnavailableSections int          `json:"unavailable_sections"`
}

type SectionQuality struct {
	DurationMS int64 `json:"duration_ms"`
	Samples    int   `json:"samples"`
	Total      int   `json:"total"`
	Truncated  bool  `json:"truncated"`
	Errors     int   `json:"errors"`
}

type Capability struct {
	Name       CapabilityName `json:"name"`
	State      SupportState   `json:"state"`
	ReasonCode ReasonCode     `json:"reason_code,omitempty"`
}

type Section[T any] struct {
	State      SupportState   `json:"state"`
	ReasonCode ReasonCode     `json:"reason_code,omitempty"`
	Quality    SectionQuality `json:"quality"`
	Data       *T             `json:"data,omitempty"`
}

type CPU struct {
	LogicalCPUs  int     `json:"logical_cpus"`
	UsagePercent float64 `json:"usage_percent"`
}

type Memory struct {
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsagePercent   float64 `json:"usage_percent"`
	SwapTotalBytes uint64  `json:"swap_total_bytes"`
	SwapUsedBytes  uint64  `json:"swap_used_bytes"`
}

type Filesystem struct {
	ID           string  `json:"id"`
	DisplayName  string  `json:"display_name"`
	Type         string  `json:"type"`
	TotalBytes   uint64  `json:"total_bytes"`
	UsedBytes    uint64  `json:"used_bytes"`
	FreeBytes    uint64  `json:"free_bytes"`
	UsagePercent float64 `json:"usage_percent"`
}

type Network struct {
	BytesSent   uint64 `json:"bytes_sent"`
	BytesRecv   uint64 `json:"bytes_received"`
	PacketsSent uint64 `json:"packets_sent"`
	PacketsRecv uint64 `json:"packets_received"`
	ErrorsIn    uint64 `json:"errors_in"`
	ErrorsOut   uint64 `json:"errors_out"`
	DropsIn     uint64 `json:"drops_in"`
	DropsOut    uint64 `json:"drops_out"`
}

type Uptime struct {
	Seconds uint64 `json:"seconds"`
}

type Process struct {
	PID           int32   `json:"pid"`
	Name          string  `json:"name"`
	State         string  `json:"state"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryBytes   uint64  `json:"memory_bytes"`
	UptimeSeconds uint64  `json:"uptime_seconds"`
}

type Snapshot struct {
	SchemaVersion    string                `json:"schema_version"`
	CollectorVersion string                `json:"collector_version"`
	Source           Source                `json:"source"`
	ObservedAt       time.Time             `json:"observed_at"`
	Quality          Quality               `json:"quality"`
	Capabilities     []Capability          `json:"capabilities"`
	CPU              Section[CPU]          `json:"cpu"`
	Memory           Section[Memory]       `json:"memory"`
	Filesystems      Section[[]Filesystem] `json:"filesystems"`
	Network          Section[Network]      `json:"network"`
	Uptime           Section[Uptime]       `json:"uptime"`
	Processes        Section[[]Process]    `json:"processes"`
}

func (s Snapshot) Validate() error {
	if s.SchemaVersion != SchemaVersion {
		return fmt.Errorf("invalid schema version")
	}
	if s.ObservedAt.IsZero() || s.ObservedAt.Location() != time.UTC {
		return fmt.Errorf("observed_at must be UTC")
	}
	if len(s.Capabilities) != 6 {
		return fmt.Errorf("expected six capabilities")
	}
	if !finitePercent(s.CPU.Data, func(v CPU) float64 { return v.UsagePercent }) || !finitePercent(s.Memory.Data, func(v Memory) float64 { return v.UsagePercent }) {
		return fmt.Errorf("invalid percentage")
	}
	return nil
}

func finitePercent[T any](data *T, get func(T) float64) bool {
	if data == nil {
		return true
	}
	v := get(*data)
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 100
}
