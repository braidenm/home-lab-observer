package localapi

import (
	"strings"
	"time"

	"github.com/braidenm/home-lab-observer/internal/projection"
)

const (
	capabilitiesSchemaVersion = "observer-capabilities/v1"
	capabilitiesResponseLimit = 131_072
	currentResponseLimit      = 1_048_576
)

type capabilitiesResponse struct {
	SchemaVersion string             `json:"schema_version"`
	APIVersion    string             `json:"api_version"`
	GeneratedAt   time.Time          `json:"generated_at"`
	Observer      capabilityObserver `json:"observer"`
	Platform      capabilityPlatform `json:"platform"`
	Policy        capabilityPolicy   `json:"policy"`
	Collectors    []collector        `json:"collectors"`
}

type capabilityObserver struct {
	Version string `json:"version"`
	Mode    string `json:"mode"`
}

type capabilityPlatform struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}

type capabilityPolicy struct {
	ReadOnly           bool                 `json:"read_only"`
	DefaultBind        string               `json:"default_bind"`
	CORSEnabled        bool                 `json:"cors_enabled"`
	RetentionDays      int                  `json:"retention_days"`
	RetentionBytes     int                  `json:"retention_bytes"`
	LogBodies          logBodyPolicy        `json:"log_bodies"`
	ResponseLimitBytes responseLimitsPolicy `json:"response_limits_bytes"`
}

type logBodyPolicy struct {
	DefaultState    string `json:"default_state"`
	MaximumBodyByte int    `json:"maximum_body_bytes"`
	UploadEligible  bool   `json:"upload_eligible"`
}

type responseLimitsPolicy struct {
	Capabilities    int `json:"capabilities"`
	CurrentSnapshot int `json:"current_snapshot"`
}

type collector struct {
	Name               string  `json:"name"`
	SupportState       string  `json:"support_state"`
	ReasonCode         *string `json:"reason_code"`
	DataClassification string  `json:"data_classification"`
	UploadEligible     bool    `json:"upload_eligible"`
}

func buildCapabilities(version string, now time.Time, current projection.CurrentSnapshot, hasCurrent bool) capabilitiesResponse {
	system := projection.NativeSystemInfo()
	result := capabilitiesResponse{
		SchemaVersion: capabilitiesSchemaVersion,
		APIVersion:    "v1",
		GeneratedAt:   now.UTC(),
		Observer:      capabilityObserver{Version: version, Mode: "LOCAL_DASHBOARD"},
		Platform:      capabilityPlatform{OS: system.OS, Architecture: boundedString(system.Architecture, "unknown", 32)},
		Policy: capabilityPolicy{
			ReadOnly: true, DefaultBind: "127.0.0.1", CORSEnabled: false,
			RetentionDays: 7, RetentionBytes: 262_144_000,
			LogBodies:          logBodyPolicy{DefaultState: "OMITTED", MaximumBodyByte: 2048, UploadEligible: false},
			ResponseLimitBytes: responseLimitsPolicy{Capabilities: capabilitiesResponseLimit, CurrentSnapshot: currentResponseLimit},
		},
	}
	states := map[string]projection.SectionStatus{
		"overview": current.Sections.Overview.SectionStatus, "filesystems": current.Sections.Filesystems.SectionStatus,
		"processes": current.Sections.Processes.SectionStatus, "services": current.Sections.Services.SectionStatus,
		"containers": current.Sections.Containers.SectionStatus, "logs": current.Sections.Logs.SectionStatus,
		"observer": current.Sections.Observer.SectionStatus,
	}
	for _, name := range sectionNames {
		classification, eligible := "PUBLIC_METADATA", true
		if name == "processes" || name == "logs" {
			classification, eligible = "LOCAL_SENSITIVE", false
		}
		state, reason := "SUPPORTED", (*string)(nil)
		if name == "services" || name == "containers" || name == "logs" {
			state, reason = "UNSUPPORTED", stringPointer("COLLECTOR_NOT_IMPLEMENTED")
		} else if name != "observer" && hasCurrent {
			state, reason = states[name].SupportState, states[name].ReasonCode
			if state == "" {
				state, reason = "UNAVAILABLE", stringPointer("SNAPSHOT_NOT_AVAILABLE")
			}
		}
		result.Collectors = append(result.Collectors, collector{Name: name, SupportState: state, ReasonCode: reason, DataClassification: classification, UploadEligible: eligible})
	}
	return result
}

func boundedString(value, fallback string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if len(value) > limit {
		value = value[:limit]
	}
	return value
}
