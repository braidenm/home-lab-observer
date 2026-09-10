package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type httpJoinStopper func(context.Context) error

func (stop httpJoinStopper) Stop(ctx context.Context) error { return stop(ctx) }

func TestHTTPShutdownJoinsActiveHandlersBeforeStoreCleanup(t *testing.T) {
	for _, unexpected := range []bool{false, true} {
		name := "cancellation"
		if unexpected {
			name = "accept_failure"
		}
		t.Run(name, func(t *testing.T) { testHTTPShutdownJoin(t, unexpected, false) })
	}
}

func TestHTTPShutdownTimeoutWithholdsCleanupAfterNetworkClose(t *testing.T) {
	for _, unexpected := range []bool{false, true} {
		name := "cancellation"
		if unexpected {
			name = "accept_failure"
		}
		t.Run(name, func(t *testing.T) { testHTTPShutdownJoin(t, unexpected, true) })
	}
}

func testHTTPShutdownJoin(t *testing.T, unexpected, timeout bool) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal("synthetic listener failed")
	}
	defer listener.Close()
	entered, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	var storeClosed, closedUnderHandler atomic.Bool
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release // Deliberately ignores request cancellation to exercise Close != join.
		if storeClosed.Load() {
			closedUnderHandler.Store(true)
		}
		close(exited)
		w.WriteHeader(http.StatusNoContent)
	})}
	defer server.Close()
	serveStopped := make(chan error, 1)
	go func() { serveStopped <- server.Serve(listener) }()
	client := &http.Client{Timeout: 4 * time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not enter")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	collectorsStopped := make(chan struct{})
	var logStops, hostStops atomic.Int32
	stopCollection := func(joined bool) error {
		defer close(collectorsStopped)
		return stopCollectors(
			httpJoinStopper(func(ctx context.Context) error { logStops.Add(1); return nil }),
			httpJoinStopper(func(ctx context.Context) error { hostStops.Add(1); return nil }),
			func() error {
				if !joined {
					return nil
				}
				select {
				case <-exited:
				default:
					closedUnderHandler.Store(true)
				}
				storeClosed.Store(true)
				return nil
			})
	}
	drain := 2 * time.Second
	if timeout {
		drain = 30 * time.Millisecond
	}
	done := make(chan error, 1)
	go func() { done <- finishHTTP(ctx, server, serveStopped, drain, stopCollection) }()
	if unexpected {
		// Force a real permanent Accept error; active accepted connections remain open.
		if listener.Close() != nil {
			t.Fatal("listener failure injection failed")
		}
	} else {
		cancel()
	}
	if timeout {
		select {
		case err := <-done:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("missing drain deadline")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timeout path did not stop")
		}
		if storeClosed.Load() || logStops.Load() != 1 || hostStops.Load() != 1 {
			t.Fatal("timeout closed shared state or skipped collector stops")
		}
		// The handler is still live even though Close has terminated its connection.
		select {
		case <-exited:
			t.Fatal("fixture handler unexpectedly joined")
		default:
		}
		unblock()
	} else {
		select {
		case <-collectorsStopped:
			t.Fatal("collectors stopped beneath held handler")
		case <-time.After(40 * time.Millisecond):
		}
		unblock()
		select {
		case err := <-done:
			if (err != nil) != unexpected {
				t.Fatal("wrong ordinary/unexpected shutdown outcome")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("drained path did not stop")
		}
		if !storeClosed.Load() || logStops.Load() != 1 || hostStops.Load() != 1 {
			t.Fatal("joined resources not cleaned")
		}
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("handler not released")
	}
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("request goroutine not joined")
	}
	if closedUnderHandler.Load() {
		t.Fatal("shared state closed under active handler")
	}
	if timeout && storeClosed.Load() {
		t.Fatal("timeout cleanup was not withheld")
	}
}
