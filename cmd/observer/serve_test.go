package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/history"
)

func TestClassifyShutdownFailure(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantResult string
		wantCode   string
	}{
		{name: "deadline", err: context.DeadlineExceeded, wantResult: "TIMEOUT", wantCode: "SHUTDOWN_TIMEOUT"},
		{name: "wrapped cancellation", err: errors.Join(errors.New("stop"), context.Canceled), wantResult: "TIMEOUT", wantCode: "SHUTDOWN_TIMEOUT"},
		{name: "durable close", err: errors.New("synthetic store close failure"), wantResult: "FAILED", wantCode: "SHUTDOWN_FAILED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, code := classifyShutdownFailure(test.err)
			if string(result) != test.wantResult || code != test.wantCode {
				t.Fatalf("classifyShutdownFailure() = (%q, %q), want (%q, %q)", result, code, test.wantResult, test.wantCode)
			}
		})
	}
}

func TestHTTPInternalLogsDoNotEchoSourceDetails(t *testing.T) {
	var output strings.Builder
	writer := safeHTTPLog{logger: slog.New(slog.NewJSONHandler(&output, nil))}
	message := []byte("panic: synthetic-private-token and source payload")
	count, err := writer.Write(message)
	if err != nil || count != len(message) {
		t.Fatalf("write count=%d err=%v", count, err)
	}
	if strings.Contains(output.String(), "synthetic-private-token") || !strings.Contains(output.String(), "HTTP_INTERNAL_ERROR") {
		t.Fatal("server log did not preserve safe code-only boundary")
	}
}

func TestServeDrainsAndReleasesHistory(t *testing.T) {
	reservation, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := reservation.Addr().String()
	_ = reservation.Close()
	state := filepath.Join(t.TempDir(), "state")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serve(ctx, address, state, io.Discard, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(20 * time.Second)
	for {
		response, err := client.Get("http://" + address + "/health/live")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		select {
		case err := <-done:
			t.Fatalf("service exited before health: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("service did not become live")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(25 * time.Second):
		t.Fatal("service did not drain")
	}
	store, err := history.Open(context.Background(), history.DefaultConfig(filepath.Join(state, "history.sqlite")), nil)
	if err != nil {
		t.Fatalf("history not reusable after shutdown: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}
