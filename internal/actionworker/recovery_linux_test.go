//go:build linux

package actionworker

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionipc"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
	"github.com/braidenm/home-lab-observer/internal/actionremote"
)

type recoveryFixture struct {
	deliveryFixture
	retained          actionprotocol.RetainedStatus
	view              actionprotocol.RecoveryView
	decision          actionprotocol.RecoveryDecision
	final             actionprotocol.Finalization
	operations        []string
	uncertainFinalize bool
	checkErr          error
	checkEntered      chan struct{}
}

func (f *recoveryFixture) Retained(context.Context, string) (actionprotocol.RetainedStatus, error) {
	f.operations = append(f.operations, "RETAINED")
	return f.retained, nil
}
func (f *recoveryFixture) Propose(context.Context, string, actionprotocol.RecoveryProposal) (actionprotocol.RecoveryView, error) {
	f.operations = append(f.operations, "PROPOSE")
	return f.view, nil
}
func (f *recoveryFixture) RecoveryStatus(context.Context, string) (actionprotocol.RecoveryView, error) {
	f.operations = append(f.operations, "STATUS")
	return f.view, nil
}
func (f *recoveryFixture) RecoveryCheck(ctx context.Context, _ string, _ actionprotocol.RecoveryCheck) (actionprotocol.RecoveryDecision, error) {
	f.operations = append(f.operations, "CHECK")
	if f.checkEntered != nil {
		close(f.checkEntered)
		<-ctx.Done()
	}
	return f.decision, f.checkErr
}

func TestRecoveryTimingRefusesEqualitySuspendAndPostSampleCancellation(t *testing.T) {
	origin := nativeStamp{at: time.Now(), boot: time.Second}
	stamp := func(d time.Duration) nativeStamp { return nativeStamp{at: origin.at.Add(d), boot: origin.boot + d} }
	ctx := context.Background()
	if relayInBudget(ctx, origin, stamp(85*time.Second), stamp(90*time.Second-time.Nanosecond)) != nil {
		t.Fatal("valid original reserve refused")
	}
	if relayInBudget(ctx, origin, stamp(86*time.Second), stamp(90*time.Second)) == nil {
		t.Fatal("whole-operation equality accepted")
	}
	if relayInBudget(ctx, origin, stamp(time.Second), stamp(6*time.Second)) == nil {
		t.Fatal("relay equality accepted")
	}
	suspended := stamp(2 * time.Second)
	suspended.boot += time.Second
	if relayInBudget(ctx, origin, stamp(time.Second), suspended) == nil {
		t.Fatal("suspend accepted")
	}
	canceled, cancel := context.WithCancel(ctx)
	sampled := stamp(2 * time.Second)
	cancel()
	if relayInBudget(canceled, origin, stamp(time.Second), sampled) == nil {
		t.Fatal("post-sample cancellation ignored")
	}
}

func TestRecoveryContinuationAllowsGuardedACKProcessingBeforeFinalize(t *testing.T) {
	w, f, vectors := recoveryWorkerFixture(t)
	client, broker := conversationPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { defer close(done); done <- w.converse(ctx, client) }()
	defer func() { cancel(); client.Close(); <-done }()
	if greeting, err := actionipc.Read(ctx, broker); err != nil || greeting.Kind != "BEGIN" {
		t.Fatal("owned BEGIN")
	}
	for i, op := range []string{"RETAINED", "STATUS"} {
		if exchangeRecovery(t, ctx, broker, i+1, queryFor(t, f, op, nil)).Outcome != "OK" {
			t.Fatal("retained recovery setup")
		}
	}
	check, _ := actionprotocol.DecodeRecoveryCheck(vectors["RecoveryCheck"])
	final, _ := actionprotocol.DecodeFinalizeRequest(vectors["FinalizeRequest"])
	if exchangeRecovery(t, ctx, broker, 3, queryFor(t, f, "CHECK", check)).Outcome != "OK" {
		t.Fatal("fresh recovery check")
	}
	// Actual consumed five-second old reader regression, not a widened duration
	// or injected receiver. Broker ACK work may legitimately take up to ten seconds.
	select {
	case <-time.After(5250 * time.Millisecond):
	case <-ctx.Done():
		t.Fatal("fixture lost original operation deadline")
	}
	if exchangeRecovery(t, ctx, broker, 4, queryFor(t, f, "FINALIZE", final)).Outcome != "OK" {
		t.Fatal("guarded ACK processing stranded finalization")
	}
	if err := <-done; err != nil {
		t.Fatal("valid recovery sequence failed after final reply")
	}
	if !w.executionDisabled() || strings.Join(f.operations, ",") != "RETAINED,STATUS,CHECK,FINALIZE" || f.claims != 0 {
		t.Fatal("processing wait restarted execution or widened recovery graph")
	}
}

func TestRecoveryCheckDenialOrInvalidApprovalCannotFinalize(t *testing.T) {
	for _, mode := range []string{"denied", "wrong_nonce", "wrong_prepare", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			w, f, v := recoveryWorkerFixture(t)
			check, err := actionprotocol.DecodeRecoveryCheck(v["RecoveryCheck"])
			if err != nil {
				t.Fatal("check fixture")
			}
			switch mode {
			case "denied":
				f.decision.Decision = "DENIED"
			case "wrong_nonce":
				f.decision.CheckNonce = strings.Repeat("f", 32)
			case "wrong_prepare":
				check.PrepareID = strings.Repeat("f", 32)
			case "unavailable":
				f.checkErr = errors.New("secret-error-body-canary")
			}
			ctx, broker, done := startRecoveryWorker(t, w)
			for i, op := range []string{"RETAINED", "STATUS"} {
				if exchangeRecovery(t, ctx, broker, i+1, queryFor(t, f, op, nil)).Outcome != "OK" {
					t.Fatal("source fixture")
				}
			}
			r := exchangeRecovery(t, ctx, broker, 3, queryFor(t, f, "CHECK", check))
			if r.Outcome == "OK" || string(r.Body) != "null" || <-done != nil || !w.executionDisabled() {
				t.Fatal("invalid permission became finalization")
			}
			want := "RETAINED,STATUS,CHECK"
			if mode == "wrong_prepare" {
				want = "RETAINED,STATUS"
			}
			if strings.Join(f.operations, ",") != want || f.claims != 0 {
				t.Fatal("denial caused extra remote work")
			}
		})
	}
}

func TestRecoveryCancellationJoinsRemoteAndNeverReportsAcceptance(t *testing.T) {
	w, f, v := recoveryWorkerFixture(t)
	f.checkEntered = make(chan struct{})
	f.checkErr = errors.New("private-cancellation-canary")
	c, broker := conversationPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.converse(ctx, c) }()
	if _, err := actionipc.Read(ctx, broker); err != nil {
		t.Fatal("greeting")
	}
	for i, op := range []string{"RETAINED", "STATUS"} {
		if exchangeRecovery(t, ctx, broker, i+1, queryFor(t, f, op, nil)).Outcome != "OK" {
			t.Fatal("source fixture")
		}
	}
	check, err := actionprotocol.DecodeRecoveryCheck(v["RecoveryCheck"])
	if err != nil || sendLocal(ctx, broker, "RECOVERY_QUERY", strings.Repeat("1", 32), "3", queryFor(t, f, "CHECK", check)) != nil {
		t.Fatal("check fixture")
	}
	select {
	case <-f.checkEntered:
	case <-ctx.Done():
		t.Fatal("remote check not reached")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancellation accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("remote operation not joined")
	}
	if !w.executionDisabled() || strings.Join(f.operations, ",") != "RETAINED,STATUS,CHECK" || f.claims != 0 {
		t.Fatal("cancellation resumed work")
	}
}

func TestRecoveryRefusesDirectOwnerMutationAndSixthQuery(t *testing.T) {
	for _, op := range []string{"PROPOSE", "CHECK", "FINALIZE"} {
		w, f, v := recoveryWorkerFixture(t)
		var body any
		switch op {
		case "PROPOSE":
			body, _ = actionprotocol.DecodeRecoveryProposal(v["RecoveryProposal"])
		case "CHECK":
			body, _ = actionprotocol.DecodeRecoveryCheck(v["RecoveryCheck"])
		case "FINALIZE":
			body, _ = actionprotocol.DecodeFinalizeRequest(v["FinalizeRequest"])
		}
		ctx, broker, done := startRecoveryWorker(t, w)
		if sendLocal(ctx, broker, "RECOVERY_QUERY", strings.Repeat("1", 32), "1", queryFor(t, f, op, body)) != nil {
			t.Fatal("direct mutation fixture")
		}
		if <-done == nil || len(f.operations) != 0 || !w.executionDisabled() {
			t.Fatal("direct mutation reached remote")
		}
	}
	w, f, v := recoveryWorkerFixture(t)
	f.uncertainFinalize = true
	ctx, broker, done := startRecoveryWorker(t, w)
	for i, op := range []string{"RETAINED", "STATUS"} {
		exchangeRecovery(t, ctx, broker, i+1, queryFor(t, f, op, nil))
	}
	check, _ := actionprotocol.DecodeRecoveryCheck(v["RecoveryCheck"])
	final, _ := actionprotocol.DecodeFinalizeRequest(v["FinalizeRequest"])
	exchangeRecovery(t, ctx, broker, 3, queryFor(t, f, "CHECK", check))
	exchangeRecovery(t, ctx, broker, 4, queryFor(t, f, "FINALIZE", final))
	exchangeRecovery(t, ctx, broker, 5, queryFor(t, f, "STATUS", nil))
	if <-done != nil {
		t.Fatal("bounded sequence failed")
	}
	// The worker has closed, not left a generic relay available for query6.
	if _, err := actionipc.Read(ctx, broker); err == nil || len(f.operations) != 5 || !w.executionDisabled() {
		t.Fatal("relay exceeded five queries")
	}
}
func (f *recoveryFixture) Finalize(context.Context, string, actionprotocol.FinalizeRequest) (actionprotocol.Finalization, error) {
	f.operations = append(f.operations, "FINALIZE")
	if f.uncertainFinalize {
		return actionprotocol.Finalization{}, actionremote.ErrUnavailable
	}
	return f.final, nil
}

func recoveryWorkerFixture(t *testing.T) (*conversationClient, *recoveryFixture, map[string]json.RawMessage) {
	t.Helper()
	raw, err := os.ReadFile("../actionprotocol/testdata/paired-container-execution-v1.json")
	var vectors struct {
		Objects map[string]json.RawMessage `json:"positive_codec_objects"`
	}
	if err != nil || json.Unmarshal(raw, &vectors) != nil {
		t.Fatal("recovery vectors")
	}
	v := vectors.Objects
	f := &recoveryFixture{}
	f.retained, err = actionprotocol.DecodeRetainedStatus(v["RetainedStatus"])
	if err != nil || f.retained.Request == nil {
		t.Fatal("retained source")
	}
	f.decision, err = actionprotocol.DecodeRecoveryDecision(v["RecoveryDecision"])
	if err != nil {
		t.Fatal("decision source")
	}
	// Frozen maxima use LongMAX for sizing; only this private reachable pair
	// changes to held3/released4. The fixture and decoder remain unchanged.
	f.decision.Approval.LaneRevision = "3"
	f.final, err = actionprotocol.DecodeFinalization(v["Finalization"])
	if err != nil {
		t.Fatal("final source")
	}
	f.final.ReleasedLaneRevision = "4"
	a := f.decision.Approval
	used, _ := strconv.Atoi(a.ApprovalRevision)
	f.view = actionprotocol.RecoveryView{ActionID: a.ActionID, RecoveryID: &a.RecoveryID, RemoteReceiptDigest: &a.RemoteReceiptDigest, EvidenceKind: &a.EvidenceKind, LaneRevision: "3", State: "PREPARED", CurrentApproval: &a, RemainingApprovals: 8 - used}
	body, _ := json.Marshal(f.view)
	if _, err = actionprotocol.DecodeRecoveryView(body); err != nil {
		t.Fatal("reachable view")
	}
	r := f.retained.Request
	w := &conversationClient{remote: f, binding: actionremote.Binding{ServerID: r.ServerID, OwnerUserID: r.OwnerUserID, ConnectorInstanceID: r.ConnectorInstanceID}}
	return w, f, v
}

func queryFor(t *testing.T, f *recoveryFixture, operation string, body any) actionprotocol.RecoveryQuery {
	t.Helper()
	d, err := actionprotocol.ReceiptDigest(*f.retained.Request)
	if err != nil {
		t.Fatal("digest")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal("query body")
	}
	return actionprotocol.RecoveryQuery{Operation: operation, ActionID: f.retained.ActionID, RemoteReceiptDigest: hex.EncodeToString(d[:]), Body: raw}
}

func startRecoveryWorker(t *testing.T, w *conversationClient) (context.Context, *net.UnixConn, <-chan error) {
	t.Helper()
	c, broker := conversationPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- w.converse(ctx, c) }()
	if m, err := actionipc.Read(ctx, broker); err != nil || m.Kind != "BEGIN" {
		t.Fatal("greeting")
	}
	return ctx, broker, done
}

func exchangeRecovery(t *testing.T, ctx context.Context, broker *net.UnixConn, ordinal int, q actionprotocol.RecoveryQuery) actionprotocol.RecoveryReply {
	t.Helper()
	nonce := strings.Repeat("1", 32)
	if sendLocal(ctx, broker, "RECOVERY_QUERY", nonce, strconv.Itoa(ordinal), q) != nil {
		t.Fatal("recovery query")
	}
	m, err := actionipc.Read(ctx, broker)
	if err != nil || m.Kind != "RECOVERY_REPLY" || m.Ordinal != strconv.Itoa(ordinal) || m.ConversationNonce != nonce {
		t.Fatal("recovery reply")
	}
	r, err := actionprotocol.DecodeRecoveryReply(m.Payload)
	if err != nil || r.Operation != q.Operation || r.ActionID != q.ActionID || r.RemoteReceiptDigest != q.RemoteReceiptDigest {
		t.Fatal("reply correlation")
	}
	return r
}

func TestRecoveryStatusDoesNotLatchOrClearExecutionState(t *testing.T) {
	for _, latched := range []bool{false, true} {
		w, f, _ := recoveryWorkerFixture(t)
		w.disabled = latched
		ctx, broker, done := startRecoveryWorker(t, w)
		if r := exchangeRecovery(t, ctx, broker, 1, queryFor(t, f, "STATUS", nil)); r.Outcome != "OK" {
			t.Fatal("read-only status refused")
		}
		if <-done != nil || w.executionDisabled() != latched || strings.Join(f.operations, ",") != "STATUS" || f.claims != 0 {
			t.Fatal("status altered execution")
		}
	}
}

func TestRecoveryInitRelaysOnlyRetainedThenProposal(t *testing.T) {
	w, f, v := recoveryWorkerFixture(t)
	ctx, broker, done := startRecoveryWorker(t, w)
	if r := exchangeRecovery(t, ctx, broker, 1, queryFor(t, f, "RETAINED", nil)); r.Outcome != "OK" || !w.executionDisabled() {
		t.Fatal("retained recovery did not latch")
	}
	p, err := actionprotocol.DecodeRecoveryProposal(v["RecoveryProposal"])
	if err != nil {
		t.Fatal("proposal source")
	}
	if r := exchangeRecovery(t, ctx, broker, 2, queryFor(t, f, "PROPOSE", p)); r.Outcome != "OK" {
		t.Fatal("proposal refused")
	}
	if <-done != nil || strings.Join(f.operations, ",") != "RETAINED,PROPOSE" || f.claims != 0 {
		t.Fatal("init escaped closed sequence")
	}
}

func TestRecoveryContinueAndLostFinalizeUseNoMutationRetry(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirmed", true: "lost_response"}[uncertain], func(t *testing.T) {
			w, f, v := recoveryWorkerFixture(t)
			f.uncertainFinalize = uncertain
			ctx, broker, done := startRecoveryWorker(t, w)
			for i, op := range []string{"RETAINED", "STATUS"} {
				if r := exchangeRecovery(t, ctx, broker, i+1, queryFor(t, f, op, nil)); r.Outcome != "OK" {
					t.Fatal("source read refused")
				}
			}
			check, err := actionprotocol.DecodeRecoveryCheck(v["RecoveryCheck"])
			if err != nil {
				t.Fatal("check source")
			}
			if r := exchangeRecovery(t, ctx, broker, 3, queryFor(t, f, "CHECK", check)); r.Outcome != "OK" {
				t.Fatal("fresh owner check refused")
			}
			final, err := actionprotocol.DecodeFinalizeRequest(v["FinalizeRequest"])
			if err != nil {
				t.Fatal("final request")
			}
			r := exchangeRecovery(t, ctx, broker, 4, queryFor(t, f, "FINALIZE", final))
			if uncertain {
				if r.Outcome != "UNAVAILABLE" || string(r.Body) != "null" {
					t.Fatal("uncertainty became receipt")
				}
				if r = exchangeRecovery(t, ctx, broker, 5, queryFor(t, f, "STATUS", nil)); r.Outcome != "OK" {
					t.Fatal("lookup after uncertainty")
				}
			} else if r.Outcome != "OK" {
				t.Fatal("finalization refused")
			}
			if <-done != nil || !w.executionDisabled() || f.claims != 0 {
				t.Fatal("recovery resumed execution")
			}
			want := "RETAINED,STATUS,CHECK,FINALIZE"
			if uncertain {
				want += ",STATUS"
			}
			if strings.Join(f.operations, ",") != want {
				t.Fatal("recovery retried a mutation")
			}
		})
	}
}

func TestRecoveryChangedBindingAndOutOfOrderCommandsNeverReachRemote(t *testing.T) {
	for _, mode := range []string{"action", "digest", "nonce", "ordinal", "operation", "owner"} {
		t.Run(mode, func(t *testing.T) {
			w, f, _ := recoveryWorkerFixture(t)
			initial := queryFor(t, f, "RETAINED", nil)
			if mode == "owner" {
				r := *f.retained.Request
				r.OwnerUserID = "usr_" + strings.Repeat("f", 32)
				f.retained.Request = &r
			}
			ctx, broker, done := startRecoveryWorker(t, w)
			if mode == "owner" {
				if reply := exchangeRecovery(t, ctx, broker, 1, initial); reply.Outcome != "UNAVAILABLE" || string(reply.Body) != "null" {
					t.Fatal("foreign owner exposed")
				}
				if <-done != nil || len(f.operations) != 1 || !w.executionDisabled() {
					t.Fatal("foreign source resumed")
				}
				return
			}
			if reply := exchangeRecovery(t, ctx, broker, 1, queryFor(t, f, "RETAINED", nil)); reply.Outcome != "OK" {
				t.Fatal("retained")
			}
			q := queryFor(t, f, "STATUS", nil)
			nonce, ordinal := strings.Repeat("1", 32), "2"
			switch mode {
			case "action":
				q.ActionID = "hla_" + strings.Repeat("f", 32)
			case "digest":
				q.RemoteReceiptDigest = strings.Repeat("f", 64)
			case "nonce":
				nonce = strings.Repeat("2", 32)
			case "ordinal":
				ordinal = "3"
			case "operation":
				q.Operation = "RETAINED"
			}
			if sendLocal(ctx, broker, "RECOVERY_QUERY", nonce, ordinal, q) != nil {
				t.Fatal("invalid sequence fixture")
			}
			if <-done == nil || len(f.operations) != 1 || !w.executionDisabled() {
				t.Fatal("invalid continuation reached network")
			}
		})
	}
}
