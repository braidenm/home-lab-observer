package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/history"
	"github.com/braidenm/home-lab-observer/internal/localapi"
	"github.com/braidenm/home-lab-observer/internal/logobs"
	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/projection"
	"github.com/braidenm/home-lab-observer/internal/scheduler"
)

type shapeSource struct{ snapshot projection.CurrentSnapshot }

func (s shapeSource) Current() (projection.CurrentSnapshot, bool) { return s.snapshot, true }
func (s shapeSource) Stats() scheduler.Stats                      { return scheduler.Stats{} }
func (s shapeSource) StoreHealth() history.Health                 { return history.Health{} }

type shapeSummary struct{ store *history.Store }

func (s shapeSummary) Summary(ctx context.Context, q logobs.SummaryQuery) (logobs.Summary, error) {
	return s.store.QuerySummary(ctx, []logobs.Source{logobs.SourceSystem}, q)
}

type shapeClock struct{ now time.Time }

func (c shapeClock) Now() time.Time { return c.now }

// Exercise real SQLite summary and HTTP projection, not a hand-invented JSON response.
func TestActualRuntimeJSONShapes(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 10, 1, 0, 17, 0, time.UTC)
	for _, code := range []logobs.ReasonCode{logobs.ReasonReaderFailed, logobs.ReasonLogHelperUnavailable} {
		t.Run(string(code), func(t *testing.T) {
			store, err := history.Open(ctx, history.DefaultConfig(filepath.Join(t.TempDir(), "state.db")), shapeClock{now})
			if err != nil {
				t.Fatal("synthetic store failed")
			}
			defer store.Close()
			state := logobs.CollectionFailed
			if code == logobs.ReasonLogHelperUnavailable {
				state = logobs.CollectionNotRun
			}
			batch := logobs.Batch{Kind: logobs.BatchNormal, Source: logobs.SourceSystem, QueryStartedAt: now, StartedAt: now, FinishedAt: now, SupportState: logobs.SupportUnavailable, CollectionState: state, ReasonCode: &code}
			if store.CommitBatch(ctx, batch) != nil {
				t.Fatal("failure batch rejected")
			}
			snapshot := projection.Current(observation.Snapshot{ObservedAt: now}, projection.SystemInfo{})
			handler, err := localapi.NewHandler(localapi.Config{Port: 9847, Token: "synthetic", Source: shapeSource{snapshot}, History: store, LogSummarySource: shapeSummary{store}, Now: func() time.Time { return now }})
			if err != nil {
				t.Fatal("handler failed")
			}
			client := &http.Client{Transport: transportFunc(func(request *http.Request) (*http.Response, error) {
				request.RemoteAddr = "127.0.0.1:34567"
				request.URL.Scheme, request.URL.Host = "", ""
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, request)
				return recorder.Result(), nil
			})}
			body, status, err := get(ctx, client, "/api/v1/logs/summary?range=1h", "synthetic", 256<<10)
			if err != nil || status != 200 || failedSummary(body) != nil {
				t.Fatal("actual runtime unavailable JSON rejected")
			}
			if current(ctx, client, "synthetic") != nil {
				t.Fatal("actual current snapshot JSON rejected")
			}
		})
	}
}
