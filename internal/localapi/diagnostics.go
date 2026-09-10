package localapi

import (
	"net/http"
	"time"

	"github.com/braidenm/home-lab-observer/internal/diagnostics"
)

const diagnosticsResponseLimit = 32 * 1024

type diagnosticsHealthResponse struct {
	SchemaVersion string    `json:"schema_version"`
	GeneratedAt   time.Time `json:"generated_at"`
	Enabled       bool      `json:"enabled"`
	Available     bool      `json:"available"`
	State         string    `json:"state"`
	ReasonCode    *string   `json:"reason_code"`
	Limits        struct {
		MaxFiles       int   `json:"max_files"`
		MaxFileBytes   int64 `json:"max_file_bytes"`
		MaxTotalBytes  int64 `json:"max_total_bytes"`
		MaxRecordBytes int64 `json:"max_record_bytes"`
		MaxAgeSeconds  int64 `json:"max_age_seconds"`
	} `json:"limits"`
	Usage struct {
		TotalBytes int64 `json:"total_bytes"`
		FileCount  int   `json:"file_count"`
	} `json:"usage"`
	Counters struct {
		DroppedRecords uint64 `json:"dropped_records"`
		WriteFailures  uint64 `json:"write_failures"`
	} `json:"counters"`
	Policy struct {
		DataClassification   string `json:"data_classification"`
		ContainsLogContents  bool   `json:"contains_log_contents"`
		ContainsPaths        bool   `json:"contains_paths"`
		RemoteUploadEligible bool   `json:"remote_upload_eligible"`
	} `json:"policy"`
}

func (h *handler) diagnosticsHealth(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		writeProblem(w, http.StatusBadRequest, "INVALID_QUERY", "Invalid query")
		return
	}
	health := diagnostics.Health{
		Enabled: false, Available: false, State: "DISABLED", ReasonCode: "DIAGNOSTICS_DISABLED",
		MaxFiles: diagnostics.MaxFiles, MaxFileBytes: diagnostics.MaxFileBytes, MaxTotalBytes: diagnostics.MaxTotalBytes,
		MaxRecordBytes: diagnostics.MaxRecordBytes, MaxAgeSeconds: diagnostics.MaxAgeSeconds,
	}
	if h.config.Diagnostics != nil {
		health = h.config.Diagnostics.Health()
	}
	response := diagnosticsHealthResponse{
		SchemaVersion: "observer-diagnostics-health/v1", GeneratedAt: h.config.Now().UTC(),
		Enabled: health.Enabled, Available: health.Available, State: health.State,
	}
	if health.ReasonCode != "" {
		reason := health.ReasonCode
		response.ReasonCode = &reason
	}
	response.Limits.MaxFiles = health.MaxFiles
	response.Limits.MaxFileBytes = health.MaxFileBytes
	response.Limits.MaxTotalBytes = health.MaxTotalBytes
	response.Limits.MaxRecordBytes = health.MaxRecordBytes
	response.Limits.MaxAgeSeconds = health.MaxAgeSeconds
	response.Usage.TotalBytes = health.TotalBytes
	response.Usage.FileCount = health.FileCount
	response.Counters.DroppedRecords = health.DroppedRecords
	response.Counters.WriteFailures = health.WriteFailures
	response.Policy.DataClassification = "PUBLIC_METADATA"
	h.writeJSON(w, r, http.StatusOK, response, diagnosticsResponseLimit)
}
