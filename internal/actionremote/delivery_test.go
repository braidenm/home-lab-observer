package actionremote

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

func remoteVector(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("../actionprotocol/testdata/paired-container-execution-v1.json")
	var fixture struct {
		Objects map[string]json.RawMessage `json:"positive_codec_objects"`
	}
	if err != nil || json.Unmarshal(raw, &fixture) != nil || fixture.Objects[name] == nil {
		t.Fatal("public paired vector")
	}
	return fixture.Objects[name]
}

func TestDeliveryMethodsUseClosedRoutesAndFullBinding(t *testing.T) {
	b, q, _ := controlFixture(t)
	var calls atomic.Int32
	claim, _ := actionprotocol.DecodeClaimResponse(remoteVector(t, "ClaimResponse"))
	check, _ := actionprotocol.DecodeCheckRequest(remoteVector(t, "CheckRequest"))
	report, _ := actionprotocol.DecodeReport(remoteVector(t, "Report"))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method == "POST" {
			body, err := io.ReadAll(io.LimitReader(r.Body, actionprotocol.MaxEnvelopeBytes+1))
			if err != nil || len(body) > actionprotocol.MaxEnvelopeBytes {
				t.Error("bounded request body")
				w.WriteHeader(400)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		base := "/v1/connectors/home-lab/servers/" + b.ServerID + "/container-actions"
		var name string
		switch r.Method + " " + r.URL.Path {
		case "POST " + base + "/claim":
			name = "ClaimResponse"
		case "POST " + base + "/current-check":
			name = "ExecutionCheckDecision"
		case "POST " + base + "/" + report.Request.ActionID + "/report-result":
			name = "ResultReceipt"
		case "GET " + base + "/" + report.Request.ActionID + "/retained-status":
			name = "RetainedStatus"
		default:
			t.Error("non-closed delivery route")
			w.WriteHeader(404)
			return
		}
		w.Write(remoteVector(t, name))
	}))
	defer server.Close()
	client := ownedClient(t, server, b, &credentialsFixture{})
	if result, err := client.Claim(context.Background(), claim.ClaimNonce, []string{q.Entries[0].TargetID}); err != nil || result == nil || *result != claim.Claim {
		t.Fatal("correlated new claim")
	}
	if _, err := client.Check(context.Background(), check); err != nil {
		t.Fatal("fresh exact checkpoint", err, calls.Load())
	}
	if _, err := client.Report(context.Background(), report.Request.ActionID, report); err != nil {
		t.Fatal("exact retained terminal report")
	}
	if _, err := client.Retained(context.Background(), report.Request.ActionID); err != nil || calls.Load() != 4 {
		t.Fatal("retained source read")
	}
}

func TestClaimEmptyAndLostReplyHaveDifferentOutcomes(t *testing.T) {
	for _, empty := range []bool{true, false} {
		t.Run(map[bool]string{true: "verified204", false: "lost-claimed-response"}[empty], func(t *testing.T) {
			b, q, _ := controlFixture(t)
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if empty {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error("owned hijack")
					return
				}
				conn.Close()
			}))
			defer server.Close()
			client := ownedClient(t, server, b, &credentialsFixture{})
			claim, err := client.Claim(context.Background(), q.PublicationID, []string{q.Entries[0].TargetID})
			if claim != nil || calls.Load() != 1 || empty && err != nil || !empty && !errors.Is(err, ErrClaimUnconfirmed) {
				t.Fatal("empty poll confused with possibly consumed claim")
			}
		})
	}
}

func TestInvalidDeliveryBindingAndEmptyEligibilityPerformNoIO(t *testing.T) {
	b, q, _ := controlFixture(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("invalid input reached network") }))
	defer server.Close()
	client := ownedClient(t, server, b, &credentialsFixture{})
	if _, err := client.Claim(context.Background(), q.PublicationID, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("empty eligible claim")
	}
	check, _ := actionprotocol.DecodeCheckRequest(remoteVector(t, "CheckRequest"))
	check.Request.OwnerUserID = "usr_" + strings.Repeat("f", 32)
	check.Request.ActorUserID = check.Request.OwnerUserID
	if _, err := client.Check(context.Background(), check); !errors.Is(err, ErrInvalid) {
		t.Fatal("cross-owner checkpoint")
	}
	if _, err := client.Retained(context.Background(), "../../elsewhere"); !errors.Is(err, ErrInvalid) {
		t.Fatal("arbitrary resource path")
	}
}
