package main

import (
	"context"
	"encoding/json"
	"errors"
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
)

type syntheticRestartReader struct{}

func (syntheticRestartReader) Read(context.Context, logobs.ReadRequest) (logobs.Batch, error) {
	return logobs.Batch{}, errors.New("synthetic reader failure")
}

func TestRetainedHistoryAcrossRealStoreAndCollectorRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "history.sqlite")
	now := time.Now().UTC()
	// Match the original probe's previously committed failure, rather than only testing a new empty database.
	store, err := history.Open(ctx, history.DefaultConfig(path), shapeClock{now})
	if err != nil {
		t.Fatal("store failed")
	}
	reason := logobs.ReasonReaderFailed
	initial := logobs.Batch{Kind: logobs.BatchNormal, Source: logobs.SourceSystem, QueryStartedAt: now, StartedAt: now, FinishedAt: now, SupportState: logobs.SupportUnavailable, CollectionState: logobs.CollectionFailed, ReasonCode: &reason}
	if store.CommitBatch(ctx, initial) != nil || store.Close() != nil {
		t.Fatal("initial failure fixture rejected")
	}
	seed, err := seedHistory(ctx, path, now.Add(time.Second))
	if err != nil {
		t.Fatal("seeding/CAS replay proof failed")
	}
	for iteration := 0; iteration < 2; iteration++ {
		at := now.Add(time.Duration(iteration+2) * time.Second)
		store, err := history.Open(ctx, history.DefaultConfig(path), shapeClock{at})
		if err != nil {
			t.Fatal("store reopen failed")
		}
		cp, err := store.LoadCheckpoint(ctx, logobs.SourceSystem)
		if err != nil || cp.Revision != uint64(iteration+2) {
			t.Fatal("unexpected durable checkpoint revision")
		}
		collector, err := logobs.New(logobs.Config{Sources: []logobs.Source{logobs.SourceSystem}, Reader: syntheticRestartReader{}, Store: store, Clock: shapeClock{at}})
		if err != nil || collector.Start(ctx) != nil {
			t.Fatal("synthetic collector failed")
		}
		func() {
			defer store.Close()
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
				defer stopCancel()
				if collector.Stop(stopCtx) != nil {
					t.Error("collector did not join")
				}
			}()
			if !until(ctx, func() bool {
				status := collector.Current().Status
				return status.AttemptedAt != nil && status.AttemptedAt.Equal(at)
			}) {
				t.Fatal("synthetic failure not observed")
			}
			snapshot := projection.Current(observation.Snapshot{ObservedAt: at}, projection.SystemInfo{})
			handler, err := localapi.NewHandler(localapi.Config{Port: 9847, Token: "synthetic", Source: shapeSource{snapshot}, History: store, LogSource: collector, LogSummarySource: collector, Now: func() time.Time { return at }})
			if err != nil {
				t.Fatal("handler failed")
			}
			client := &http.Client{Transport: transportFunc(func(request *http.Request) (*http.Response, error) {
				request.URL.Scheme, request.URL.Host = "", ""
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, request)
				return recorder.Result(), nil
			})}
			body, status, err := get(ctx, client, "/api/v1/logs/summary?range=1h", "synthetic", 256<<10)
			if err != nil || status != 200 || retainedSummary(body, seed, at) != nil {
				t.Fatal("retained summary rejected")
			}
			if failedSummary(body) == nil {
				t.Fatal("seeded history weakened original null-count case")
			}
			assertRetainedRejectsMutations(t, body, seed, at)
			body, status, err = get(ctx, client, "/api/v1/snapshots/current", "synthetic", 1<<20)
			if err != nil || status != 200 || emptyCurrentRing(body) != nil {
				t.Fatal("new collector ring not empty")
			}
			var value map[string]any
			_ = json.Unmarshal(body, &value)
			logs := value["sections"].(map[string]any)["logs"].(map[string]any)
			for _, bad := range []any{nil, []any{map[string]any{"event_code": syntheticCode}}} {
				logs["items"] = bad
				changed, _ := json.Marshal(value)
				if emptyCurrentRing(changed) == nil {
					t.Fatal("invalid/replayed ring accepted")
				}
			}
		}()
	}
}

func assertRetainedRejectsMutations(t *testing.T, body []byte, seed, at time.Time) {
	t.Helper()
	for _, mode := range []string{"duplicate-total", "no-counts", "healthy", "stale-attempt", "duplicate-bucket", "ring-canary"} {
		var value map[string]any
		_ = json.Unmarshal(body, &value)
		source := value["sources"].([]any)[0].(map[string]any)
		status := source["status"].(map[string]any)
		switch mode {
		case "duplicate-total":
			value["counts"].(map[string]any)["captured"] = 2
		case "no-counts":
			source["counts"] = nil
		case "healthy":
			status["support_state"] = "SUPPORTED"
		case "stale-attempt":
			status["attempted_at"] = at.Add(-time.Second).Format(time.RFC3339Nano)
		case "duplicate-bucket":
			for _, item := range source["buckets"].([]any) {
				bucket := item.(map[string]any)
				if counts, ok := bucket["counts"].(map[string]any); ok && counts["captured"] == float64(1) {
					counts["captured"] = 2
					break
				}
			}
		case "ring-canary":
			value["event_code"] = syntheticCode
		}
		changed, _ := json.Marshal(value)
		if retainedSummary(changed, seed, at) == nil {
			t.Fatal("retained predicate accepted " + mode)
		}
	}
}
