package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/containerobs/model"
	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

func dockerExchangeBody() string {
	return `{"enrollment_secret":"` + dockerGrant + `","connector_instance_id":"agent_` + strings.Repeat("e", 32) + `","collection_profile":"` + connectedprofile.RichProcessesDockerProfile + `","collection_definition_sha256":"` + connectedprofile.RichProcessesDockerDefinitionSHA256 + `"}`
}

func TestDockerFixtureExchangeApprovesOnlyExactIndependentKindAndDefinition(t *testing.T) {
	a := &dockerReceiver{save: func(s dockerCounts) error {
		if !validDockerCounts(s) {
			t.Fatal("invalid enrollment state")
		}
		return nil
	}}
	valid := dockerExchangeBody()
	for _, body := range []string{
		strings.Replace(valid, connectedprofile.RichProcessesDockerProfile, connectedprofile.RichProcessesProfile, 1),
		strings.Replace(valid, connectedprofile.RichProcessesDockerDefinitionSHA256, strings.Repeat("0", 64), 1),
		strings.Replace(valid, dockerGrant, richGrant, 1), valid, valid,
	} {
		before := a.state
		r := httptest.NewRequest(http.MethodPost, "https://"+connectedprofile.Hostname+path, strings.NewReader(body))
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, r)
		if body == valid && before.Connector == "" {
			if w.Code != 200 {
				t.Fatal("exact approval refused")
			}
			var v map[string]string
			if json.Unmarshal(w.Body.Bytes(), &v) != nil || v["collection_profile"] != connectedprofile.RichProcessesDockerProfile || v["collection_definition_sha256"] != connectedprofile.RichProcessesDockerDefinitionSHA256 || v["server_id"] != dockerServerID {
				t.Fatal("independent echo lost")
			}
		} else if w.Code != 400 || a.state != before {
			t.Fatal("wrong/reused enrollment changed state")
		}
	}
}

func dockerSnapshot(t *testing.T, unavailable, containersEnabled bool) []byte {
	t.Helper()
	rows := []observation.Process{{PID: 42, Name: "hlofixture000001", State: "sleeping", MemoryBytes: 4096}}
	at := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	raw := observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: at, Processes: observation.Section[[]observation.Process]{State: observation.Available, Data: &rows, Quality: observation.SectionQuality{Samples: 1, Total: 1}}}
	zero, memory := float64(0), uint64(4096)
	engine := model.Inventory{SchemaVersion: model.SchemaVersion, ObservedAt: &at, SupportState: model.SupportSupported, CollectionState: model.CollectionOK, Freshness: model.FreshnessCurrent, ReturnedCount: 2, TotalCount: 2, Items: []model.Container{
		{IDAlias: "ctr_0000000000000001", Name: "hlo-fixture-running", Image: "fixture:local", State: model.StateRunning, CPUPercent: &zero, MemoryBytes: &memory, MetricsState: model.MetricsAvailable},
		{IDAlias: "ctr_0000000000000002", Name: "hlo-fixture-exited", Image: "fixture:local", State: model.StateExited, MetricsState: model.MetricsNotRunning},
	}}
	if unavailable {
		engine = model.Inventory{SchemaVersion: model.SchemaVersion, SupportState: model.SupportUnavailable}
	}
	host, err := remoteprojection.EncodeNative(raw, remoteprojection.Identity{SourceID: dockerServerID, Version: "0.1.0-canary.3", OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	workloads, err := workloadprojection.Encode(raw, engine, workloadprojection.Identity{SourceID: dockerServerID, Version: "0.1.0-canary.3"}, workloadprojection.Consent{Processes: true, Containers: containersEnabled})
	if err != nil {
		t.Fatal(err)
	}
	body, err := combinedprojection.Encode(host, workloads, dockerServerID)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func dockerPut(a *dockerReceiver, body []byte, sequence string) int {
	r := httptest.NewRequest(http.MethodPut, "https://"+connectedprofile.Hostname+"/v1/connectors/home-lab/servers/"+dockerServerID+"/snapshot", bytes.NewReader(body))
	r.Header.Set("Authorization", "Connector "+dockerCredential)
	r.Header.Set("X-Connector-Sequence", sequence)
	w := httptest.NewRecorder()
	a.handler().ServeHTTP(w, r)
	return w.Code
}

func TestDockerFixtureObservesRunningExitedNullableWarmAndExactRetry(t *testing.T) {
	var persisted dockerCounts
	a := &dockerReceiver{state: dockerCounts{Connector: "agent_" + strings.Repeat("e", 32)}, save: func(s dockerCounts) error { persisted = s; return nil }}
	body := dockerSnapshot(t, false, true)
	if dockerPut(a, body, "1") != 503 || dockerPut(a, body, "1") != 200 {
		t.Fatal("lost ACK/exact retry not exercised")
	}
	if !persisted.Foreign || !persisted.Running || !persisted.ExitedNull || !persisted.Warmed || persisted.Accepted != 1 || persisted.Duplicates != 1 {
		t.Fatal("useful data proof missing")
	}
	before := a.state
	for _, tc := range []struct {
		b    []byte
		seq  string
		want int
	}{
		{dockerSnapshot(t, false, false), "2", 400}, {[]byte("{}"), "2", 400}, {body, "01", 400},
		{bytes.ReplaceAll(body, []byte("0.1.0-canary.3"), []byte("0.1.1-canary.3")), "1", 409},
		{bytes.ReplaceAll(body, []byte("fixture:local"), []byte("hlo-docker-private-canary")), "2", 400},
	} {
		if dockerPut(a, tc.b, tc.seq) != tc.want || a.state != before {
			t.Fatal("invalid upload changed receipt")
		}
	}
	missing := dockerSnapshot(t, true, true)
	if dockerPut(a, missing, "2") != 200 || !a.state.Unavailable || a.state.UnavailableCount != 1 {
		t.Fatal("unavailability became empty health")
	}
	if dockerPut(a, missing, "2") != 200 || a.state.UnavailableCount != 1 {
		t.Fatal("duplicate created new fault proof")
	}
	if dockerPut(a, missing, "3") != 200 || a.state.UnavailableCount != 2 {
		t.Fatal("fresh unavailable sequence not distinguished")
	}
	if dockerPut(a, body, "2") != 409 {
		t.Fatal("old sequence accepted")
	}
}

func TestDockerFixturePersistFailureNeverAcknowledgesOrChangesState(t *testing.T) {
	a := &dockerReceiver{state: dockerCounts{Connector: "agent_" + strings.Repeat("e", 32)}, save: func(dockerCounts) error { return errors.New("syntheticprivate") }}
	before := a.state
	if dockerPut(a, dockerSnapshot(t, false, true), "1") != 500 || a.state != before {
		t.Fatal("uncertain persistence acknowledged")
	}
	a.save = nil
	if dockerPut(a, dockerSnapshot(t, false, true), "1") != 500 || a.state != before {
		t.Fatal("missing persistence acknowledged")
	}
}

func TestDockerFixtureStateIsClosedBoundedMetadataWithoutBodies(t *testing.T) {
	valid := dockerCounts{Connector: "agent_" + strings.Repeat("e", 32), Sequence: 1, Digest: strings.Repeat("a", 64), Accepted: 1, Lost: true}
	if !validDockerCounts(valid) {
		t.Fatal("valid state refused")
	}
	for _, mutate := range []func(*dockerCounts){
		func(s *dockerCounts) { s.Digest = "private" }, func(s *dockerCounts) { s.Sequence = 0 }, func(s *dockerCounts) { s.Accepted = 0 },
		func(s *dockerCounts) { s.Accepted = 10001 }, func(s *dockerCounts) { s.Duplicates = 10001 }, func(s *dockerCounts) { s.UnavailableCount = 2 },
		func(s *dockerCounts) { s.Unavailable = true }, func(s *dockerCounts) { s.Warmed = true }, func(s *dockerCounts) { s.Connector = "bad" },
	} {
		bad := valid
		mutate(&bad)
		if validDockerCounts(bad) {
			t.Fatal("invalid durable state accepted")
		}
	}
	b, err := json.Marshal(valid)
	if err != nil || len(b) > 1024 || bytes.Contains(b, []byte(dockerGrant)) || bytes.Contains(b, []byte(dockerCredential)) {
		t.Fatal("unbounded or secret durable state")
	}
	var keys map[string]json.RawMessage
	if json.Unmarshal(b, &keys) != nil || len(keys) != 13 {
		t.Fatal("durable shape changed")
	}
	for _, key := range []string{"snapshot", "body", "name", "image", "items", "token", "credential"} {
		if keys[key] != nil {
			t.Fatal("raw data persisted")
		}
	}
}
