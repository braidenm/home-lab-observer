package containerobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testID1 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testID2 = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestDisabledDoesNotAccessDocker(t *testing.T) {
	collector, err := New(Config{Now: func() time.Time { t.Fatal("disabled collector called clock"); return time.Time{} }})
	if err != nil {
		t.Fatal(err)
	}
	got := collector.Collect(context.Background())
	if got.SupportState != SupportDisabled || got.CollectionState != CollectionNotRun || got.ReasonCode == nil || *got.ReasonCode != "DOCKER_NOT_CONFIGURED" || len(got.Items) != 0 {
		t.Fatalf("unexpected disabled inventory: %+v", got)
	}
}

func TestCollectNormalizesInventoryAndStats(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.FixedZone("test", 3600))
	var paths []string
	var lock sync.Mutex
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.Body != nil {
			t.Fatalf("unsafe request: %s body=%v", request.Method, request.Body)
		}
		lock.Lock()
		paths = append(paths, request.URL.RequestURI())
		lock.Unlock()
		switch request.URL.RequestURI() {
		case "/version":
			return jsonResponse(http.StatusOK, `{"ApiVersion":"1.47"}`), nil
		case "/v1.45/containers/json?all=1":
			return jsonResponse(http.StatusOK, `[{"Id":"`+testID1+`","Names":["/app token=secret-canary"],"Image":"https://user:pass@example.invalid/image user@example.com","State":"RUNNING","Env":["SECRET=leak"],"Labels":{"token":"leak"}},{"Id":"`+testID2+`","Names":["/db"],"Image":"postgres:17","State":"exited"}]`), nil
		case "/v1.45/containers/" + testID1 + "/stats?one-shot=true&stream=false":
			return jsonResponse(http.StatusOK, `{"cpu_stats":{"cpu_usage":{"total_usage":200,"percpu_usage":[1,1,1,1]},"system_cpu_usage":2000,"online_cpus":4},"precpu_stats":{"cpu_usage":{"total_usage":100},"system_cpu_usage":1000},"memory_stats":{"usage":12345},"networks":{"eth0":{"rx_bytes":999}}}`), nil
		default:
			t.Fatalf("unexpected path: %s", request.URL.RequestURI())
			return nil, nil
		}
	})}
	collector := newCollectorForTest(client, func() time.Time { return now })
	got := collector.Collect(context.Background())
	if got.SchemaVersion != SchemaVersion || got.SupportState != SupportSupported || got.CollectionState != CollectionOK || got.Freshness != FreshnessCurrent || got.ObservedAt == nil || got.ObservedAt.Location() != time.UTC {
		t.Fatalf("unexpected inventory quality: %+v", got)
	}
	if got.TotalCount != 2 || got.ReturnedCount != 2 || got.Truncated || len(got.Items) != 2 {
		t.Fatalf("unexpected counts: %+v", got)
	}
	byState := map[string]Container{got.Items[0].State: got.Items[0], got.Items[1].State: got.Items[1]}
	running := byState[StateRunning]
	if running.IDAlias == "" || strings.Contains(running.IDAlias, testID1) || running.CPUPercent == nil || *running.CPUPercent != 10 || running.MemoryBytes == nil || *running.MemoryBytes != 12345 || running.MetricsState != MetricsAvailable || running.ReasonCode != nil {
		t.Fatalf("unexpected running item: %+v", running)
	}
	stopped := byState[StateExited]
	if stopped.CPUPercent != nil || stopped.MemoryBytes != nil || stopped.MetricsState != MetricsNotRunning || stopped.ReasonCode == nil || *stopped.ReasonCode != "NOT_RUNNING" {
		t.Fatalf("unexpected stopped item: %+v", stopped)
	}
	encoded, _ := json.Marshal(got)
	for _, secret := range []string{testID1, testID2, "secret-canary", "user@example.com", "user:pass", "Env", "Labels"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("sensitive or disallowed value %q survived: %s", secret, encoded)
		}
	}
	wantPaths := []string{"/version", "/v1.45/containers/json?all=1", "/v1.45/containers/" + testID1 + "/stats?one-shot=true&stream=false"}
	for _, want := range wantPaths {
		if !contains(paths, want) {
			t.Errorf("missing request %s in %v", want, paths)
		}
	}
}

func TestCPUUsesPreviousBoundedPollWhenPreCPUIsAbsent(t *testing.T) {
	var poll atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/version":
			return jsonResponse(200, `{"ApiVersion":"1.45"}`), nil
		case "/v1.45/containers/json":
			return jsonResponse(200, `[{"Id":"`+testID1+`","Names":["/app"],"Image":"app","State":"running"}]`), nil
		default:
			cycle := poll.Add(1)
			if cycle == 1 {
				return jsonResponse(200, `{"cpu_stats":{"cpu_usage":{"total_usage":100,"percpu_usage":[1,1,1,1]},"system_cpu_usage":1000,"online_cpus":4},"precpu_stats":{"cpu_usage":{"total_usage":0},"system_cpu_usage":0},"memory_stats":{"usage":10}}`), nil
			}
			return jsonResponse(200, `{"cpu_stats":{"cpu_usage":{"total_usage":200,"percpu_usage":[1,1,1,1]},"system_cpu_usage":2000,"online_cpus":4},"memory_stats":{"usage":11}}`), nil
		}
	})}
	collector := newCollectorForTest(client, time.Now)
	first := collector.Collect(context.Background()).Items[0]
	second := collector.Collect(context.Background()).Items[0]
	if first.CPUPercent != nil || first.MemoryBytes == nil || first.MetricsState != MetricsPartial {
		t.Fatalf("first poll should expose memory and an honest CPU gap: %+v", first)
	}
	if second.CPUPercent == nil || *second.CPUPercent != 10 || second.MetricsState != MetricsAvailable {
		t.Fatalf("second poll should use the prior bounded sample: %+v", second)
	}
}

func TestCollectBoundsFanoutAndTruncatesInventory(t *testing.T) {
	containers := make([]engineContainer, MaxContainers+1)
	for index := range containers {
		containers[index] = engineContainer{ID: fmt.Sprintf("%064x", index+1), Names: []string{"/item"}, Image: "image", State: "running"}
	}
	payload, _ := json.Marshal(containers)
	var active atomic.Int32
	var peak atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/version":
			return jsonResponse(200, `{"ApiVersion":"1.45"}`), nil
		case "/v1.45/containers/json":
			return jsonResponse(200, string(payload)), nil
		default:
			current := active.Add(1)
			for current > peak.Load() && !peak.CompareAndSwap(peak.Load(), current) {
			}
			time.Sleep(time.Millisecond)
			active.Add(-1)
			return jsonResponse(200, `{"cpu_stats":{"cpu_usage":{"total_usage":2,"percpu_usage":[1]},"system_cpu_usage":2,"online_cpus":1},"precpu_stats":{"cpu_usage":{"total_usage":1},"system_cpu_usage":1},"memory_stats":{"usage":1}}`), nil
		}
	})}
	collector := newCollectorForTest(client, time.Now)
	got := collector.Collect(context.Background())
	if got.TotalCount != MaxContainers+1 || got.ReturnedCount != MaxContainers || !got.Truncated || len(got.Items) != MaxContainers {
		t.Fatalf("inventory was not bounded: total=%d returned=%d truncated=%v", got.TotalCount, got.ReturnedCount, got.Truncated)
	}
	if peak.Load() > maxStatsFanout {
		t.Fatalf("stats fanout exceeded %d: %d", maxStatsFanout, peak.Load())
	}
}

func TestFailureClassificationAndBodyBounds(t *testing.T) {
	tests := []struct {
		name       string
		response   *http.Response
		support    string
		reasonCode string
	}{
		{name: "permission", response: jsonResponse(http.StatusForbidden, `{}`), support: SupportPermissionDenied, reasonCode: "PERMISSION_DENIED"},
		{name: "redirect", response: response(http.StatusTemporaryRedirect, `{}`, "http://elsewhere.invalid"), support: SupportUnavailable, reasonCode: "REDIRECT_REJECTED"},
		{name: "malformed", response: jsonResponse(http.StatusOK, `{`), support: SupportUnavailable, reasonCode: "INVALID_RESPONSE"},
		{name: "oversized", response: jsonResponse(http.StatusOK, `{"ApiVersion":"`+strings.Repeat("x", maxVersionBody)+`"}`), support: SupportUnavailable, reasonCode: "RESPONSE_TOO_LARGE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return test.response, nil })}
			got := newCollectorForTest(client, time.Now).Collect(context.Background())
			if got.SupportState != test.support || got.CollectionState != CollectionNotRun || got.ReasonCode == nil || *got.ReasonCode != test.reasonCode || got.ObservedAt != nil || len(got.Items) != 0 {
				t.Fatalf("unexpected classified failure: %+v", got)
			}
		})
	}
}

func TestUnsupportedVersionAndImmutableCurrent(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(200, `{"ApiVersion":"1.40"}`), nil
	})}
	collector := newCollectorForTest(client, time.Now)
	got := collector.Collect(context.Background())
	if got.SupportState != SupportUnsupported || got.ReasonCode == nil || *got.ReasonCode != "API_VERSION_UNSUPPORTED" {
		t.Fatalf("unexpected unsupported version response: %+v", got)
	}
	*got.ReasonCode = "mutated"
	got.Items = append(got.Items, Container{Name: "injected"})
	current := collector.Current()
	if current.ReasonCode == nil || *current.ReasonCode != "API_VERSION_UNSUPPORTED" || len(current.Items) != 0 {
		t.Fatalf("Current returned mutable cache storage: %+v", current)
	}
}

func TestEngineMinimumVersionAboveReaderRangeIsUnsupported(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(200, `{"ApiVersion":"1.50","MinAPIVersion":"1.46"}`), nil
	})}
	got := newCollectorForTest(client, time.Now).Collect(context.Background())
	if got.SupportState != SupportUnsupported || got.ReasonCode == nil || *got.ReasonCode != "API_VERSION_UNSUPPORTED" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestPerContainerFailureRetainsSafeInventory(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/version":
			return jsonResponse(200, `{"ApiVersion":"1.45"}`), nil
		case "/v1.45/containers/json":
			return jsonResponse(200, `[{"Id":"`+testID1+`","Names":["/app"],"Image":"app","State":"running"}]`), nil
		default:
			return jsonResponse(http.StatusForbidden, `{}`), nil
		}
	})}
	got := newCollectorForTest(client, time.Now).Collect(context.Background())
	if got.SupportState != SupportSupported || got.CollectionState != CollectionPartial || got.ReasonCode == nil || *got.ReasonCode != "STATS_PARTIAL" || len(got.Items) != 1 {
		t.Fatalf("inventory should survive a stats failure: %+v", got)
	}
	item := got.Items[0]
	if item.MetricsState != MetricsUnavailable || item.CPUPercent != nil || item.MemoryBytes != nil || item.ReasonCode == nil || *item.ReasonCode != "PERMISSION_DENIED" {
		t.Fatalf("unexpected failed metrics quality: %+v", item)
	}
}

func TestCancellationAndOSPermissionAreCodeOnly(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		support string
		reason  string
	}{
		{name: "cancel", err: context.Canceled, support: SupportUnavailable, reason: "COLLECTION_CANCELED"},
		{name: "permission", err: os.ErrPermission, support: SupportPermissionDenied, reason: "PERMISSION_DENIED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, test.err })}
			got := newCollectorForTest(client, time.Now).Collect(context.Background())
			if got.SupportState != test.support || got.ReasonCode == nil || *got.ReasonCode != test.reason {
				t.Fatalf("unexpected code-only error result: %+v", got)
			}
		})
	}
}

func TestDuplicateIDsAndInvalidCountersAreRejectedOrNull(t *testing.T) {
	duplicatePayload := `[{"Id":"` + testID1 + `","State":"running"},{"Id":"` + testID1 + `","State":"running"}]`
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/version" {
			return jsonResponse(200, `{"ApiVersion":"1.45"}`), nil
		}
		return jsonResponse(200, duplicatePayload), nil
	})}
	got := newCollectorForTest(client, time.Now).Collect(context.Background())
	if got.ReasonCode == nil || *got.ReasonCode != "INVALID_RESPONSE" {
		t.Fatalf("duplicate identifiers should reject the response: %+v", got)
	}

	currentSystem, previousSystem := uint64(100), uint64(200)
	currentTotal, previousTotal := uint64(50), uint64(60)
	stats := engineStats{}
	stats.CPUStats.SystemUsage = &currentSystem
	stats.PreCPUStats.SystemUsage = &previousSystem
	stats.CPUStats.CPUUsage.TotalUsage = &currentTotal
	stats.PreCPUStats.CPUUsage.TotalUsage = &previousTotal
	stats.CPUStats.OnlineCPUs = 1
	if cpuPercent(&stats, cpuCounters{}, false) != nil {
		t.Fatal("reset counters must remain null")
	}
	stats.CPUStats.CPUUsage.TotalUsage = nil
	if cpuPercent(&stats, cpuCounters{}, false) != nil {
		t.Fatal("missing total_usage must remain null")
	}
}

func TestSanitizeMetadataRedactsCommonCanariesAndBoundsUnicode(t *testing.T) {
	input := "\x00 ssh://person:pass@host.invalid/repo person:pass@registry.invalid/image token=topsecret github_pat_ABCDEFGHIJKLMNOPQRSTUVWXYZ123456 user@example.com " + strings.Repeat("界", 200)
	got := sanitizeMetadata(input, 128, "fallback")
	if len([]rune(got)) > 128 || strings.ContainsRune(got, '\x00') {
		t.Fatalf("metadata was not normalized and bounded: %q", got)
	}
	for _, secret := range []string{"person:pass", "topsecret", "github_pat_", "user@example.com"} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret canary %q survived: %q", secret, got)
		}
	}
}

func TestParseLocalEndpointRejectsRemoteAndAmbiguousEndpoints(t *testing.T) {
	valid := map[string]localEndpoint{
		"unix:///var/run/docker.sock":    {scheme: "unix", address: "/var/run/docker.sock"},
		"npipe:////./pipe/docker_engine": {scheme: "npipe", address: `\\.\pipe\docker_engine`},
	}
	for raw, want := range valid {
		got, err := parseLocalEndpoint(raw)
		if err != nil || got != want {
			t.Errorf("parseLocalEndpoint(%q) = %+v, %v; want %+v", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", " unix:///var/run/docker.sock", "unix://server/run/docker.sock", "unix://relative", "npipe:////server/pipe/docker_engine", "npipe:////./pipe/", "tcp://127.0.0.1:2375", "ssh://host", "http://localhost", "unix:///var/run/docker.sock?x=1"} {
		if _, err := parseLocalEndpoint(raw); err == nil {
			t.Errorf("parseLocalEndpoint(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestNativeTransportCanBeConstructedWithoutAccessingDocker(t *testing.T) {
	endpoint := "unix:///tmp/nonexistent-docker.sock"
	if runtime.GOOS == "windows" {
		endpoint = "npipe:////./pipe/nonexistent-docker"
	}
	collector, err := New(Config{Endpoint: endpoint})
	if err != nil {
		t.Fatal(err)
	}
	if err := collector.Close(); err != nil {
		t.Fatal(err)
	}
}

func jsonResponse(status int, body string) *http.Response {
	return response(status, body, "")
}

func response(status int, body, location string) *http.Response {
	header := make(http.Header)
	if location != "" {
		header.Set("Location", location)
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
