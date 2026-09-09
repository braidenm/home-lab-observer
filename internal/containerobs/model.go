// Package containerobs isolates local, read-only container observations from host metrics and transports.
package containerobs

import "time"

const SchemaVersion = "observer-container-inventory/v1"
const MaxContainers = 500

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
	return Inventory{SchemaVersion: SchemaVersion, SupportState: "DISABLED", CollectionState: "NOT_RUN", Freshness: "UNKNOWN", ReasonCode: &reason, Items: []Container{}, Policy: Policy{ReadOnly: true, DataClassification: "LOCAL_SENSITIVE"}}
}
