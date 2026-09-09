package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/braidenm/home-lab-observer/internal/history"
	"github.com/braidenm/home-lab-observer/internal/projection"
	"github.com/braidenm/home-lab-observer/internal/scheduler"
	"github.com/braidenm/home-lab-observer/internal/series"
	"github.com/braidenm/home-lab-observer/internal/webui"
)

const requestTimeout = 5 * time.Second

type Source interface {
	Current() (projection.CurrentSnapshot, bool)
	Stats() scheduler.Stats
	StoreHealth() history.Health
}

type Config struct {
	Port    int
	Token   string
	Version string
	Source  Source
	History series.Reader
	Now     func() time.Time
}

type handler struct {
	config Config
	ui     http.Handler
}

func NewHandler(config Config) (http.Handler, error) {
	if config.Port < 1 || config.Port > 65535 {
		return nil, errors.New("local API port must be between 1 and 65535")
	}
	if strings.TrimSpace(config.Token) == "" || strings.ContainsAny(config.Token, " \t\r\n") {
		return nil, errors.New("local API token is required")
	}
	config.Version = boundedString(config.Version, "dev", 64)
	if config.Source == nil {
		return nil, errors.New("local API source is required")
	}
	if config.History == nil {
		return nil, errors.New("local API history reader is required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	h := &handler{config: config, ui: webui.HTTPHandler()}
	return SecurityHeaders(LocalBoundary(config.Port, http.HandlerFunc(h.route))), nil
}

func (h *handler) route(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		writeProblem(w, http.StatusBadRequest, "REQUEST_BODY_NOT_ALLOWED", "Request body not allowed")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	r = r.WithContext(ctx)
	switch r.URL.Path {
	case "/health/live":
		h.health(w, r, false)
	case "/health/ready":
		h.health(w, r, true)
	default:
		if strings.HasPrefix(r.URL.Path, "/api/") {
			RequireBearer(h.config.Token, http.HandlerFunc(h.api)).ServeHTTP(w, r)
			return
		}
		h.ui.ServeHTTP(w, r)
	}
}

func (h *handler) api(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v1/capabilities":
		if r.URL.RawQuery != "" {
			writeProblem(w, http.StatusBadRequest, "INVALID_QUERY", "Invalid query")
			return
		}
		current, ok := h.config.Source.Current()
		h.writeJSON(w, r, http.StatusOK, buildCapabilities(h.config.Version, h.config.Now(), current, ok), capabilitiesResponseLimit)
	case "/api/v1/snapshots/current":
		h.current(w, r)
	case "/api/v1/metrics/series":
		h.series(w, r)
	default:
		writeProblem(w, http.StatusNotFound, "ENDPOINT_NOT_FOUND", "Endpoint not found")
	}
}

func (h *handler) health(w http.ResponseWriter, r *http.Request, readiness bool) {
	if r.URL.RawQuery != "" {
		writeProblem(w, http.StatusBadRequest, "INVALID_QUERY", "Invalid query")
		return
	}
	status, code := "UP", http.StatusOK
	if readiness {
		_, available := h.config.Source.Current()
		health := h.config.Source.StoreHealth()
		if !available || (health.State != "" && health.State != "AVAILABLE") {
			status, code = "NOT_READY", http.StatusServiceUnavailable
		}
	}
	h.writeJSON(w, r, code, struct {
		Status string `json:"status"`
	}{Status: status}, 1024)
}

func (h *handler) current(w http.ResponseWriter, r *http.Request) {
	query, err := parseCurrentQuery(r.URL.RawQuery)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_QUERY", "Invalid query")
		return
	}
	current, ok := h.config.Source.Current()
	if !ok {
		writeProblem(w, http.StatusServiceUnavailable, "SNAPSHOT_NOT_AVAILABLE", "Snapshot not available")
		return
	}
	current = addObserverSignals(current, h.config.Now(), h.config.Source.Stats(), h.config.Source.StoreHealth())
	current = selectSections(current, query)
	markStale(&current, h.config.Now())
	h.writeJSON(w, r, http.StatusOK, current, currentResponseLimit)
}

func (h *handler) series(w http.ResponseWriter, r *http.Request) {
	query, err := parseSeriesQuery(r.URL.RawQuery)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_QUERY", "Invalid query")
		return
	}
	response, err := series.Build(r.Context(), h.config.History, h.config.Now(), query.rangeValue, query.metrics)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "SERIES_UNAVAILABLE", "Metric series unavailable")
		return
	}
	h.writeJSON(w, r, http.StatusOK, response, series.MaxResponseBytes)
}

func (h *handler) writeJSON(w http.ResponseWriter, r *http.Request, status int, value any, limit int) {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(value); err != nil {
		writeProblem(w, http.StatusInternalServerError, "ENCODING_FAILED", "Response unavailable")
		return
	}
	if encoded.Len() > limit {
		writeProblem(w, http.StatusInternalServerError, "RESPONSE_LIMIT_EXCEEDED", "Response unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", encoded.Len()))
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(encoded.Bytes())
	}
}

func selectSections(current projection.CurrentSnapshot, query currentQuery) projection.CurrentSnapshot {
	if !query.sections["overview"] {
		current.Sections.Overview = projection.OverviewSection{SectionStatus: notSelectedStatus()}
	}
	if !query.sections["filesystems"] {
		current.Sections.Filesystems = projection.FilesystemSection{ListStatus: notSelectedListStatus(), Items: []projection.Filesystem{}}
	}
	if !query.sections["processes"] {
		current.Sections.Processes = projection.ProcessSection{ListStatus: notSelectedListStatus(), Items: []projection.Process{}}
	} else if len(current.Sections.Processes.Items) > query.processLimit {
		current.Sections.Processes.Items = current.Sections.Processes.Items[:query.processLimit]
		current.Sections.Processes.ReturnedCount = len(current.Sections.Processes.Items)
		current.Sections.Processes.Truncated = true
	}
	if !query.sections["services"] {
		current.Sections.Services = projection.EmptySection{ListStatus: notSelectedListStatus(), Items: []any{}}
	}
	if !query.sections["containers"] {
		current.Sections.Containers = projection.EmptySection{ListStatus: notSelectedListStatus(), Items: []any{}}
	}
	if !query.sections["logs"] {
		current.Sections.Logs = projection.EmptySection{ListStatus: notSelectedListStatus(), Items: []any{}}
	}
	if !query.sections["observer"] {
		current.Sections.Observer = projection.ObserverSection{ListStatus: notSelectedListStatus(), Items: []projection.ObserverSignal{}}
	}
	return current
}

func notSelectedStatus() projection.SectionStatus {
	reason := "QUERY_NOT_SELECTED"
	return projection.SectionStatus{SupportState: "DISABLED", CollectionState: "NOT_RUN", Freshness: "UNKNOWN", ReasonCode: &reason}
}

func notSelectedListStatus() projection.ListStatus {
	return projection.ListStatus{SectionStatus: notSelectedStatus()}
}

func markStale(current *projection.CurrentSnapshot, now time.Time) {
	if current.ObservedAt.IsZero() || now.UTC().Sub(current.ObservedAt.UTC()) <= 45*time.Second {
		return
	}
	stale := func(status *projection.SectionStatus) {
		if status.SupportState == "SUPPORTED" && status.ObservedAt != nil {
			status.Freshness = "STALE"
			if status.ReasonCode == nil {
				status.ReasonCode = stringPointer("LATEST_SAMPLE_STALE")
			}
		}
	}
	stale(&current.Sections.Overview.SectionStatus)
	stale(&current.Sections.Filesystems.SectionStatus)
	stale(&current.Sections.Processes.SectionStatus)
	stale(&current.Sections.Services.SectionStatus)
	stale(&current.Sections.Containers.SectionStatus)
	stale(&current.Sections.Logs.SectionStatus)
	stale(&current.Sections.Observer.SectionStatus)
}

func addObserverSignals(current projection.CurrentSnapshot, now time.Time, stats scheduler.Stats, health history.Health) projection.CurrentSnapshot {
	type signalInput struct {
		name  string
		value uint64
		unit  string
		warn  bool
	}
	inputs := []signalInput{
		{"collections", stats.Collections, "count", false},
		{"collection_failures", stats.CollectionFailures, "count", stats.CollectionFailures > 0},
		{"store_failures", stats.StoreFailures, "count", stats.StoreFailures > 0},
		{"triggers_dropped", stats.TriggersDropped, "count", stats.TriggersDropped > 0},
		{"history_database_bytes", nonnegativeUint64(health.DatabaseBytes), "bytes", health.State == "DEGRADED"},
		{"history_retention_dropped", health.DroppedCount(), "count", health.DroppedCount() > 0},
		{"history_checkpoint_failures", health.CheckpointFailures, "count", health.CheckpointFailures > 0},
		{"history_write_failures", health.WriteFailures, "count", health.WriteFailures > 0},
		{"history_sequence_failures", health.SequenceFailures, "count", health.SequenceFailures > 0},
		{"history_maintenance_failures", health.MaintenanceFailures, "count", health.MaintenanceFailures > 0},
	}
	signals := make([]projection.ObserverSignal, 0, len(inputs))
	for _, input := range inputs {
		state := "OK"
		if input.warn {
			state = "WARN"
		}
		signals = append(signals, projection.ObserverSignal{Name: input.name, State: state, Value: float64(input.value), Unit: input.unit})
	}
	reason := ""
	if health.State != "" && health.State != "AVAILABLE" {
		reason = boundedString(health.ReasonCode, "STORE_DEGRADED", 64)
	} else if stats.CollectionFailures > 0 || stats.StoreFailures > 0 {
		reason = "OBSERVER_DEGRADED"
	}
	return projection.WithObserverSignals(current, now.UTC(), signals, reason)
}

func nonnegativeUint64(value int64) uint64 {
	if value < 0 {
		return 0
	}
	return uint64(value)
}
