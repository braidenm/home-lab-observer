package platformtransport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/containerobs/model"
	"github.com/braidenm/home-lab-observer/internal/enrollmentcoord"
	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

func richProcessesBody(t *testing.T, consent workloadprojection.Consent) []byte {
	t.Helper()
	at := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	processes := []observation.Process{{PID: 7, Name: "fixture", State: "running", CPUPercent: 123, MemoryBytes: 456, UptimeSeconds: 789}}
	snapshot := observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: at,
		Processes: observation.Section[[]observation.Process]{State: observation.Available, Data: &processes, Quality: observation.SectionQuality{Samples: 1, Total: 1}}}
	identity := remoteprojection.Identity{SourceID: transportBinding.ServerID, Version: "0.2.0-canary.1", OS: "linux"}
	host, err := remoteprojection.EncodeNative(snapshot, identity)
	if err != nil {
		t.Fatal(err)
	}
	dockerReason := "DOCKER_NOT_CONFIGURED"
	docker := model.Inventory{
		SchemaVersion:   model.SchemaVersion,
		SupportState:    model.SupportDisabled,
		CollectionState: model.CollectionNotRun,
		Freshness:       model.FreshnessUnknown,
		ReasonCode:      &dockerReason,
		Items:           []model.Container{},
	}
	workloads, err := workloadprojection.Encode(snapshot, docker, workloadprojection.Identity{SourceID: identity.SourceID, Version: identity.Version}, consent)
	if err != nil {
		t.Fatal(err)
	}
	body, err := combinedprojection.Encode(host, workloads, transportBinding.ServerID)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestRichEnrollmentRequiresExactServerApprovalEcho(t *testing.T) {
	var requestBody []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestBody, _ = io.ReadAll(request.Body)
		response.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(response, `{"server_id":%q,"connector_secret":%q,"collection_profile":%q,"collection_definition_sha256":%q}`,
			transportBinding.ServerID, transportCredential, connectedprofile.RichProcessesProfile, connectedprofile.RichProcessesDefinitionSHA256)
	}))
	defer server.Close()
	transport, err := newEnrollmentTransport(server.URL, ownedRoots(server), true)
	if err != nil {
		t.Fatal(err)
	}
	result, err := transport.Exchange(context.Background(), validEnrollmentRequestFixture())
	if err != nil || result.Outcome != enrollmentcoord.ExchangeAccepted {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	clear(result.ConnectorSecret)
	want := fmt.Sprintf(`{"enrollment_secret":%q,"connector_instance_id":%q,"collection_profile":%q,"collection_definition_sha256":%q}`,
		enrollmentGrant, transportBinding.ConnectorID, connectedprofile.RichProcessesProfile, connectedprofile.RichProcessesDefinitionSHA256)
	if string(requestBody) != want {
		t.Fatalf("rich enrollment body=%s", requestBody)
	}

	for _, responseBody := range []string{
		fmt.Sprintf(`{"server_id":%q,"connector_secret":%q}`, transportBinding.ServerID, transportCredential),
		fmt.Sprintf(`{"server_id":%q,"connector_secret":%q,"collection_profile":"LEGACY_COMPATIBLE","collection_definition_sha256":%q}`, transportBinding.ServerID, transportCredential, connectedprofile.RichProcessesDefinitionSHA256),
		fmt.Sprintf(`{"server_id":%q,"connector_secret":%q,"collection_profile":%q,"collection_definition_sha256":"%s"}`, transportBinding.ServerID, transportCredential, connectedprofile.RichProcessesProfile, bytes.Repeat([]byte{'0'}, 64)),
	} {
		transport.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewBufferString(responseBody))}, nil
		})
		result, err := transport.Exchange(context.Background(), validEnrollmentRequestFixture())
		if err != nil || result.Outcome != enrollmentcoord.ExchangeAmbiguous || len(result.ConnectorSecret) != 0 {
			t.Fatalf("legacy/wrong echo accepted: %#v %v", result, err)
		}
	}
}

func TestRichTransportValidatesCombinedProcessOnlyBeforeCredential(t *testing.T) {
	body := richProcessesBody(t, workloadprojection.Consent{Processes: true})
	provider := &fakeCredentials{credential: transportCredential}
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		got, _ := io.ReadAll(request.Body)
		if !bytes.Equal(got, body) {
			t.Error("rich body changed")
		}
		response.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(response, `{"server_id":%q,"sequence":1}`, transportBinding.ServerID)
	}))
	defer server.Close()
	transport, err := newTransport(server.URL, provider, ownedRoots(server), true)
	if err != nil {
		t.Fatal(err)
	}
	request := uploadstate.Request{Binding: transportBinding, Sequence: 1, Body: body}
	if result, err := transport.Send(context.Background(), request); err != nil || result.Outcome != uploadstate.Acknowledged || provider.calls != 1 {
		t.Fatalf("rich send=%#v err=%v calls=%d", result, err, provider.calls)
	}

	for _, invalid := range [][]byte{
		validTransportBody(t),
		richProcessesBody(t, workloadprojection.Consent{}),
		richProcessesBody(t, workloadprojection.Consent{Processes: true, Containers: true}),
		bytes.Repeat([]byte{'x'}, combinedprojection.MaxBytes+1),
	} {
		request.Body = invalid
		if _, err := transport.Send(context.Background(), request); err != ErrRequest {
			t.Fatalf("invalid rich body returned %v", err)
		}
	}
	if provider.calls != 1 {
		t.Fatal("invalid rich body reached credential provider")
	}

	numeric, err := newTransport(server.URL, provider, ownedRoots(server), false)
	if err != nil {
		t.Fatal(err)
	}
	request.Body = body
	if _, err := numeric.Send(context.Background(), request); err != ErrRequest {
		t.Fatal("combined body admitted by numeric transport")
	}
}
