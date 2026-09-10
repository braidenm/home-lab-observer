package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func syntheticSummary() map[string]any {
	return map[string]any{"schema_version": "observer-log-summary/v1", "counts": nil, "sources": []any{map[string]any{
		"source": "system", "status": map[string]any{"support_state": "UNAVAILABLE", "collection_state": "FAILED", "reason_code": "READER_FAILED", "attempted_at": "2026-01-01T00:00:00Z"},
		"counts": nil, "coverage_state": "UNKNOWN", "covered_seconds": 0, "buckets": []any{map[string]any{"counts": nil, "covered_seconds": 0, "coverage_state": "UNKNOWN"}},
	}}}
}

func TestFailureProofRejectsFalsePasses(t *testing.T) {
	for _, mode := range []string{"valid", "native-unavailable", "healthy", "initial", "missing-reason", "disabled", "permission", "storage", "zero-counts", "covered", "bucket-zero", "missing-system", "duplicate-system"} {
		t.Run(mode, func(t *testing.T) {
			value := syntheticSummary()
			source := value["sources"].([]any)[0].(map[string]any)
			state := source["status"].(map[string]any)
			switch mode {
			case "native-unavailable":
				state["collection_state"] = "NOT_RUN"
				state["reason_code"] = "LOG_HELPER_UNAVAILABLE"
			case "healthy":
				state["support_state"] = "SUPPORTED"
				state["collection_state"] = "OK"
			case "initial":
				state["attempted_at"] = nil
			case "missing-reason":
				delete(state, "reason_code")
			case "disabled":
				state["support_state"] = "DISABLED"
			case "permission":
				state["reason_code"] = "PERMISSION_DENIED"
			case "storage":
				state["reason_code"] = "LOG_STORAGE_UNAVAILABLE"
			case "zero-counts":
				source["counts"] = map[string]int{"captured": 0, "discarded": 0}
			case "covered":
				source["covered_seconds"] = 1
			case "bucket-zero":
				source["buckets"].([]any)[0].(map[string]any)["counts"] = map[string]int{"captured": 0}
			case "missing-system":
				source["source"] = "application"
			case "duplicate-system":
				value["sources"] = []any{source, source}
			}
			body, _ := json.Marshal(value)
			err := failedSummary(body)
			if (err == nil) != (mode == "valid" || mode == "native-unavailable") {
				t.Fatal("wrong proof decision")
			}
		})
	}
}

func TestExtractClosedPackage(t *testing.T) {
	for _, bad := range []string{"", "../escape", "duplicate", "link", "missing"} {
		t.Run(bad, func(t *testing.T) {
			var buffer bytes.Buffer
			writer := tar.NewWriter(&buffer)
			names := []string{"observer", "observer-journal-helper", "LICENSE", "START-HERE.md", "run-observer.sh"}
			if bad == "missing" {
				names = names[:4]
			}
			for _, name := range names {
				_ = writer.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: 1, Typeflag: tar.TypeReg})
				_, _ = writer.Write([]byte("x"))
			}
			switch bad {
			case "../escape":
				_ = writer.WriteHeader(&tar.Header{Name: bad, Mode: 0644, Size: 1})
				_, _ = writer.Write([]byte("x"))
			case "duplicate":
				_ = writer.WriteHeader(&tar.Header{Name: "observer", Mode: 0755, Size: 1})
				_, _ = writer.Write([]byte("x"))
			case "link":
				_ = writer.WriteHeader(&tar.Header{Name: "other", Linkname: "observer", Typeflag: tar.TypeSymlink})
			}
			_ = writer.Close()
			directory := t.TempDir()
			err := extract(tar.NewReader(&buffer), directory)
			if (err == nil) != (bad == "") {
				t.Fatal("wrong extraction decision")
			}
			if bad == "" {
				info, _ := os.Stat(filepath.Join(directory, "observer"))
				if info == nil || info.Size() != 1 {
					t.Fatal("missing program")
				}
			}
		})
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHTTPBoundAndAuth(t *testing.T) {
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "127.0.0.1:9847" || r.Header.Get("Authorization") != "Bearer synthetic" {
			t.Fatal("wrong fixed request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("12345")), Header: make(http.Header)}, nil
	})}
	if _, _, err := get(context.Background(), client, "/api/v1/snapshots/current", "synthetic", 4); err == nil {
		t.Fatal("oversize accepted")
	}
}

func TestCurrentSnapshotProof(t *testing.T) {
	for _, body := range []string{
		`{"schema_version":"observer-current-snapshot/v1","observed_at":"2026-01-01T00:00:00Z","sections":{"overview":{}}}`,
		`{"schema_version":"observer-current-snapshot/v1","sections":{"overview":{}}}`,
		`{"schema_version":"wrong","observed_at":"2026-01-01T00:00:00Z","sections":{"overview":{}}}`,
	} {
		client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})}
		want := strings.Contains(body, `"schema_version":"observer-current-snapshot/v1","observed_at"`)
		if (current(context.Background(), client, "synthetic") == nil) != want {
			t.Fatal("wrong current snapshot decision")
		}
	}
}

func TestFileBound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic")
	if os.WriteFile(path, []byte("12345"), 0600) != nil {
		t.Fatal("fixture failed")
	}
	if _, err := boundedFile(path, 4); err == nil {
		t.Fatal("oversized file accepted")
	}
	if body, err := boundedFile(path, 5); err != nil || string(body) != "12345" {
		t.Fatal("bounded file rejected")
	}
}
