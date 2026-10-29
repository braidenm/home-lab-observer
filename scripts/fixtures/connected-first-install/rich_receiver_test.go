package main

import (
	"bytes"
	"encoding/json"
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

func richRequestBody() string {
	return `{"enrollment_secret":"` + richGrant + `","connector_instance_id":"agent_` + strings.Repeat("c", 32) + `","collection_profile":"` + connectedprofile.RichProcessesProfile + `","collection_definition_sha256":"` + connectedprofile.RichProcessesDefinitionSHA256 + `"}`
}

func TestRichReceiverRequiresExactApprovalBeforeOneUseExchange(t *testing.T) {
	a := &richReceiver{save: func(richCounts) error { return nil }}
	h := a.handler()
	for _, body := range []string{strings.Replace(richRequestBody(), connectedprofile.RichProcessesProfile, "LEGACY_COMPATIBLE", 1), strings.Replace(richRequestBody(), connectedprofile.RichProcessesDefinitionSHA256, strings.Repeat("0", 64), 1), richRequestBody(), richRequestBody()} {
		r := httptest.NewRequest(http.MethodPost, "https://app.braidenmiller.com"+path, strings.NewReader(body))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if body == richRequestBody() && w.Code == http.StatusOK {
			var response map[string]string
			if json.Unmarshal(w.Body.Bytes(), &response) != nil || response["collection_profile"] != connectedprofile.RichProcessesProfile || response["collection_definition_sha256"] != connectedprofile.RichProcessesDefinitionSHA256 || response["server_id"] != richServerID {
				t.Fatal("approval echo missing")
			}
		} else if w.Code != http.StatusBadRequest {
			t.Fatal("invalid or reused enrollment accepted")
		}
	}
	if a.state.Connector == "" {
		t.Fatal("exchange never admitted")
	}
}

func TestRichReceiverAcceptsCombinedOnlyAndExactRetry(t *testing.T) {
	rows := []observation.Process{{PID: 42, Name: "hlofixture000001", State: "sleeping", CPUPercent: 0, MemoryBytes: 4096}}
	raw := observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC), Processes: observation.Section[[]observation.Process]{State: observation.Available, Data: &rows, Quality: observation.SectionQuality{Samples: 1, Total: 1}}}
	host, _ := remoteprojection.EncodeNative(raw, remoteprojection.Identity{SourceID: richServerID, Version: "0.2.0-canary.1", OS: "linux"})
	workloads, _ := workloadprojection.Encode(raw, model.Inventory{}, workloadprojection.Identity{SourceID: richServerID, Version: "0.2.0-canary.1"}, workloadprojection.Consent{Processes: true})
	body, err := combinedprojection.Encode(host, workloads, richServerID)
	if err != nil {
		t.Fatal(err)
	}
	a := &richReceiver{state: richCounts{Connector: "agent_" + strings.Repeat("c", 32)}, save: func(richCounts) error { return nil }}
	for _, tc := range []struct {
		body      []byte
		seq, auth string
		want      int
	}{
		{body, "1", "wrong", 401}, {[]byte("{}"), "1", "Connector " + richCredential, 400},
		{body, "01", "Connector " + richCredential, 400}, {body, "1", "Connector " + richCredential, 503},
		{body, "1", "Connector " + richCredential, 200},
		{bytes.ReplaceAll(body, []byte("0.2.0-canary.1"), []byte("0.2.1-canary.1")), "1", "Connector " + richCredential, 409},
	} {
		r := httptest.NewRequest(http.MethodPut, "https://app.braidenmiller.com/v1/connectors/home-lab/servers/"+richServerID+"/snapshot", bytes.NewReader(tc.body))
		r.Header.Set("Authorization", tc.auth)
		r.Header.Set("X-Connector-Sequence", tc.seq)
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("got %d want %d", w.Code, tc.want)
		}
	}
	if a.state.Duplicates != 1 || !a.state.Lost || a.state.Accepted != 1 {
		t.Fatal("exact retry proof missing")
	}
}
