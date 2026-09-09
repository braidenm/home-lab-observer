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
const snapshotStaleAfter = 45 * time.Second

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
		current, available := h.config.Source.Current()
		health := h.config.Source.StoreHealth()
		stats := h.config.Source.Stats()
		if !available || current.CollectionState == "FAILED" || stats.LatestCollectionState == "FAILED" || isStale(current.ObservedAt, h.config.Now()) || (health.State != "" && health.State != "AVAILABLE") {
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
	current = projection.RecomputeCollectionState(current)
	markStale(&current, h.config.Now())
	h.writeJSON(w, r, http.StatusOK, current, currentResponseLimit)
}

func (h *handler) series(w http.ResponseWriter, r *http.Request) {
	query, err := parseSeriesQuery(r.URL.RawQuery)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_QUERY", "Invalid query")
		return
	}
	var response series.Response
	if source, ok := h.config.Source.(interface {
		MetricStatuses() map[history.MetricID]projection.SectionStatus
	}); ok {
		response, err = series.Build(r.Context(), h.config.History, h.config.Now(), query.rangeValue, query.metrics, source.MetricStatuses())
	} else {
		response, err = series.Build(r.Context(), h.config.History, h.config.Now(), query.rangeValue, query.metrics)
	}
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
	stale := func(status *projection.SectionStatus) {
		if status.SupportState == "SUPPORTED" && status.ObservedAt != nil && isStale(*status.ObservedAt, now) {
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

func isStale(observedAt, now time.Time) bool {
	return !observedAt.IsZero() && now.UTC().Sub(observedAt.UTC()) > snapshotStaleAfter
}

func addObserverSignals(current projection.CurrentSnapshot, now time.Time, stats scheduler.Stats, health history.Health) projection.CurrentSnapshot {
	type signalInput struct {
		name  string
		value uint64
		unit  string
		state string
	}
	latestState := "OK"
	if stats.LatestCollectionState == "PARTIAL" {
		latestState = "WARN"
	} else if stats.LatestCollectionState == "FAILED" {
		latestState = "ERROR"
	}
	databaseState := "OK"
	if health.State == "DEGRADED" {
		databaseState = "WARN"
	}
	inputs := []signalInput{
		{"latest_collection", 1, "count", latestState},
		{"collections", stats.Collections, "count", "OK"},
		{"collection_failures", stats.CollectionFailures, "count", "OK"},
		{"store_failures", stats.StoreFailures, "count", "OK"},
		{"triggers_dropped", stats.TriggersDropped, "count", "OK"},
		{"history_database_bytes", nonnegativeUint64(health.DatabaseBytes), "bytes", databaseState},
		{"history_retention_dropped", health.DroppedCount(), "count", "OK"},
		{"history_checkpoint_failures", health.CheckpointFailures, "count", "OK"},
		{"history_write_failures", health.WriteFailures, "count", "OK"},
		{"history_sequence_failures", health.SequenceFailures, "count", "OK"},
		{"history_maintenance_failures", health.MaintenanceFailures, "count", "OK"},
	}
	signals := make([]projection.ObserverSignal, 0, len(inputs))
	for _, input := range inputs {
		signals = append(signals, projection.ObserverSignal{Name: input.name, State: input.state, Value: float64(input.value), Unit: input.unit})
	}
	reason := ""
	if health.State != "" && health.State != "AVAILABLE" {
		reason = boundedString(health.ReasonCode, "STORE_DEGRADED", 64)
	} else if stats.LatestCollectionState == "FAILED" {
		reason = "LATEST_COLLECTION_FAILED"
	} else if stats.LatestCollectionState == "PARTIAL" {
		reason = "LATEST_COLLECTION_PARTIAL"
	}
	return projection.WithObserverSignals(current, now.UTC(), signals, reason)
}

func nonnegativeUint64(value int64) uint64 {
	if value < 0 {
		return 0
	}
	return uint64(value)
}
