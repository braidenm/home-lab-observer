package localapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/containerobs"
)

type fakeContainerSource struct {
	inventory containerobs.Inventory
	calls     int
}

func (source *fakeContainerSource) Current() containerobs.Inventory {
	source.calls++
	return source.inventory
}

func TestContainersDisabledByDefaultAndProtected(t *testing.T) {
	now := time.Date(2026, 9, 9, 18, 0, 0, 0, time.UTC)
	handler := newTestHandlerWithContainers(t, nil, now)
	unauthorized := serve(handler, http.MethodGet, "/api/v1/containers", "", "")
	if unauthorized.Code != http.StatusUnauthorized || unauthorized.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("unauthorized=%d/%s", unauthorized.Code, unauthorized.Header().Get("Content-Type"))
	}
	response := serve(handler, http.MethodGet, "/api/v1/containers", testToken, "")
	if response.Code != http.StatusOK {
		t.Fatalf("disabled=%d %s", response.Code, response.Body.String())
	}
	var inventory containerobs.Inventory
	if err := json.Unmarshal(response.Body.Bytes(), &inventory); err != nil {
		t.Fatal(err)
	}
	if inventory.SchemaVersion != containerobs.SchemaVersion || inventory.SupportState != "DISABLED" || inventory.CollectionState != "NOT_RUN" || inventory.Freshness != "UNKNOWN" || inventory.ReasonCode == nil || *inventory.ReasonCode != "DOCKER_NOT_CONFIGURED" {
		t.Fatalf("disabled inventory=%+v", inventory)
	}
	if inventory.Items == nil || inventory.TotalCount != 0 || inventory.ReturnedCount != 0 || inventory.Truncated || !inventory.Policy.ReadOnly || inventory.Policy.DataClassification != "LOCAL_SENSITIVE" || inventory.Policy.RemoteUploadEligible {
		t.Fatalf("disabled bounds/policy=%+v", inventory)
	}
}

func TestContainersReadCacheOnceApplyLimitAndStalenessWithoutMutation(t *testing.T) {
	now := time.Date(2026, 9, 9, 18, 0, 0, 0, time.UTC)
	observedAt := now.Add(-snapshotStaleAfter - time.Second)
	cpu, memory := 0.0, uint64(2048)
	source := &fakeContainerSource{inventory: containerobs.Inventory{
		ObservedAt: &observedAt, SupportState: "SUPPORTED", CollectionState: "OK", Freshness: "CURRENT", TotalCount: 3,
		Items: []containerobs.Container{
			{IDAlias: "ctr_0000000000000001", Name: "one", Image: "example.invalid/one:1", State: "running", CPUPercent: &cpu, MemoryBytes: &memory, MetricsState: "AVAILABLE"},
			{IDAlias: "ctr_0000000000000002", Name: "two", Image: "example.invalid/two:1", State: "exited", MetricsState: "NOT_RUNNING", ReasonCode: stringPointer("NOT_RUNNING")},
			{IDAlias: "ctr_0000000000000003", Name: "three", Image: "example.invalid/three:1", State: "running", MetricsState: "UNAVAILABLE", ReasonCode: stringPointer("METRICS_UNAVAILABLE")},
		},
	}}
	handler := newTestHandlerWithContainers(t, source, now)
	response := serve(handler, http.MethodGet, "/api/v1/containers?limit=2", testToken, "")
	if response.Code != http.StatusOK {
		t.Fatalf("containers=%d %s", response.Code, response.Body.String())
	}
	var inventory containerobs.Inventory
	if err := json.Unmarshal(response.Body.Bytes(), &inventory); err != nil {
		t.Fatal(err)
	}
	if source.calls != 1 || inventory.TotalCount != 3 || inventory.ReturnedCount != 2 || len(inventory.Items) != 2 || !inventory.Truncated {
		t.Fatalf("cache/counts calls=%d inventory=%+v", source.calls, inventory)
	}
	if inventory.Freshness != "STALE" || inventory.ReasonCode == nil || *inventory.ReasonCode != "LATEST_SAMPLE_STALE" {
		t.Fatalf("freshness=%s reason=%v", inventory.Freshness, inventory.ReasonCode)
	}
	if len(source.inventory.Items) != 3 || source.inventory.Freshness != "CURRENT" || source.inventory.ReasonCode != nil || source.inventory.SchemaVersion != "" {
		t.Fatalf("cached source was mutated: %+v", source.inventory)
	}
	if !inventory.Policy.ReadOnly || inventory.Policy.DataClassification != "LOCAL_SENSITIVE" || inventory.Policy.RemoteUploadEligible {
		t.Fatalf("unsafe policy=%+v", inventory.Policy)
	}
}

func TestContainersDefaultLimitAndLegacySnapshotRemainIndependent(t *testing.T) {
	now := time.Date(2026, 9, 9, 18, 0, 0, 0, time.UTC)
	items := make([]containerobs.Container, 101)
	for index := range items {
		items[index] = containerobs.Container{IDAlias: "ctr_0000000000000001", Name: "container", Image: "example.invalid/container:1", State: "exited", MetricsState: "NOT_RUNNING", ReasonCode: stringPointer("NOT_RUNNING")}
	}
	source := &fakeContainerSource{inventory: containerobs.Inventory{ObservedAt: &now, SupportState: "SUPPORTED", CollectionState: "OK", Freshness: "CURRENT", TotalCount: 101, ReturnedCount: 101, Items: items}}
	handler := newTestHandlerWithContainers(t, source, now)
	response := serve(handler, http.MethodGet, "/api/v1/containers", testToken, "")
	var inventory containerobs.Inventory
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &inventory) != nil || inventory.ReturnedCount != 100 || !inventory.Truncated {
		t.Fatalf("default limit=%d %s", response.Code, response.Body.String())
	}
	legacy := serve(handler, http.MethodGet, "/api/v1/snapshots/current", testToken, "")
	if legacy.Code != http.StatusOK || !strings.Contains(legacy.Body.String(), `"containers":{"support_state":"UNSUPPORTED"`) {
		t.Fatalf("legacy projection changed=%d %s", legacy.Code, legacy.Body.String())
	}
}

func TestContainersRejectInvalidQueryAndBoundResponse(t *testing.T) {
	now := time.Date(2026, 9, 9, 18, 0, 0, 0, time.UTC)
	handler := newTestHandlerWithContainers(t, nil, now)
	for _, target := range []string{"/api/v1/containers?limit=0", "/api/v1/containers?limit=501", "/api/v1/containers?limit=1&raw=true", "/api/v1/containers?limit=secret-canary"} {
		response := serve(handler, http.MethodGet, target, testToken, "")
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"INVALID_QUERY"`) || strings.Contains(response.Body.String(), "secret-canary") {
			t.Fatalf("invalid query=%s response=%d/%s", target, response.Code, response.Body.String())
		}
	}
	oversized := &fakeContainerSource{inventory: containerobs.Inventory{ObservedAt: &now, SupportState: "SUPPORTED", CollectionState: "OK", Freshness: "CURRENT", TotalCount: 1, Items: []containerobs.Container{{IDAlias: "ctr_0000000000000001", Name: "container", Image: strings.Repeat("x", containerResponseLimit+1), State: "exited", MetricsState: "NOT_RUNNING", ReasonCode: stringPointer("NOT_RUNNING")}}}}
	response := serve(newTestHandlerWithContainers(t, oversized, now), http.MethodGet, "/api/v1/containers", testToken, "")
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"code":"RESPONSE_LIMIT_EXCEEDED"`) || strings.Contains(response.Body.String(), strings.Repeat("x", 32)) {
		t.Fatalf("oversized=%d/%s", response.Code, response.Body.String())
	}
}

func TestContainerProjectionCountsBeforeEnforcingRowCeiling(t *testing.T) {
	items := make([]containerobs.Container, containerobs.MaxContainers+1)
	inventory := projectContainerInventory(containerobs.Inventory{Items: items}, containerobs.MaxContainers, time.Now())
	if inventory.TotalCount != containerobs.MaxContainers+1 || inventory.ReturnedCount != containerobs.MaxContainers || !inventory.Truncated {
		t.Fatalf("projection lost known source count: total=%d returned=%d truncated=%v", inventory.TotalCount, inventory.ReturnedCount, inventory.Truncated)
	}
}

func newTestHandlerWithContainers(t *testing.T, containers ContainerSource, now time.Time) http.Handler {
	t.Helper()
	handler, err := NewHandler(Config{Port: 9847, Token: testToken, Version: "0.1.0", Source: fakeSource{current: snapshotAt(now), ok: true}, History: handlerReader{}, ContainerSource: containers, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}
