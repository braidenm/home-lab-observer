package actionremote

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

func recoveryResponseVector(t *testing.T, name string) []byte {
	t.Helper()
	// The original frozen maximum uses LongMAX for sizing, which cannot be a
	// reachable held->released CAS pair. Normalize only these private test
	// responses to held3->released4; never widen the decoder or edit the fixture.
	raw := remoteVector(t, name)
	switch name {
	case "RecoveryView":
		var v actionprotocol.RecoveryView
		if json.Unmarshal(raw, &v) != nil || v.CurrentApproval == nil || v.Finalization == nil {
			t.Fatal("structural recovery view")
		}
		v.CurrentApproval.LaneRevision = "3"
		v.Finalization.ReleasedLaneRevision = "4"
		v.LaneRevision = "4"
		raw, _ = json.Marshal(v)
		if _, err := actionprotocol.DecodeRecoveryView(raw); err != nil {
			t.Fatal("reachable recovery view")
		}
	case "RecoveryDecision":
		var d actionprotocol.RecoveryDecision
		if json.Unmarshal(raw, &d) != nil {
			t.Fatal("structural decision")
		}
		d.Approval.LaneRevision = "3"
		raw, _ = json.Marshal(d)
	case "Finalization":
		var f actionprotocol.Finalization
		if json.Unmarshal(raw, &f) != nil {
			t.Fatal("structural finalization")
		}
		f.ReleasedLaneRevision = "4"
		raw, _ = json.Marshal(f)
	default:
		t.Fatal("unsupported test response")
	}
	return raw
}

func TestRecoveryMethodsRemainClosedMachineOnlyRoutes(t *testing.T) {
	b, _, _ := controlFixture(t)
	proposal, _ := actionprotocol.DecodeRecoveryProposal(remoteVector(t, "RecoveryProposal"))
	check, _ := actionprotocol.DecodeRecoveryCheck(remoteVector(t, "RecoveryCheck"))
	finalize, _ := actionprotocol.DecodeFinalizeRequest(remoteVector(t, "FinalizeRequest"))
	request, _ := actionprotocol.DecodeRequest(remoteVector(t, "RequestReceipt"))
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method == "POST" {
			io.Copy(io.Discard, io.LimitReader(r.Body, actionprotocol.MaxEnvelopeBytes+1))
		}
		w.Header().Set("Content-Type", "application/json")
		base := "/v1/connectors/home-lab/servers/" + b.ServerID + "/container-actions/" + request.ActionID + "/recovery"
		switch r.Method + " " + r.URL.Path {
		case "POST " + base + "/propose", "GET " + base:
			w.Write(recoveryResponseVector(t, "RecoveryView"))
		case "POST " + base + "/current-check":
			w.Write(recoveryResponseVector(t, "RecoveryDecision"))
		case "POST " + base + "/finalize":
			w.Write(recoveryResponseVector(t, "Finalization"))
		default:
			t.Error("worker reached owner prepare or arbitrary recovery route")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client := ownedClient(t, server, b, &credentialsFixture{})
	if _, err := client.Propose(context.Background(), request.ActionID, proposal); err != nil {
		t.Fatal("bound proposal relay", err)
	}
	if _, err := client.RecoveryStatus(context.Background(), request.ActionID); err != nil {
		t.Fatal("bound status relay", err)
	}
	if _, err := client.RecoveryCheck(context.Background(), request.ActionID, check); err != nil {
		t.Fatal("bound current recovery check", err)
	}
	if _, err := client.Finalize(context.Background(), request.ActionID, finalize); err != nil || calls.Load() != 4 {
		t.Fatal("exact ACK finalize relay", err)
	}
}

func TestRecoveryRefusesArbitraryPathAndChangedDigestBeforeIO(t *testing.T) {
	b, _, _ := controlFixture(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("invalid recovery reached network") }))
	defer server.Close()
	client := ownedClient(t, server, b, &credentialsFixture{})
	proposal, _ := actionprotocol.DecodeRecoveryProposal(remoteVector(t, "RecoveryProposal"))
	proposal.RemoteReceiptDigest = strings.Repeat("f", 64)
	if _, err := client.Propose(context.Background(), "hla_"+strings.Repeat("1", 32), proposal); err == nil {
		t.Fatal("proposal correlation substitution")
	}
	if _, err := client.RecoveryStatus(context.Background(), "../../prepare"); err == nil {
		t.Fatal("arbitrary path")
	}
}
