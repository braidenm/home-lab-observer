package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/history"
	"github.com/braidenm/home-lab-observer/internal/logobs"
)

type stopFunction func(context.Context) error

func (f stopFunction) Stop(ctx context.Context) error { return f(ctx) }

func TestSharedHistoryClosesOnlyAfterCollectorsJoin(t *testing.T) {
	want := errors.New("synthetic stop failure")
	for _, failing := range []string{"", "logs", "host", "close"} {
		var order []string
		stop := func(name string, maximum time.Duration) stopFunction {
			return func(ctx context.Context) error {
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > maximum {
					t.Fatal("shutdown unbounded")
				}
				order = append(order, name)
				if failing == name {
					return want
				}
				return nil
			}
		}
		err := stopCollectors(stop("logs", 5*time.Second), stop("host", 15*time.Second), func() error {
			order = append(order, "close")
			if failing == "close" {
				return want
			}
			return nil
		})
		expected := []string{"logs", "host"}
		if failing == "" || failing == "close" {
			expected = append(expected, "close")
		}
		if !reflect.DeepEqual(order, expected) || (failing == "" && err != nil) || (failing != "" && !errors.Is(err, want)) {
			t.Fatal("unsafe shutdown ordering or lost failure")
		}
	}
}

type inactiveLogRuntime struct{ started, stopped int }

func (r *inactiveLogRuntime) Start(context.Context) error { r.started++; return nil }
func (r *inactiveLogRuntime) Stop(context.Context) error  { r.stopped++; return nil }
func (*inactiveLogRuntime) Current() logobs.Snapshot      { return logobs.Snapshot{} }
func (*inactiveLogRuntime) Summary(context.Context, logobs.SummaryQuery) (logobs.Summary, error) {
	return logobs.Summary{}, errors.New("not queried")
}

func TestDisabledLogRuntimeDoesNotResolveOrStartReader(t *testing.T) {
	reservation, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := reservation.Addr().String()
	_ = reservation.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logs := &inactiveLogRuntime{}
	state := filepath.Join(canonicalTestTempDir(t), "state")
	err = serveRuntime(ctx, address, state, io.Discard, slog.New(slog.NewTextHandler(io.Discard, nil)), serveRuntimeOptions{
		newLogs: func(config logobs.Config) (logRuntime, error) {
			if config.Reader != nil || len(config.Sources) != 0 || config.Store == nil {
				t.Fatal("disabled configuration touched reader")
			}
			cancel()
			return logs, nil
		},
	})
	if err != nil || logs.started != 0 || logs.stopped != 1 {
		t.Fatal("disabled collector started or shutdown failed")
	}
	store, err := history.Open(context.Background(), history.DefaultConfig(filepath.Join(state, "history.sqlite")), nil)
	if err != nil {
		t.Fatal("shared store not released")
	}
	_ = store.Close()
}

func TestServeRejectsBadSourcesBeforeOpeningState(t *testing.T) {
	state := filepath.Join(t.TempDir(), "never-created")
	for _, values := range [][]string{{"--log-source", "unknown"}, {"--log-source", "system", "--log-source", "system"}} {
		args := append([]string{"--state-dir", state}, values...)
		if code := runServe(args, io.Discard, io.Discard); code != 2 {
			t.Fatal("bad source accepted")
		}
	}
}
