//go:build linux

package actionworker

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionipc"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
	"github.com/braidenm/home-lab-observer/internal/actionremote"
	"golang.org/x/sys/unix"
)

type deliveryFixture struct {
	claim                   *actionprotocol.Claim
	claimErr                error
	claims, checks, reports int
}

func TestReportingRetainsLeaseReserveAfterExecutionHorizon(t *testing.T) {
	origin := nativeStamp{at: time.Now(), boot: time.Second}
	for _, duration := range []time.Duration{89 * time.Second, 118 * time.Second} {
		later := nativeStamp{at: origin.at.Add(duration), boot: origin.boot + duration}
		if _, err := elapsed(origin, later, executionHorizon); err == nil {
			t.Fatal("execution horizon widened")
		}
		if _, err := elapsed(origin, later, reportingHorizon); err != nil {
			t.Fatal("reporting reserve lost")
		}
	}
	deadline := nativeStamp{at: origin.at.Add(119 * time.Second), boot: origin.boot + 119*time.Second}
	if _, err := elapsed(origin, deadline, reportingHorizon); err == nil {
		t.Fatal("lease deadline equality admitted")
	}
}

func (f *deliveryFixture) RefreshSnapshot(context.Context, actionprotocol.SnapshotQuery) (actionprotocol.SnapshotReply, error) {
	return actionprotocol.SnapshotReply{Outcome: "DENIED"}, actionremote.ErrDenied
}
func (f *deliveryFixture) Claim(context.Context, string, []string) (*actionprotocol.Claim, error) {
	f.claims++
	return f.claim, f.claimErr
}
func (f *deliveryFixture) Check(_ context.Context, q actionprotocol.CheckRequest) (actionprotocol.ExecutionDecision, error) {
	f.checks++
	return actionprotocol.ExecutionDecision{Request: q.Request, ExecutionLeaseID: q.ExecutionLeaseID, Checkpoint: q.Checkpoint, CheckNonce: q.CheckNonce, CredentialID: f.claim.ClaimCredentialID, CredentialRevision: f.claim.ClaimCredentialRevision, CheckedAt: f.claim.ClaimedAt, Decision: "PERMITTED"}, nil
}
func (f *deliveryFixture) Report(_ context.Context, _ string, r actionprotocol.Report) (actionprotocol.ResultReceipt, error) {
	f.reports++
	return actionprotocol.ResultReceipt{Request: r.Request, ExecutionLeaseID: r.ExecutionLeaseID, ReportingCredentialID: f.claim.ClaimCredentialID, ReportingCredentialRevision: f.claim.ClaimCredentialRevision, ResultCode: r.ResultCode, RecordedAt: f.claim.ClaimedAt, CorrelationID: r.CorrelationID}, nil
}

func conversationPair(t *testing.T) (*net.UnixConn, *net.UnixConn) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal("socketpair")
	}
	connections := make([]*net.UnixConn, 2)
	for index, fd := range fds {
		f := os.NewFile(uintptr(fd), "worker-conversation-fixture")
		c, err := net.FileConn(f)
		f.Close()
		if err != nil {
			t.Fatal("owned socket")
		}
		connections[index] = c.(*net.UnixConn)
		t.Cleanup(func() { c.Close() })
	}
	return connections[0], connections[1]
}

func workerFixture(t *testing.T) (actionprotocol.Claim, actionprotocol.ApprovalSnapshot) {
	t.Helper()
	raw, err := os.ReadFile("../actionprotocol/testdata/paired-container-execution-v1.json")
	if err != nil {
		t.Fatal("fixture")
	}
	var vectors struct {
		Objects map[string]json.RawMessage `json:"positive_codec_objects"`
	}
	if json.Unmarshal(raw, &vectors) != nil {
		t.Fatal("vectors")
	}
	response, err := actionprotocol.DecodeClaimResponse(vectors.Objects["ClaimResponse"])
	if err != nil {
		t.Fatal("claim fixture")
	}
	claim := response.Claim
	r := claim.Request
	entry := actionprotocol.Entry{TargetID: r.TargetID, TargetKey: r.TargetKey, Operation: r.Operation, Parameters: r.Parameters, ParameterSHA256: r.ParameterSHA256, PolicyRevision: r.PolicyRevision, CapabilityRevision: r.CapabilityRevision}
	return claim, actionprotocol.ApprovalSnapshot{ServerID: r.ServerID, OwnerUserID: r.OwnerUserID, ConnectorInstanceID: r.ConnectorInstanceID, Entries: []actionprotocol.LocalEntry{actionprotocol.LocalFromEntry(entry)}}
}

func TestWorkerNoClaimVsUncertainClaim(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(map[bool]string{false: "verified204", true: "uncertain"}[uncertain], func(t *testing.T) {
			claim, approval := workerFixture(t)
			fixture := &deliveryFixture{}
			if uncertain {
				fixture.claimErr = actionremote.ErrClaimUnconfirmed
			}
			worker := &conversationClient{remote: fixture, binding: actionremote.Binding{ServerID: claim.Request.ServerID, OwnerUserID: claim.Request.OwnerUserID, ConnectorInstanceID: claim.Request.ConnectorInstanceID}}
			client, broker := conversationPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- worker.converse(ctx, client) }()
			if begin, err := actionipc.Read(ctx, broker); err != nil || begin.Kind != "BEGIN" {
				t.Fatal("greeting")
			}
			if sendLocal(ctx, broker, "APPROVAL_SNAPSHOT", "11111111111111111111111111111111", "1", approval) != nil {
				t.Fatal("approval")
			}
			message, err := actionipc.Read(ctx, broker)
			result := <-done
			if uncertain {
				if result == nil || err == nil || !worker.executionDisabled() {
					t.Fatal("uncertain claim resumed")
				}
			} else if result != nil || err != nil || message.Kind != "NO_CLAIM" || worker.executionDisabled() {
				t.Fatal("verified empty claim refused")
			}
			if fixture.claims != 1 || fixture.checks != 0 || fixture.reports != 0 {
				t.Fatal("unexpected remote work")
			}
		})
	}
}

func TestWorkerControlDenialClosesWithoutClaimOrExecutionLatch(t *testing.T) {
	claim, approval := workerFixture(t)
	fixture := &deliveryFixture{}
	worker := &conversationClient{remote: fixture, binding: actionremote.Binding{ServerID: claim.Request.ServerID, OwnerUserID: claim.Request.OwnerUserID, ConnectorInstanceID: claim.Request.ConnectorInstanceID}}
	client, broker := conversationPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- worker.converse(ctx, client) }()
	if _, err := actionipc.Read(ctx, broker); err != nil {
		t.Fatal("greeting")
	}
	nonce := "11111111111111111111111111111111"
	query := actionprotocol.SnapshotQuery{ServerID: approval.ServerID, OwnerUserID: approval.OwnerUserID, ConnectorInstanceID: approval.ConnectorInstanceID, PublicationID: nonce, Entries: []actionprotocol.Entry{approval.Entries[0].HTTP()}}
	if sendLocal(ctx, broker, "SNAPSHOT_QUERY", nonce, "1", query) != nil {
		t.Fatal("control query")
	}
	message, err := actionipc.Read(ctx, broker)
	if err != nil || message.Kind != "SNAPSHOT_REPLY" || message.ConversationNonce != nonce {
		t.Fatal("finite control reply")
	}
	reply, err := actionprotocol.DecodeSnapshotReply(message.Payload)
	if err != nil || reply.Outcome != "DENIED" || reply.Receipt != nil || <-done != nil || worker.executionDisabled() || fixture.claims != 0 || fixture.checks != 0 || fixture.reports != 0 {
		t.Fatal("control denial became execution")
	}
	if _, err := actionipc.Read(ctx, broker); err == nil {
		t.Fatal("control connection remained open")
	}
}

func TestWorkerCrossConversationChallengeLatchesWithoutRemoteCheck(t *testing.T) {
	claim, approval := workerFixture(t)
	fixture := &deliveryFixture{claim: &claim}
	worker := &conversationClient{remote: fixture, binding: actionremote.Binding{ServerID: claim.Request.ServerID, OwnerUserID: claim.Request.OwnerUserID, ConnectorInstanceID: claim.Request.ConnectorInstanceID}}
	client, broker := conversationPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- worker.converse(ctx, client) }()
	if _, err := actionipc.Read(ctx, broker); err != nil {
		t.Fatal("greeting")
	}
	if sendLocal(ctx, broker, "APPROVAL_SNAPSHOT", "11111111111111111111111111111111", "1", approval) != nil {
		t.Fatal("approval")
	}
	if _, err := actionipc.Read(ctx, broker); err != nil {
		t.Fatal("claim delivery")
	}
	query := actionprotocol.CheckRequest{Request: claim.Request, ExecutionLeaseID: claim.ExecutionLeaseID, Checkpoint: "1", CheckNonce: "33333333333333333333333333333333"}
	if sendLocal(ctx, broker, "AUTHORITY_CHALLENGE", "22222222222222222222222222222222", "3", query) != nil {
		t.Fatal("challenge fixture")
	}
	if <-done == nil || !worker.executionDisabled() || fixture.claims != 1 || fixture.checks != 0 || fixture.reports != 0 {
		t.Fatal("foreign challenge reached HTTP")
	}
}

func TestWorkerTwoFreshChecksAndOneReport(t *testing.T) {
	claim, approval := workerFixture(t)
	fixture := &deliveryFixture{claim: &claim}
	worker := &conversationClient{remote: fixture, binding: actionremote.Binding{ServerID: claim.Request.ServerID, OwnerUserID: claim.Request.OwnerUserID, ConnectorInstanceID: claim.Request.ConnectorInstanceID}}
	client, broker := conversationPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- worker.converse(ctx, client) }()
	if _, err := actionipc.Read(ctx, broker); err != nil {
		t.Fatal("greeting")
	}
	nonce := "11111111111111111111111111111111"
	if sendLocal(ctx, broker, "APPROVAL_SNAPSHOT", nonce, "1", approval) != nil {
		t.Fatal("approval")
	}
	if m, err := actionipc.Read(ctx, broker); err != nil || m.Kind != "CLAIMED_EXECUTE" {
		t.Fatal("claim delivery")
	}
	for _, q := range []struct{ ordinal, checkpoint, nonce string }{{"3", "1", "22222222222222222222222222222222"}, {"4", "2", "33333333333333333333333333333333"}} {
		request := actionprotocol.CheckRequest{Request: claim.Request, ExecutionLeaseID: claim.ExecutionLeaseID, Checkpoint: q.checkpoint, CheckNonce: q.nonce}
		if sendLocal(ctx, broker, "AUTHORITY_CHALLENGE", nonce, q.ordinal, request) != nil {
			t.Fatal("challenge")
		}
		if m, err := actionipc.Read(ctx, broker); err != nil || m.Kind != "AUTHORITY_REPLY" || m.Ordinal != q.ordinal {
			t.Fatal("reply")
		}
	}
	report := actionprotocol.Report{Request: claim.Request, ExecutionLeaseID: claim.ExecutionLeaseID, ResultCode: "RESTARTED", EvidenceKind: "RETAINED_TERMINAL", CorrelationID: claim.CorrelationID}
	if sendLocal(ctx, broker, "RESULT", nonce, "5", report) != nil {
		t.Fatal("result")
	}
	if <-done != nil || worker.executionDisabled() || fixture.claims != 1 || fixture.checks != 2 || fixture.reports != 1 {
		t.Fatal("execution bridge incomplete")
	}
}
