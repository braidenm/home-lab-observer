package containerobs

import (
	"context"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

type nativeListenerFactory func(*testing.T) (string, net.Listener)

func exerciseNativeTransport(t *testing.T, factory nativeListenerFactory) {
	t.Helper()
	t.Run("get round trip", func(t *testing.T) {
		endpoint, listener := factory(t)
		var unsafeMethod atomic.Bool
		server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Method != http.MethodGet || request.Host != "docker" {
				unsafeMethod.Store(true)
			}
			writer.Header().Set("Content-Type", "application/json")
			switch request.URL.RequestURI() {
			case "/version":
				_, _ = writer.Write([]byte(`{"ApiVersion":"1.45","MinAPIVersion":"1.24","Os":"linux"}`))
			case "/v1.45/containers/json?all=1":
				_, _ = writer.Write([]byte(`[{"Id":"` + testID1 + `","Names":["/native"],"Image":"native:1","State":"running"}]`))
			case "/v1.45/containers/" + testID1 + "/stats?one-shot=true&stream=false":
				_, _ = writer.Write([]byte(`{"cpu_stats":{"cpu_usage":{"total_usage":20,"percpu_usage":[1]},"system_cpu_usage":200,"online_cpus":1},"precpu_stats":{"cpu_usage":{"total_usage":10},"system_cpu_usage":100},"memory_stats":{"usage":64}}`))
			default:
				http.Error(writer, "not found", http.StatusNotFound)
			}
		})}
		go func() { _ = server.Serve(listener) }()
		t.Cleanup(func() { _ = server.Close() })

		collector, err := New(Config{Endpoint: endpoint, Now: time.Now})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = collector.Close() })
		got := collector.Collect(context.Background())
		if unsafeMethod.Load() || got.CollectionState != CollectionOK || len(got.Items) != 1 || got.Items[0].MetricsState != MetricsAvailable || got.Items[0].MemoryBytes == nil || *got.Items[0].MemoryBytes != 64 {
			t.Fatalf("native GET round trip failed: %+v", got)
		}
	})

	t.Run("redirect never followed", func(t *testing.T) {
		endpoint, listener := factory(t)
		var redirected atomic.Bool
		server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/redirected" {
				redirected.Store(true)
			}
			http.Redirect(writer, request, "http://docker/redirected", http.StatusTemporaryRedirect)
		})}
		go func() { _ = server.Serve(listener) }()
		t.Cleanup(func() { _ = server.Close() })
		collector, err := New(Config{Endpoint: endpoint})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = collector.Close() })
		got := collector.Collect(context.Background())
		if redirected.Load() || got.ReasonCode == nil || *got.ReasonCode != "REDIRECT_REJECTED" {
			t.Fatalf("redirect was not rejected: %+v redirected=%v", got, redirected.Load())
		}
	})

	t.Run("request cancellation reaches transport", func(t *testing.T) {
		endpoint, listener := factory(t)
		started := make(chan struct{})
		server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			close(started)
			<-request.Context().Done()
		})}
		go func() { _ = server.Serve(listener) }()
		t.Cleanup(func() { _ = server.Close() })
		collector, err := New(Config{Endpoint: endpoint})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = collector.Close() })
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result := make(chan Inventory, 1)
		go func() { result <- collector.Collect(ctx) }()
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("native request did not reach listener")
		}
		cancel()
		select {
		case got := <-result:
			if got.ReasonCode == nil || *got.ReasonCode != "COLLECTION_CANCELED" {
				t.Fatalf("unexpected canceled result: %+v", got)
			}
		case <-time.After(time.Second):
			t.Fatal("native request did not stop after cancellation")
		}
	})
}
