package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/braidenm/home-lab-observer/internal/history"
	"github.com/braidenm/home-lab-observer/internal/logobs"
)

const syntheticCode = "SYSTEMD_PRIORITY_3"

func probeHistory(ctx context.Context) error {
	seed, err := seedHistory(ctx, "/state/observer/history.sqlite", time.Now().UTC())
	if err != nil {
		return errProof
	}
	if err := retainedSession(ctx, seed, true); err != nil {
		return errProof
	}
	return retainedSession(ctx, seed, false)
}

// Test-only helper; there is no public seeding mode or production mutation endpoint.
func seedHistory(ctx context.Context, path string, now time.Time) (eventAt time.Time, result error) {
	if ctx.Err() != nil {
		return time.Time{}, errProof
	}
	store, err := history.Open(ctx, history.DefaultConfig(path), nil)
	if err != nil {
		return time.Time{}, errProof
	}
	defer func() {
		if store.Close() != nil {
			result = errProof
		}
	}()
	checkpoint, err := store.LoadCheckpoint(ctx, logobs.SourceSystem)
	if err != nil || checkpoint.ResetPending {
		return time.Time{}, errProof
	}
	eventAt = now.Truncate(time.Minute).Add(-2 * time.Minute)
	batch := logobs.Batch{Kind: logobs.BatchNormal, Source: logobs.SourceSystem, ExpectedRevision: checkpoint.Revision,
		QueryStartedAt: now, StartedAt: now, FinishedAt: now, SupportState: logobs.SupportSupported, CollectionState: logobs.CollectionOK,
		CaughtUp: true, ExaminedCount: 1, NextOpaque: []byte("synthetic-restart-proof-cursor"),
		Events: []logobs.Event{{ObservedAt: eventAt, Source: logobs.SourceSystem, Severity: logobs.SeverityError, EventCode: syntheticCode}}}
	if store.CommitBatch(ctx, batch) != nil {
		return time.Time{}, errProof
	}
	if !errors.Is(store.CommitBatch(ctx, batch), logobs.ErrRevisionConflict) {
		return time.Time{}, errProof
	}
	return eventAt, nil
}

func retainedSession(ctx context.Context, seed time.Time, force bool) (result error) {
	if ctx.Err() != nil {
		return errProof
	}
	started := time.Now().UTC()
	command := exec.Command("/package/observer", "serve", "--listen", "127.0.0.1:9847", "--state-dir", "/state/observer", "--log-source", "system")
	command.Env = []string{"LANG=C", "LC_ALL=C", "TZ=UTC"}
	command.WaitDelay = time.Second
	if command.Start() != nil {
		return errProof
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	joined := false
	defer func() {
		if !joined && stop(command, done) != nil {
			result = errProof
		}
	}()
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return errProof }}
	defer client.CloseIdleConnections()
	contents, err := boundedFile("/state/observer/local-api.token", 256)
	if err != nil {
		return errProof
	}
	token := strings.TrimSpace(string(contents))
	if len(token) != 43 {
		return errProof
	}
	check := func() bool {
		body, status, err := get(ctx, client, "/api/v1/logs/summary?range=1h", token, 256<<10)
		return err == nil && status == 200 && retainedSummary(body, seed, started) == nil
	}
	if !until(ctx, check) || !check() {
		return errProof
	}
	if current(ctx, client, token) != nil {
		return errProof
	}
	body, status, err := get(ctx, client, "/api/v1/snapshots/current", token, 1<<20)
	if err != nil || status != 200 || emptyCurrentRing(body) != nil {
		return errProof
	}
	if _, status, err := get(ctx, client, "/health/ready", "", 4096); err != nil || status != 200 {
		return errProof
	}
	if force {
		// No retry: kill only the child we started, and wait before another DB owner starts.
		if killAndReap(command, done) != nil {
			return errProof
		}
		joined = true
		return nil
	}
	if stop(command, done) != nil {
		joined = true
		return errProof
	}
	joined = true
	return nil
}

type retainedCounts struct {
	Captured  uint64 `json:"captured"`
	Discarded uint64 `json:"discarded"`
}
type retainedStatus struct {
	Support    string     `json:"support_state"`
	Collection string     `json:"collection_state"`
	Reason     *string    `json:"reason_code"`
	Attempted  *time.Time `json:"attempted_at"`
}

func retainedSummary(body []byte, seed, started time.Time) error {
	var summary struct {
		Schema  string          `json:"schema_version"`
		Counts  *retainedCounts `json:"counts"`
		Sources []struct {
			Source  string          `json:"source"`
			Status  retainedStatus  `json:"status"`
			Counts  *retainedCounts `json:"counts"`
			Buckets []struct {
				At     time.Time `json:"at"`
				Counts *struct {
					retainedCounts
					Severity map[string]uint64 `json:"severity"`
				} `json:"counts"`
			} `json:"buckets"`
		} `json:"sources"`
	}
	if json.Unmarshal(body, &summary) != nil || summary.Schema != "observer-log-summary/v1" || !oneCaptured(summary.Counts) || len(summary.Sources) != 1 || bytes.Contains(body, []byte(syntheticCode)) {
		return errProof
	}
	source := summary.Sources[0]
	state := source.Status
	if source.Source != "system" || !oneCaptured(source.Counts) || state.Support != "UNAVAILABLE" || state.Attempted == nil || state.Attempted.Before(started) || state.Reason == nil ||
		!((state.Collection == "FAILED" && *state.Reason == "READER_FAILED") || (state.Collection == "NOT_RUN" && *state.Reason == "LOG_HELPER_UNAVAILABLE")) {
		return errProof
	}
	var captured, discarded uint64
	found := false
	for _, bucket := range source.Buckets {
		if bucket.Counts == nil {
			continue
		}
		// Small fixed expected totals also reject overflow or unexpected duplicates.
		if bucket.Counts.Captured > 1 || bucket.Counts.Discarded != 0 {
			return errProof
		}
		if len(bucket.Counts.Severity) != 7 || bucket.Counts.Severity["error"] != bucket.Counts.Captured {
			return errProof
		}
		captured += bucket.Counts.Captured
		discarded += bucket.Counts.Discarded
		if bucket.Counts.Captured == 1 {
			if found || !bucket.At.Equal(seed) || len(bucket.Counts.Severity) != 7 || bucket.Counts.Severity["error"] != 1 {
				return errProof
			}
			found = true
		}
		for _, severity := range []string{"trace", "debug", "info", "warn", "critical", "unknown"} {
			value, ok := bucket.Counts.Severity[severity]
			if !ok || value != 0 {
				return errProof
			}
		}
	}
	if !found || captured != 1 || discarded != 0 {
		return errProof
	}
	return nil
}

func oneCaptured(counts *retainedCounts) bool {
	return counts != nil && counts.Captured == 1 && counts.Discarded == 0
}

func emptyCurrentRing(body []byte) error {
	var snapshot struct {
		Schema   string `json:"schema_version"`
		Sections struct {
			Logs struct {
				Items    *[]json.RawMessage `json:"items"`
				Total    *int               `json:"total_count"`
				Returned *int               `json:"returned_count"`
				Support  string             `json:"support_state"`
				Reason   *string            `json:"reason_code"`
			} `json:"logs"`
		} `json:"sections"`
	}
	if json.Unmarshal(body, &snapshot) != nil || snapshot.Schema != "observer-current-snapshot/v1" {
		return errProof
	}
	logs := snapshot.Sections.Logs
	if logs.Items == nil || len(*logs.Items) != 0 || logs.Total == nil || *logs.Total != 0 || logs.Returned == nil || *logs.Returned != 0 || logs.Support != "UNAVAILABLE" || logs.Reason == nil ||
		(*logs.Reason != "READER_FAILED" && *logs.Reason != "LOG_HELPER_UNAVAILABLE") {
		return errProof
	}
	return nil
}
