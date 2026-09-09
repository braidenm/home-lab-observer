package localapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/containerobs"
	"github.com/braidenm/home-lab-observer/internal/history"
	"github.com/braidenm/home-lab-observer/internal/projection"
)

// Emits synthetic handler responses for the CI JSON Schema validator, never real host observations.
func TestSchemaScenarioContracts(t *testing.T) {
	fixture, err := os.ReadFile("../../schemas/v1/fixtures/valid/current-snapshot-native-preview.json")
	if err != nil {
		t.Fatal(err)
	}
	var current projection.CurrentSnapshot
	if err := json.Unmarshal(fixture, &current); err != nil {
		t.Fatal(err)
	}
	now := current.ObservedAt.Add(2 * time.Minute)
	for _, test := range []struct {
		path, schema string
		source       fakeSource
		code         int
	}{
		{"/api/v1/capabilities", "capabilities", fakeSource{current: current, ok: true}, 200},
		{"/api/v1/snapshots/current", "current-snapshot", fakeSource{current: current, ok: true}, 200},
		{"/api/v1/snapshots/current?section=overview", "current-snapshot", fakeSource{current: current, ok: true}, 200},
		{"/api/v1/snapshots/current?section=observer", "current-snapshot", fakeSource{current: current, ok: true, health: history.Health{State: "DEGRADED", ReasonCode: "DATABASE_CORRUPT"}}, 200},
		{"/api/v1/snapshots/current", "problem-details", fakeSource{}, 503},
		{"/api/v1/metrics/series?range=1h&metric=memory.utilization.percent", "metric-series", fakeSource{current: current, ok: true}, 200},
	} {
		handler := newTestHandler(t, test.source, handlerReader{}, now)
		response := serve(handler, http.MethodGet, test.path, testToken, "")
		if response.Code != test.code {
			t.Fatalf("%s status=%d", test.path, response.Code)
		}
		t.Logf("SCHEMA_SCENARIO %s %s", test.schema, strings.TrimSpace(response.Body.String()))
	}
	for _, support := range []string{"UNSUPPORTED", "DISABLED", "UNAVAILABLE", "PERMISSION_DENIED"} {
		reason := "COLLECTOR_UNAVAILABLE"
		source := schemaMetricSource{fakeSource: fakeSource{current: current, ok: true}, status: projection.SectionStatus{SupportState: support, ReasonCode: &reason}}
		handler := newTestHandler(t, source, handlerReader{}, now)
		response := serve(handler, http.MethodGet, "/api/v1/metrics/series?range=1h&metric=process.count", testToken, "")
		if response.Code != 200 || !strings.Contains(response.Body.String(), `"support_state":"`+support+`"`) {
			t.Fatalf("missing support state %s", support)
		}
		t.Logf("SCHEMA_SCENARIO metric-series %s", strings.TrimSpace(response.Body.String()))
	}
}

func TestContainerSchemaScenarioContract(t *testing.T) {
	fixture, err := os.ReadFile("../../schemas/v1/fixtures/valid/container-inventory-running-stopped.json")
	if err != nil {
		t.Fatal(err)
	}
	var inventory containerobs.Inventory
	if err := json.Unmarshal(fixture, &inventory); err != nil {
		t.Fatal(err)
	}
	now := *inventory.ObservedAt
	handler := newTestHandlerWithContainers(t, &fakeContainerSource{inventory: inventory}, now)
	response := serve(handler, http.MethodGet, "/api/v1/containers?limit=2", testToken, "")
	if response.Code != http.StatusOK {
		t.Fatalf("container status=%d body=%s", response.Code, response.Body.String())
	}
	t.Logf("SCHEMA_SCENARIO container-inventory %s", strings.TrimSpace(response.Body.String()))
}

type schemaMetricSource struct {
	fakeSource
	status projection.SectionStatus
}

func (source schemaMetricSource) MetricStatuses() map[history.MetricID]projection.SectionStatus {
	return map[history.MetricID]projection.SectionStatus{history.ProcessCount: source.status}
}
