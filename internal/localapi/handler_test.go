package localapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/history"
	"github.com/braidenm/home-lab-observer/internal/projection"
	"github.com/braidenm/home-lab-observer/internal/scheduler"
)

const testToken = "0123456789012345678901234567890123456789012"

type fakeSource struct {
	current projection.CurrentSnapshot
	ok      bool
	stats   scheduler.Stats
	health  history.Health
}

func (s fakeSource) Current() (projection.CurrentSnapshot, bool) { return s.current, s.ok }
func (s fakeSource) Stats() scheduler.Stats                      { return s.stats }
func (s fakeSource) StoreHealth() history.Health                 { return s.health }

type fakeMetricSource struct {
	fakeSource
	statuses map[history.MetricID]projection.SectionStatus
}

func (s fakeMetricSource) MetricStatuses() map[history.MetricID]projection.SectionStatus {
	return s.statuses
}

type handlerReader struct {
	samples map[history.MetricID][]history.Sample
	err     error
}

func (r handlerReader) Samples(_ context.Context, metric history.MetricID, _, _ time.Time, _ int) ([]history.Sample, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.samples[metric], nil
}
func (r handlerReader) Rollups(context.Context, history.MetricID, time.Time, time.Time, int) ([]history.Rollup, error) {
	if r.err != nil {
		return nil, r.err
	}
	return []history.Rollup{}, nil
}

func TestNewHandlerValidatesDependencies(t *testing.T) {
	source := fakeSource{}
	reader := handlerReader{}
	valid := Config{Port: 9847, Token: testToken, Version: "0.1.0", Source: source, History: reader}
	if _, err := NewHandler(valid); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	for _, mutate := range []func(*Config){
		func(c *Config) { c.Port = 0 }, func(c *Config) { c.Token = "" }, func(c *Config) { c.Token = "has space" },
		func(c *Config) { c.Source = nil }, func(c *Config) { c.History = nil },
	} {
		candidate := valid
		mutate(&candidate)
		if _, err := NewHandler(candidate); err == nil {
			t.Fatal("accepted invalid handler configuration")
		}
	}
}

func TestHealthIsPublicMinimalAndReadinessReflectsSource(t *testing.T) {
	now := time.Date(2026, 9, 9, 18, 0, 0, 0, time.UTC)
	ready := newTestHandler(t, fakeSource{
		current: snapshotAt(now),
		ok:      true,
		stats: scheduler.Stats{
			Collections:           12,
			CollectionFailures:    2,
			StoreFailures:         1,
			LatestCollectionState: "OK",
			LatestCollectionAt:    now,
		},
		health: history.Health{State: "AVAILABLE"},
	}, handlerReader{}, now)
	for _, test := range []struct {
		path   string
		status int
		body   string
	}{{"/health/live", 200, `{"status":"UP"}`}, {"/health/ready", 200, `{"status":"UP"}`}} {
		response := serve(ready, http.MethodGet, test.path, "", "")
		if response.Code != test.status || strings.TrimSpace(response.Body.String()) != test.body {
			t.Fatalf("%s code/body = %d/%q", test.path, response.Code, response.Body.String())
		}
	}
	notReady := newTestHandler(t, fakeSource{health: history.Health{State: "DEGRADED"}}, handlerReader{}, now)
	response := serve(notReady, http.MethodGet, "/health/ready", "", "")
	if response.Code != http.StatusServiceUnavailable || strings.TrimSpace(response.Body.String()) != `{"status":"NOT_READY"}` {
		t.Fatalf("not-ready response=%d/%q", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "DEGRADED") {
		t.Fatal("readiness leaked store details")
	}
	for name, source := range map[string]fakeSource{
		"failed current": {current: func() projection.CurrentSnapshot {
			value := snapshotAt(now)
			value.CollectionState = "FAILED"
			return value
		}(), ok: true, health: history.Health{State: "AVAILABLE"}},
		"latest failed": {current: snapshotAt(now), ok: true, stats: scheduler.Stats{LatestCollectionState: "FAILED", LatestCollectionAt: now}, health: history.Health{State: "AVAILABLE"}},
		"stale current": {current: snapshotAt(now.Add(-snapshotStaleAfter - time.Second)), ok: true, stats: scheduler.Stats{LatestCollectionState: "OK"}, health: history.Health{State: "AVAILABLE"}},
	} {
		t.Run(name, func(t *testing.T) {
			response := serve(newTestHandler(t, source, handlerReader{}, now), http.MethodGet, "/health/ready", "", "")
			if response.Code != http.StatusServiceUnavailable || strings.TrimSpace(response.Body.String()) != `{"status":"NOT_READY"}` {
				t.Fatalf("response=%d/%q", response.Code, response.Body.String())
			}
		})
	}
}

func TestProtectedRoutesCapabilitiesAndJSONNotFound(t *testing.T) {
	now := time.Date(2026, 9, 9, 18, 0, 0, 0, time.UTC)
	handler := newTestHandler(t, fakeSource{current: snapshotAt(now), ok: true}, handlerReader{}, now)
	unauthorized := serve(handler, http.MethodGet, "/api/v1/capabilities", "", "")
	if unauthorized.Code != http.StatusUnauthorized || unauthorized.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("unauthorized=%d/%s", unauthorized.Code, unauthorized.Header().Get("Content-Type"))
	}
	response := serve(handler, http.MethodGet, "/api/v1/capabilities", testToken, "")
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("capabilities=%d headers=%v", response.Code, response.Header())
	}
	var capability map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &capability); err != nil {
		t.Fatal(err)
	}
	if capability["schema_version"] != capabilitiesSchemaVersion {
		t.Fatalf("schema=%v", capability["schema_version"])
	}
	collectors := capability["collectors"].([]any)
	if len(collectors) != 7 {
		t.Fatalf("collectors=%d", len(collectors))
	}
	states := map[string]string{}
	for _, value := range collectors {
		item := value.(map[string]any)
		states[item["name"].(string)] = item["support_state"].(string)
	}
	if states["observer"] != "SUPPORTED" || states["services"] != "UNSUPPORTED" || states["logs"] != "UNSUPPORTED" {
		t.Fatalf("collector states=%v", states)
	}
	notFound := serve(handler, http.MethodGet, "/api/v1/nope", testToken, "")
	if notFound.Code != http.StatusNotFound || notFound.Header().Get("Content-Type") != "application/problem+json" || strings.Contains(notFound.Body.String(), "<html") {
		t.Fatalf("api not found=%d/%q", notFound.Code, notFound.Body.String())
	}
	ui := serve(handler, http.MethodGet, "/workloads", "", "")
	if ui.Code != http.StatusOK || !strings.Contains(ui.Body.String(), `<div id="root"></div>`) {
		t.Fatalf("static fallback=%d", ui.Code)
	}
}

func TestCurrentSelectionLimitsStalenessAndObserverSignals(t *testing.T) {
	now := time.Date(2026, 9, 9, 18, 0, 0, 0, time.UTC)
	old := now.Add(-time.Minute)
	snapshot := snapshotAt(old)
	for index := 0; index < 60; index++ {
		snapshot.Sections.Processes.Items = append(snapshot.Sections.Processes.Items, projection.Process{PID: int32(index + 1), Name: "worker", State: "running"})
	}
	snapshot.Sections.Processes.TotalCount = 60
	snapshot.Sections.Processes.ReturnedCount = 60
	source := fakeSource{current: snapshot, ok: true, stats: scheduler.Stats{Collections: 5, CollectionFailures: 1, LatestCollectionState: "FAILED", LatestCollectionAt: old}, health: history.Health{State: "DEGRADED", ReasonCode: "STORAGE_PRESSURE", DatabaseBytes: 99, SizeDropped: 3}}
	handler := newTestHandler(t, source, handlerReader{}, now)
	response := serve(handler, http.MethodGet, "/api/v1/snapshots/current?section=processes&section=observer", testToken, "")
	if response.Code != http.StatusOK {
		t.Fatalf("current=%d %s", response.Code, response.Body.String())
	}
	var current projection.CurrentSnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if len(current.Sections.Processes.Items) != 50 || current.Sections.Processes.ReturnedCount != 50 || current.Sections.Processes.TotalCount != 60 || !current.Sections.Processes.Truncated {
		t.Fatalf("process bounds=%+v", current.Sections.Processes.ListStatus)
	}
	if current.Sections.Processes.Freshness != "STALE" || current.Sections.Processes.ReasonCode == nil || *current.Sections.Processes.ReasonCode != "LATEST_SAMPLE_STALE" {
		t.Fatalf("process freshness=%+v", current.Sections.Processes.SectionStatus)
	}
	if current.Sections.Overview.SupportState != "DISABLED" || current.Sections.Overview.CollectionState != "NOT_RUN" || current.Sections.Overview.Data != nil || *current.Sections.Overview.ReasonCode != "QUERY_NOT_SELECTED" {
		t.Fatalf("unselected overview=%+v", current.Sections.Overview)
	}
	if current.Sections.Services.SupportState != "DISABLED" || current.Sections.Services.Items == nil || current.Sections.Services.ObservedAt != nil {
		t.Fatalf("unselected service envelope=%+v", current.Sections.Services)
	}
	if current.Sections.Observer.CollectionState != "PARTIAL" || current.Sections.Observer.Freshness != "CURRENT" || current.Sections.Observer.ReasonCode == nil || *current.Sections.Observer.ReasonCode != "STORAGE_PRESSURE" || len(current.Sections.Observer.Items) != 11 {
		t.Fatalf("observer section=%+v", current.Sections.Observer)
	}
	for _, signal := range current.Sections.Observer.Items {
		if signal.Name == "collection_failures" && signal.State != "OK" {
			t.Fatalf("historical counter latched health state: %+v", signal)
		}
		if signal.Name == "latest_collection" && signal.State != "ERROR" {
			t.Fatalf("latest collection state missing: %+v", signal)
		}
	}
}

func TestMarkStaleUsesEachSectionTimestamp(t *testing.T) {
	now := time.Date(2026, 9, 9, 18, 0, 0, 0, time.UTC)
	current := snapshotAt(now)
	old := now.Add(-snapshotStaleAfter - time.Second)
	current.Sections.Processes.ObservedAt = &old
	markStale(&current, now)
	if current.Sections.Processes.Freshness != "STALE" || current.Sections.Overview.Freshness != "CURRENT" {
		t.Fatalf("section freshness processes=%s overview=%s", current.Sections.Processes.Freshness, current.Sections.Overview.Freshness)
	}
}

func TestSeriesUsesBuilderAndConvertsFailuresToSafeProblems(t *testing.T) {
	now := time.Date(2026, 9, 9, 18, 0, 0, 0, time.UTC)
	reader := handlerReader{samples: map[history.MetricID][]history.Sample{
		history.CPUUtilization: {{Metric: history.CPUUtilization, At: now.Add(-10 * time.Second), Value: 25}},
	}}
	handler := newTestHandler(t, fakeSource{}, reader, now)
	response := serve(handler, http.MethodGet, "/api/v1/metrics/series?range=1h&metric=cpu.utilization.percent", testToken, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"value":25`) || !strings.Contains(response.Body.String(), `"value":null`) {
		t.Fatalf("series=%d %s", response.Code, response.Body.String())
	}
	invalid := serve(handler, http.MethodGet, "/api/v1/metrics/series?range=forever&metric=cpu.utilization.percent", testToken, "")
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), `"code":"INVALID_QUERY"`) {
		t.Fatalf("invalid=%d %s", invalid.Code, invalid.Body.String())
	}
	failing := newTestHandler(t, fakeSource{}, handlerReader{err: errors.New("secret database path")}, now)
	failure := serve(failing, http.MethodGet, "/api/v1/metrics/series?range=1h&metric=process.count", testToken, "")
	if failure.Code != http.StatusInternalServerError || strings.Contains(failure.Body.String(), "secret database path") {
		t.Fatalf("unsafe failure=%d %s", failure.Code, failure.Body.String())
	}
}

func TestSeriesUsesLatestMetricSupportWhenSourceProvidesIt(t *testing.T) {
	now := time.Date(2026, 9, 9, 18, 0, 0, 0, time.UTC)
	reason := "PERMISSION_DENIED"
	source := fakeMetricSource{fakeSource: fakeSource{}, statuses: map[history.MetricID]projection.SectionStatus{
		history.ProcessCount: {SupportState: "PERMISSION_DENIED", CollectionState: "NOT_RUN", Freshness: "UNKNOWN", ReasonCode: &reason},
	}}
	handler := newTestHandler(t, source, handlerReader{}, now)
	response := serve(handler, http.MethodGet, "/api/v1/metrics/series?range=1h&metric=process.count", testToken, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"support_state":"PERMISSION_DENIED"`) || !strings.Contains(response.Body.String(), `"points":[]`) {
		t.Fatalf("series support=%d %s", response.Code, response.Body.String())
	}
}

func TestBodyQueryBoundaryAndSecretCanary(t *testing.T) {
	now := time.Now().UTC()
	handler := newTestHandler(t, fakeSource{current: snapshotAt(now), ok: true}, handlerReader{}, now)
	for _, test := range []struct {
		method, target, token, body string
		status                      int
	}{
		{http.MethodGet, "/api/v1/capabilities?extra=value", testToken, "", http.StatusBadRequest},
		{http.MethodGet, "/health/live?details=true", "", "", http.StatusBadRequest},
		{http.MethodGet, "/api/v1/capabilities", testToken, "secret-canary", http.StatusBadRequest},
		{http.MethodPost, "/api/v1/capabilities", testToken, "", http.StatusMethodNotAllowed},
	} {
		response := serve(handler, test.method, test.target, test.token, test.body)
		if response.Code != test.status || strings.Contains(response.Body.String(), "secret-canary") || strings.Contains(response.Body.String(), testToken) {
			t.Errorf("%s %s=%d/%q", test.method, test.target, response.Code, response.Body.String())
		}
	}
}

func snapshotAt(at time.Time) projection.CurrentSnapshot {
	status := projection.SectionStatus{SupportState: "SUPPORTED", CollectionState: "OK", Freshness: "CURRENT", ObservedAt: &at}
	unsupportedReason := "COLLECTOR_NOT_IMPLEMENTED"
	unsupported := projection.ListStatus{SectionStatus: projection.SectionStatus{SupportState: "UNSUPPORTED", CollectionState: "NOT_RUN", Freshness: "UNKNOWN", ReasonCode: &unsupportedReason}}
	return projection.CurrentSnapshot{
		SchemaVersion: projection.CurrentSnapshotVersion, SnapshotID: "snapshot_12345678", Sequence: 9, ObservedAt: at, CollectionState: "OK",
		Privacy: projection.Privacy{Profile: "SAFE_DEFAULT", RemoteProjection: "home-lab-server-snapshot/v1", ExcludedFields: []string{}},
		Sections: projection.Sections{
			Overview:    projection.OverviewSection{SectionStatus: status, Data: &projection.OverviewData{HostAlias: "local-host", OS: "linux", Architecture: "amd64", CPULogicalCount: 8}},
			Filesystems: projection.FilesystemSection{ListStatus: projection.ListStatus{SectionStatus: status}, Items: []projection.Filesystem{}},
			Processes:   projection.ProcessSection{ListStatus: projection.ListStatus{SectionStatus: status}, Items: []projection.Process{}},
			Services:    projection.EmptySection{ListStatus: unsupported, Items: []any{}}, Containers: projection.EmptySection{ListStatus: unsupported, Items: []any{}}, Logs: projection.EmptySection{ListStatus: unsupported, Items: []any{}},
			Observer: projection.ObserverSection{ListStatus: unsupported, Items: []projection.ObserverSignal{}},
		},
	}
}

func newTestHandler(t *testing.T, source Source, reader handlerReader, now time.Time) http.Handler {
	t.Helper()
	handler, err := NewHandler(Config{Port: 9847, Token: testToken, Version: "0.1.0", Source: source, History: reader, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func serve(handler http.Handler, method, target, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Host = "127.0.0.1:9847"
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
