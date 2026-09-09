// Package containerobs isolates local, read-only container observations from host metrics and transports.
package containerobs

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

func Disabled() Inventory {
	reason := "DOCKER_NOT_CONFIGURED"
	return Inventory{SchemaVersion: SchemaVersion, SupportState: SupportDisabled, CollectionState: CollectionNotRun, Freshness: FreshnessUnknown, ReasonCode: &reason, Items: []Container{}, Policy: dataPolicy()}
}

func dataPolicy() Policy {
	return Policy{ReadOnly: true, DataClassification: "LOCAL_SENSITIVE", RemoteUploadEligible: false}
}

func cloneInventory(in Inventory) Inventory {
	out := in
	out.ReasonCode = cloneString(in.ReasonCode)
	out.ObservedAt = cloneTime(in.ObservedAt)
	out.Items = make([]Container, len(in.Items))
	for index := range in.Items {
		out.Items[index] = in.Items[index]
		out.Items[index].CPUPercent = cloneFloat(in.Items[index].CPUPercent)
		out.Items[index].MemoryBytes = cloneUint64(in.Items[index].MemoryBytes)
		out.Items[index].ReasonCode = cloneString(in.Items[index].ReasonCode)
	}
	return out
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneUint64(value *uint64) *uint64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
