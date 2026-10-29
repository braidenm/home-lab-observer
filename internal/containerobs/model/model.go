// Package model contains normalized container observation data and constants,
// without container acquisition, transport, or host inspection dependencies.
package model

import "time"

const SchemaVersion = "observer-container-inventory/v1"
const MaxContainers = 500

const (
	SupportSupported        = "SUPPORTED"
	SupportDisabled         = "DISABLED"
	SupportUnavailable      = "UNAVAILABLE"
	SupportPermissionDenied = "PERMISSION_DENIED"
	SupportUnsupported      = "UNSUPPORTED"

	CollectionOK      = "OK"
	CollectionPartial = "PARTIAL"
	CollectionNotRun  = "NOT_RUN"

	FreshnessCurrent = "CURRENT"
	FreshnessUnknown = "UNKNOWN"

	MetricsAvailable   = "AVAILABLE"
	MetricsPartial     = "PARTIAL"
	MetricsUnavailable = "UNAVAILABLE"
	MetricsNotRunning  = "NOT_RUNNING"

	StateUnknown    = "unknown"
	StateCreated    = "created"
	StateRunning    = "running"
	StatePaused     = "paused"
	StateRestarting = "restarting"
	StateRemoving   = "removing"
	StateExited     = "exited"
	StateDead       = "dead"
)

type Policy struct {
	ReadOnly             bool   `json:"read_only"`
	DataClassification   string `json:"data_classification"`
	RemoteUploadEligible bool   `json:"remote_upload_eligible"`
}

type Container struct {
	IDAlias      string   `json:"id_alias"`
	Name         string   `json:"name"`
	Image        string   `json:"image"`
	State        string   `json:"state"`
	CPUPercent   *float64 `json:"cpu_percent"`
	MemoryBytes  *uint64  `json:"memory_bytes"`
	MetricsState string   `json:"metrics_state"`
	ReasonCode   *string  `json:"reason_code"`
}

type Inventory struct {
	SchemaVersion   string      `json:"schema_version"`
	ObservedAt      *time.Time  `json:"observed_at"`
	SupportState    string      `json:"support_state"`
	CollectionState string      `json:"collection_state"`
	Freshness       string      `json:"freshness"`
	ReasonCode      *string     `json:"reason_code"`
	TotalCount      int         `json:"total_count"`
	ReturnedCount   int         `json:"returned_count"`
	Truncated       bool        `json:"truncated"`
	Items           []Container `json:"items"`
	Policy          Policy      `json:"policy"`
}
