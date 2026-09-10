package localapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

type fakeLogSummarySource struct {
	queries []logobs.SummaryQuery
	build   func(logobs.SummaryQuery) (logobs.Summary, error)
}

func (source *fakeLogSummarySource) Summary(_ context.Context, query logobs.SummaryQuery) (logobs.Summary, error) {
	source.queries = append(source.queries, query)
	return source.build(query)
}

func TestLogSummaryIsAuthenticatedReadOnlyAndClosed(t *testing.T) {
	now := time.Date(2026, 9, 9, 13, 0, 17, 123_456_789, time.UTC)
	source := &fakeLogSummarySource{build: func(query logobs.SummaryQuery) (logobs.Summary, error) {
		return fullLogSummary(query, 1), nil
	}}
	handler := newLogSummaryHandler(t, source, func() time.Time { return now })

	unauthorizedHead := serve(handler, http.MethodHead, "/api/v1/logs/summary?range=1h", "", "")
	if unauthorizedHead.Code != http.StatusUnauthorized || unauthorizedHead.Body.Len() != 0 || len(source.queries) != 0 {
		t.Fatalf("unauthorized HEAD=%d body=%d source_calls=%d", unauthorizedHead.Code, unauthorizedHead.Body.Len(), len(source.queries))
	}
	unauthorized := serve(handler, http.MethodGet, "/api/v1/logs/summary?range=1h", "", "")
	if unauthorized.Code != http.StatusUnauthorized || len(source.queries) != 0 {
		t.Fatalf("unauthorized request=%d source_calls=%d", unauthorized.Code, len(source.queries))
	}
	response := serve(handler, http.MethodGet, "/api/v1/logs/summary?range=1h", testToken, "")
	if response.Code != http.StatusOK || response.Body.Len() > logSummaryResponseLimit || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("GET=%d bytes=%d headers=%v", response.Code, response.Body.Len(), response.Header())
	}
	assertLogSummaryJSONShape(t, response.Body.Bytes())
	for _, forbidden := range []string{"synthetic-secret", "log body", `"event_code":`, "provider_name", "native_payload", "checkpoint"} {
		if strings.Contains(strings.ToLower(response.Body.String()), forbidden) {
			t.Fatalf("response contains forbidden metadata %q", forbidden)
		}
	}

	head := serve(handler, http.MethodHead, "/api/v1/logs/summary?range=1h", testToken, "")
	if head.Code != response.Code || head.Body.Len() != 0 || head.Header().Get("Content-Length") != response.Header().Get("Content-Length") {
		t.Fatalf("HEAD=%d body=%d headers=%v", head.Code, head.Body.Len(), head.Header())
	}
	post := serve(handler, http.MethodPost, "/api/v1/logs/summary?range=1h", testToken, "")
	if post.Code != http.StatusMethodNotAllowed || len(source.queries) != 2 {
		t.Fatalf("POST=%d source_calls=%d", post.Code, len(source.queries))
	}
	withBody := serve(handler, http.MethodGet, "/api/v1/logs/summary?range=1h", testToken, `{}`)
	if withBody.Code != http.StatusBadRequest || len(source.queries) != 2 {
		t.Fatalf("body request=%d source_calls=%d", withBody.Code, len(source.queries))
	}
}

func TestLogSummaryRejectsEveryNonContractQueryWithoutReadingSource(t *testing.T) {
	now := time.Date(2026, 9, 9, 13, 0, 17, 0, time.UTC)
	source := &fakeLogSummarySource{build: func(query logobs.SummaryQuery) (logobs.Summary, error) {
		return fullLogSummary(query, 1), nil
	}}
	handler := newLogSummaryHandler(t, source, func() time.Time { return now })
	for _, target := range []string{
		"/api/v1/logs/summary", "/api/v1/logs/summary?range=", "/api/v1/logs/summary?range=30m",
		"/api/v1/logs/summary?range=1h&range=6h", "/api/v1/logs/summary?range=1h&source=system",
		"/api/v1/logs/summary?range=1h&query=error", "/api/v1/logs/summary?range=%201h",
	} {
		response := serve(handler, http.MethodGet, target, testToken, "")
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"INVALID_QUERY"`) {
			t.Fatalf("%s = %d %s", target, response.Code, response.Body.String())
		}
	}
	invalidHead := serve(handler, http.MethodHead, "/api/v1/logs/summary?range=forever", testToken, "")
	if invalidHead.Code != http.StatusBadRequest || invalidHead.Body.Len() != 0 || invalidHead.Header().Get("Content-Length") == "" {
		t.Fatalf("invalid HEAD=%d body=%d headers=%v", invalidHead.Code, invalidHead.Body.Len(), invalidHead.Header())
	}
	if len(source.queries) != 0 {
		t.Fatalf("invalid requests read source %d times", len(source.queries))
	}
}

func TestLogSummaryAbsentFailureAndInvalidDataUseBoundedProblems(t *testing.T) {
	now := time.Date(2026, 9, 9, 13, 0, 17, 0, time.UTC)
	absent := newLogSummaryHandler(t, nil, func() time.Time { return now })
	assertLogSummaryProblem(t, serve(absent, http.MethodGet, "/api/v1/logs/summary?range=1h", testToken, ""), http.StatusServiceUnavailable)

	failing := &fakeLogSummarySource{build: func(logobs.SummaryQuery) (logobs.Summary, error) {
		return logobs.Summary{}, errors.New("synthetic-secret /private/path")
	}}
	failure := serve(newLogSummaryHandler(t, failing, func() time.Time { return now }), http.MethodGet, "/api/v1/logs/summary?range=1h", testToken, "")
	assertLogSummaryProblem(t, failure, http.StatusServiceUnavailable)
	if strings.Contains(failure.Body.String(), "synthetic-secret") || strings.Contains(failure.Body.String(), "/private/path") {
		t.Fatal("source error crossed the Problem boundary")
	}

	invalid := &fakeLogSummarySource{build: func(query logobs.SummaryQuery) (logobs.Summary, error) {
		value := fullLogSummary(query, 1)
		value.BucketCount--
		return value, nil
	}}
	assertLogSummaryProblem(t, serve(newLogSummaryHandler(t, invalid, func() time.Time { return now }), http.MethodGet, "/api/v1/logs/summary?range=1h", testToken, ""), http.StatusInternalServerError)

	future := &fakeLogSummarySource{build: func(query logobs.SummaryQuery) (logobs.Summary, error) {
		value := fullLogSummary(query, 1)
		futureStatus := now.Add(time.Nanosecond)
		value.Sources[0].Status.ObservedAt = &futureStatus
		value.Sources[0].Status.AttemptedAt = &futureStatus
		value.Sources[0].Status.CoverageThrough = &futureStatus
		return value, nil
	}}
	assertLogSummaryProblem(t, serve(newLogSummaryHandler(t, future, func() time.Time { return now }), http.MethodGet, "/api/v1/logs/summary?range=1h", testToken, ""), http.StatusInternalServerError)

	for name, invalidClock := range map[string]time.Time{
		"zero":      {},
		"year10000": time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC),
	} {
		t.Run(name, func(t *testing.T) {
			clockSource := &fakeLogSummarySource{build: func(query logobs.SummaryQuery) (logobs.Summary, error) {
				return fullLogSummary(query, 1), nil
			}}
			response := serve(newLogSummaryHandler(t, clockSource, func() time.Time { return invalidClock }), http.MethodGet, "/api/v1/logs/summary?range=1h", testToken, "")
			assertLogSummaryProblem(t, response, http.StatusInternalServerError)
			if len(clockSource.queries) != 0 {
				t.Fatalf("invalid clock reached source %d times", len(clockSource.queries))
			}
		})
	}
}

func TestLogSummaryConfiguredSourceCanReportExplicitDisabled(t *testing.T) {
	now := time.Date(2026, 9, 9, 13, 0, 17, 0, time.UTC)
	source := &fakeLogSummarySource{build: func(query logobs.SummaryQuery) (logobs.Summary, error) {
		return fullLogSummary(query, 0), nil
	}}
	response := serve(newLogSummaryHandler(t, source, func() time.Time { return now }), http.MethodGet, "/api/v1/logs/summary?range=1h", testToken, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"support_state":"DISABLED"`) ||
		!strings.Contains(response.Body.String(), `"reason_code":"LOG_SOURCES_DISABLED"`) || !strings.Contains(response.Body.String(), `"sources":[]`) {
		t.Fatalf("disabled summary=%d %s", response.Code, response.Body.String())
	}
}

func TestLogSummaryRetriesOnceAcrossPresentationBoundary(t *testing.T) {
	times := []time.Time{
		time.Date(2026, 9, 9, 12, 59, 59, 900_000_000, time.UTC),
		time.Date(2026, 9, 9, 13, 0, 0, 100_000_000, time.UTC),
		time.Date(2026, 9, 9, 13, 0, 0, 200_000_000, time.UTC),
	}
	next := 0
	clock := func() time.Time {
		value := times[next]
		next++
		return value
	}
	source := &fakeLogSummarySource{build: func(query logobs.SummaryQuery) (logobs.Summary, error) {
		return fullLogSummary(query, 1), nil
	}}
	response := serve(newLogSummaryHandler(t, source, clock), http.MethodGet, "/api/v1/logs/summary?range=1h", testToken, "")
	if response.Code != http.StatusOK || len(source.queries) != 2 {
		t.Fatalf("response=%d source_calls=%d body=%s", response.Code, len(source.queries), response.Body.String())
	}
	if !source.queries[0].WindowEnd.Equal(time.Date(2026, 9, 9, 12, 59, 0, 0, time.UTC)) ||
		!source.queries[1].WindowEnd.Equal(time.Date(2026, 9, 9, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("queries did not move to the new fixed grid: %+v", source.queries)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["window_end"] != "2026-09-09T13:00:00Z" || body["generated_at"] != "2026-09-09T13:00:00.2Z" {
		t.Fatalf("response clock is inconsistent: %v", body)
	}
}

func TestLogSummaryRetainsLatestStatusObservedDuringQuery(t *testing.T) {
	times := []time.Time{
		time.Date(2026, 9, 9, 13, 0, 10, 0, time.UTC),
		time.Date(2026, 9, 9, 13, 0, 40, 0, time.UTC),
	}
	next := 0
	source := &fakeLogSummarySource{build: func(query logobs.SummaryQuery) (logobs.Summary, error) {
		value := fullLogSummary(query, 1)
		observed := time.Date(2026, 9, 9, 13, 0, 30, 0, time.UTC)
		value.Sources[0].Status.ObservedAt = &observed
		value.Sources[0].Status.AttemptedAt = &observed
		value.Sources[0].Status.CoverageThrough = &observed
		return value, nil
	}}
	response := serve(newLogSummaryHandler(t, source, func() time.Time {
		value := times[next]
		next++
		return value
	}), http.MethodGet, "/api/v1/logs/summary?range=1h", testToken, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"observed_at":"2026-09-09T13:00:30Z"`) {
		t.Fatalf("latest source status was dropped: %d %s", response.Code, response.Body.String())
	}
}

func TestLogSummaryBoundaryRetryIsBounded(t *testing.T) {
	times := []time.Time{
		time.Date(2026, 9, 9, 12, 59, 59, 900_000_000, time.UTC),
		time.Date(2026, 9, 9, 13, 0, 0, 100_000_000, time.UTC),
		time.Date(2026, 9, 9, 13, 1, 0, 100_000_000, time.UTC),
	}
	next := 0
	source := &fakeLogSummarySource{build: func(query logobs.SummaryQuery) (logobs.Summary, error) {
		return fullLogSummary(query, 1), nil
	}}
	response := serve(newLogSummaryHandler(t, source, func() time.Time {
		value := times[next]
		next++
		return value
	}), http.MethodGet, "/api/v1/logs/summary?range=1h", testToken, "")
	assertLogSummaryProblem(t, response, http.StatusServiceUnavailable)
	if len(source.queries) != 2 || next != len(times) {
		t.Fatalf("boundary retry was not bounded: calls=%d clock_reads=%d", len(source.queries), next)
	}
}

func TestLogSummaryMaximumGridRemainsWithinWireLimit(t *testing.T) {
	now := time.Date(2026, 9, 9, 13, 0, 17, 987_654_321, time.UTC)
	source := &fakeLogSummarySource{build: func(query logobs.SummaryQuery) (logobs.Summary, error) {
		return largePartialLogSummary(query), nil
	}}
	response := serve(newLogSummaryHandler(t, source, func() time.Time { return now }), http.MethodGet, "/api/v1/logs/summary?range=7d", testToken, "")
	if response.Code != http.StatusOK || response.Body.Len() > logSummaryResponseLimit {
		t.Fatalf("maximum response=%d bytes=%d", response.Code, response.Body.Len())
	}
	if !strings.Contains(response.Body.String(), `"reason_code":"RESPONSE_TOO_LARGE"`) ||
		!strings.Contains(response.Body.String(), `"trace":100000001`) ||
		!strings.Contains(response.Body.String(), `"generated_at":"2026-09-09T13:00:17.987654321Z"`) {
		t.Fatalf("maximum response did not retain rich bounded data: %s", response.Body.String()[:min(response.Body.Len(), 1000)])
	}
}

func newLogSummaryHandler(t *testing.T, logs LogSummarySource, now func() time.Time) http.Handler {
	t.Helper()
	handler, err := NewHandler(Config{
		Port: 9847, Token: testToken, Version: "0.1.0", Source: fakeSource{}, History: handlerReader{},
		LogSummarySource: logs, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func fullLogSummary(query logobs.SummaryQuery, sourceCount int) logobs.Summary {
	sources := make([]logobs.SourceSummary, sourceCount)
	for sourceIndex := range sources {
		sourceName := logobs.SourceSystem
		if sourceIndex == 1 {
			sourceName = logobs.SourceApplication
		}
		observed := query.WindowEnd
		buckets := make([]logobs.SummaryBucket, query.BucketCount)
		for index := range buckets {
			buckets[index] = logobs.SummaryBucket{
				At: query.WindowStart.Add(time.Duration(index) * query.BucketInterval), CoverageState: logobs.CoverageFull,
				CoveredSeconds: uint32(query.BucketInterval / time.Second), Counts: &logobs.BucketCounts{},
			}
		}
		sources[sourceIndex] = logobs.SourceSummary{
			Source: sourceName,
			Status: logobs.Status{
				SupportState: logobs.SupportSupported, CollectionState: logobs.CollectionOK, Freshness: logobs.FreshnessCurrent,
				ObservedAt: &observed, AttemptedAt: &observed, CoverageThrough: &observed,
			},
			CoverageState: logobs.CoverageFull, CoveredSeconds: uint64(query.WindowEnd.Sub(query.WindowStart) / time.Second),
			Counts: &logobs.Counts{}, Buckets: buckets,
		}
	}
	return logobs.Summary{
		WindowStart: query.WindowStart, WindowEnd: query.WindowEnd, BucketInterval: query.BucketInterval,
		BucketCount: query.BucketCount, Sources: sources,
	}
}

func largePartialLogSummary(query logobs.SummaryQuery) logobs.Summary {
	summary := fullLogSummary(query, logobs.MaxSources)
	statusReason := logobs.ReasonResponseTooLarge
	bucketReason := logobs.ReasonResponseTooLarge
	for sourceIndex := range summary.Sources {
		observed := query.WindowEnd.Add(-876_543_211 * time.Nanosecond)
		summary.Sources[sourceIndex].Status = logobs.Status{
			SupportState: logobs.SupportSupported, CollectionState: logobs.CollectionPartial, Freshness: logobs.FreshnessCurrent,
			ObservedAt: &observed, AttemptedAt: &observed, CoverageThrough: &observed, ReasonCode: &statusReason,
		}
		summary.Sources[sourceIndex].CoverageState = logobs.CoveragePartial
		summary.Sources[sourceIndex].CoveredSeconds = 0
		summary.Sources[sourceIndex].Counts = &logobs.Counts{}
		for bucketIndex := range summary.Sources[sourceIndex].Buckets {
			counts := &logobs.BucketCounts{
				Counts: logobs.Counts{Captured: 700_000_028, Discarded: 99_999_999},
				Severity: logobs.SeverityCounts{
					Trace: 100_000_001, Debug: 100_000_002, Info: 100_000_003, Warn: 100_000_004,
					Error: 100_000_005, Critical: 100_000_006, Unknown: 100_000_007,
				},
			}
			bucket := &summary.Sources[sourceIndex].Buckets[bucketIndex]
			bucket.CoverageState = logobs.CoveragePartial
			bucket.CoveredSeconds = uint32(query.BucketInterval/time.Second) - 1
			bucket.ReasonCode = &bucketReason
			bucket.Counts = counts
			summary.Sources[sourceIndex].CoveredSeconds += uint64(bucket.CoveredSeconds)
			summary.Sources[sourceIndex].Counts.Captured += counts.Captured
			summary.Sources[sourceIndex].Counts.Discarded += counts.Discarded
		}
	}
	return summary
}

func assertLogSummaryProblem(t *testing.T, response *httptest.ResponseRecorder, status int) {
	t.Helper()
	if response.Code != status || response.Header().Get("Content-Type") != "application/problem+json" || response.Body.Len() > 4096 {
		t.Fatalf("Problem=%d bytes=%d headers=%v body=%s", response.Code, response.Body.Len(), response.Header(), response.Body.String())
	}
	var problem Problem
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Status != status || problem.Code != "LOG_SUMMARY_UNAVAILABLE" || problem.Title != "Log summary unavailable" {
		t.Fatalf("unexpected Problem: %+v", problem)
	}
}

func assertLogSummaryJSONShape(t *testing.T, encoded []byte) {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	assertMapKeys(t, value, "bucket_interval_seconds", "collection_state", "counts", "coverage_state", "expected_bucket_count", "freshness", "generated_at", "limits", "observed_at", "privacy", "range", "reason_code", "schema_version", "sources", "support_state", "window_end", "window_start")
	privacy := value["privacy"].(map[string]any)
	assertMapKeys(t, privacy, "contains_event_codes", "contains_identity_fields", "contains_log_bodies", "data_classification", "remote_upload_eligible")
	if privacy["data_classification"] != "LOCAL_SENSITIVE" || privacy["contains_event_codes"] != false ||
		privacy["contains_identity_fields"] != false || privacy["contains_log_bodies"] != false || privacy["remote_upload_eligible"] != false {
		t.Fatalf("unsafe privacy projection: %v", privacy)
	}
	limits := value["limits"].(map[string]any)
	assertMapKeys(t, limits, "max_buckets_per_source", "max_response_bytes", "max_sources")
	if limits["max_sources"] != float64(2) || limits["max_buckets_per_source"] != float64(168) || limits["max_response_bytes"] != float64(logSummaryResponseLimit) {
		t.Fatalf("wrong response limits: %v", limits)
	}
	sources := value["sources"].([]any)
	source := sources[0].(map[string]any)
	assertMapKeys(t, source, "buckets", "counts", "coverage_state", "covered_seconds", "source", "status")
	assertMapKeys(t, source["status"].(map[string]any), "attempted_at", "collection_state", "coverage_through", "freshness", "observed_at", "reason_code", "support_state")
	bucket := source["buckets"].([]any)[0].(map[string]any)
	assertMapKeys(t, bucket, "at", "counts", "coverage_state", "covered_seconds", "reason_code")
	counts := bucket["counts"].(map[string]any)
	assertMapKeys(t, counts, "captured", "discarded", "severity")
	assertMapKeys(t, counts["severity"].(map[string]any), "critical", "debug", "error", "info", "trace", "unknown", "warn")
}

func assertMapKeys(t *testing.T, value map[string]any, expected ...string) {
	t.Helper()
	actual := make([]string, 0, len(value))
	for key := range value {
		actual = append(actual, key)
	}
	sort.Strings(actual)
	sort.Strings(expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("keys=%v expected=%v", actual, expected)
	}
}
