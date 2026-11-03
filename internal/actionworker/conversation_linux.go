//go:build linux

package actionworker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionipc"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
	"github.com/braidenm/home-lab-observer/internal/actionremote"
	"golang.org/x/sys/unix"
)

// The narrow consumer port is private. Production composition supplies the
// fixed-origin Client, never a caller-selected HTTP transport or endpoint.
type actionDelivery interface {
	RefreshSnapshot(context.Context, actionprotocol.SnapshotQuery) (actionprotocol.SnapshotReply, error)
	Claim(context.Context, string, []string) (*actionprotocol.Claim, error)
	Check(context.Context, actionprotocol.CheckRequest) (actionprotocol.ExecutionDecision, error)
	Report(context.Context, string, actionprotocol.Report) (actionprotocol.ResultReceipt, error)
}

type conversationClient struct {
	remote   actionDelivery
	binding  actionremote.Binding
	mu       sync.Mutex
	disabled bool
}

// Initial framing is not an uncertain action/recovery result. Even a worker
// already latched recovery-only must terminate rather than retry malformed input.
var errInitialSelection = errors.Join(ErrUnsafe, errors.New("invalid initial broker selection"))

func (w *conversationClient) executionDisabled() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.disabled
}
func (w *conversationClient) disableExecution() { w.mu.Lock(); w.disabled = true; w.mu.Unlock() }

type nativeStamp struct {
	at   time.Time
	boot time.Duration
}

const executionHorizon = 89 * time.Second
const reportingHorizon = 119 * time.Second

func captureStamp() (nativeStamp, error) {
	at := time.Now()
	var boot unix.Timespec
	if unix.ClockGettime(unix.CLOCK_BOOTTIME, &boot) != nil || boot.Sec < 0 || boot.Nsec < 0 || boot.Nsec >= int64(time.Second) || boot.Sec > int64((1<<63-1)/time.Second)-134 {
		return nativeStamp{}, ErrUnsafe
	}
	return nativeStamp{at: at, boot: time.Duration(boot.Sec)*time.Second + time.Duration(boot.Nsec)}, nil
}

func elapsed(from, to nativeStamp, limit time.Duration) (time.Duration, error) {
	boot := to.boot - from.boot
	mono := to.at.Sub(from.at)
	wall := to.at.UTC().Sub(from.at.UTC())
	if from.at.IsZero() || to.at.IsZero() || boot < 0 || boot >= limit || mono < 0 || wall < 0 || mono < boot-100*time.Millisecond || mono > boot+100*time.Millisecond || wall < boot-100*time.Millisecond || wall > boot+100*time.Millisecond {
		return 0, ErrUnsafe
	}
	return boot, nil
}

func sendLocal(ctx context.Context, c *net.UnixConn, kind, nonce, ordinal string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return ErrUnsafe
	}
	return actionipc.Write(ctx, c, actionprotocol.Envelope{Version: actionprotocol.WorkerVersion, Kind: kind, ConversationNonce: nonce, Ordinal: ordinal, Payload: raw})
}

func (w *conversationClient) bound(server, owner, instance string) bool {
	return w.binding.ServerID == server && w.binding.OwnerUserID == owner && w.binding.ConnectorInstanceID == instance
}

// converse consumes ONE already peer/path-authenticated connection and closes
// it in every branch. It is not a listener or peer-authentication substitute.
// The broker retains independent original timing, policy and commit guards.
func (w *conversationClient) converse(parent context.Context, c *net.UnixConn) (result error) {
	if c == nil {
		return ErrUnsafe
	}
	defer c.Close()
	if w == nil || w.remote == nil || parent == nil || parent.Err() != nil {
		return ErrUnsafe
	}
	// Execution consumes at most89 seconds; keep the conservative lease's
	// remaining reporting reserve rather than cutting Report off with it.
	ctx, cancel := context.WithTimeout(parent, reportingHorizon)
	defer cancel()
	if sendLocal(ctx, c, "BEGIN", "00000000000000000000000000000000", "1", struct{}{}) != nil {
		return ErrUnsafe
	}
	first, err := actionipc.ReadWorkerSelection(ctx, c)
	if errors.Is(err, actionipc.ErrNoSelection) {
		return actionipc.ErrNoSelection
	}
	if err != nil || first.Version != actionprotocol.WorkerVersion || first.Ordinal != "1" {
		return errInitialSelection
	}
	if first.Kind == "RECOVERY_QUERY" {
		return w.relayRecovery(ctx, c, first)
	}
	if w.executionDisabled() {
		return ErrUnsafe
	}
	if first.Kind == "SNAPSHOT_QUERY" {
		query, err := actionprotocol.DecodeSnapshotQuery(first.Payload)
		if err != nil || !w.bound(query.ServerID, query.OwnerUserID, query.ConnectorInstanceID) {
			return ErrUnsafe
		}
		// A root ceremony can interrupt an already selected snapshot exchange.
		// Retain recovery availability, but never retry publication or resume
		// claims automatically after its completion became uncertain.
		defer func() {
			if result != nil {
				w.disableExecution()
			}
		}()
		bounded, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		reply, requestErr := w.remote.RefreshSnapshot(bounded, query)
		if requestErr != nil && reply.Outcome == "OK" {
			return ErrUnsafe
		}
		raw, encodingErr := json.Marshal(reply)
		_, decodingErr := actionprotocol.DecodeSnapshotReply(raw)
		if encodingErr != nil || decodingErr != nil || reply.Outcome == "OK" && actionprotocol.MatchSnapshotReply(query, reply) != nil {
			return ErrUnsafe
		}
		return sendLocal(bounded, c, "SNAPSHOT_REPLY", first.ConversationNonce, "2", reply)
	}
	if first.Kind != "APPROVAL_SNAPSHOT" {
		return ErrUnsafe
	}
	approval, err := actionprotocol.DecodeApprovalSnapshot(first.Payload)
	if err != nil || !w.bound(approval.ServerID, approval.OwnerUserID, approval.ConnectorInstanceID) || len(approval.Entries) == 0 {
		return ErrUnsafe
	}
	// From the first claim attempt onward, a lost/invalid response can hide a
	// consumed remote claim. No error path may silently return to polling.
	defer func() {
		if result != nil {
			w.disableExecution()
		}
	}()
	start, err := captureStamp()
	if err != nil {
		return ErrUnsafe
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return ErrUnsafe
	}
	nonce := hex.EncodeToString(random[:])
	targets := make([]string, 0, len(approval.Entries))
	for _, entry := range approval.Entries {
		if len(targets) == 0 || targets[len(targets)-1] != entry.TargetID {
			targets = append(targets, entry.TargetID)
		}
	}
	claim, err := w.remote.Claim(ctx, nonce, targets)
	if err != nil {
		return ErrUnsafe
	}
	if w.executionDisabled() {
		return ErrUnsafe
	}
	if claim == nil {
		return sendLocal(ctx, c, "NO_CLAIM", first.ConversationNonce, "2", struct{}{})
	}
	recv, err := captureStamp()
	if err != nil {
		return ErrUnsafe
	}
	rtt, err := elapsed(start, recv, 30*time.Second)
	if err != nil {
		return ErrUnsafe
	}
	if !w.claimApproved(*claim, approval) {
		return ErrUnsafe
	}
	if sendLocal(ctx, c, "CLAIMED_EXECUTE", first.ConversationNonce, "2", actionprotocol.ClaimedExecute{Claim: *claim, ClaimRTTNanos: strconv.FormatInt(int64(rtt), 10)}) != nil {
		return ErrUnsafe
	}
	previousChallenge := ""
	for checkpoint := 1; checkpoint <= 2; checkpoint++ {
		beforeWait, captureErr := captureStamp()
		if captureErr != nil || w.executionDisabled() {
			return ErrUnsafe
		}
		if _, err = elapsed(start, beforeWait, executionHorizon); err != nil {
			return ErrUnsafe
		}
		message, err := actionipc.ReadExecutionMessage(ctx, c)
		if err != nil || message.Kind != "AUTHORITY_CHALLENGE" || message.ConversationNonce != first.ConversationNonce || message.Ordinal != strconv.Itoa(checkpoint+2) {
			return ErrUnsafe
		}
		query, err := actionprotocol.DecodeCheckRequest(message.Payload)
		if err != nil || query.Request != claim.Request || query.ExecutionLeaseID != claim.ExecutionLeaseID || query.Checkpoint != strconv.Itoa(checkpoint) || query.CheckNonce == previousChallenge {
			return ErrUnsafe
		}
		previousChallenge = query.CheckNonce
		checkStart, captureErr := captureStamp()
		if captureErr != nil || w.executionDisabled() {
			return ErrUnsafe
		}
		if _, err = elapsed(start, checkStart, executionHorizon); err != nil {
			return ErrUnsafe
		}
		decision, err := w.remote.Check(ctx, query)
		if err != nil {
			return ErrUnsafe
		}
		checkEnd, captureErr := captureStamp()
		if captureErr != nil || w.executionDisabled() {
			return ErrUnsafe
		}
		if _, err = elapsed(checkStart, checkEnd, 5*time.Second); err != nil {
			return ErrUnsafe
		}
		if _, err = elapsed(start, checkEnd, executionHorizon); err != nil {
			return ErrUnsafe
		}
		// A typed remote denial is delivered, then execution stays latched off.
		if decision.Decision != "PERMITTED" {
			w.disableExecution()
		}
		if decision.Request != query.Request || decision.ExecutionLeaseID != query.ExecutionLeaseID || decision.Checkpoint != query.Checkpoint || decision.CheckNonce != query.CheckNonce {
			return ErrUnsafe
		}
		if err = sendLocal(ctx, c, "AUTHORITY_REPLY", first.ConversationNonce, message.Ordinal, decision); err != nil {
			return ErrUnsafe
		}
		if decision.Decision != "PERMITTED" {
			return ErrUnsafe
		}
	}
	beforeResult, captureErr := captureStamp()
	if captureErr != nil || w.executionDisabled() {
		return ErrUnsafe
	}
	if _, err = elapsed(start, beforeResult, reportingHorizon); err != nil {
		return ErrUnsafe
	}
	message, err := actionipc.ReadExecutionMessage(ctx, c)
	if err != nil || message.Kind != "RESULT" || message.ConversationNonce != first.ConversationNonce || message.Ordinal != "5" {
		return ErrUnsafe
	}
	report, err := actionprotocol.DecodeReport(message.Payload)
	if err != nil || report.Request != claim.Request || report.ExecutionLeaseID != claim.ExecutionLeaseID || report.CorrelationID != claim.CorrelationID {
		return ErrUnsafe
	}
	end, err := captureStamp()
	if err != nil {
		return ErrUnsafe
	}
	if _, err = elapsed(start, end, reportingHorizon); err != nil || w.executionDisabled() {
		return ErrUnsafe
	}
	receipt, err := w.remote.Report(ctx, claim.Request.ActionID, report)
	if err != nil || actionprotocol.MatchResultReceipt(*claim, receipt) != nil || receipt.ResultCode != report.ResultCode || receipt.CorrelationID != report.CorrelationID {
		return ErrUnsafe
	}
	end, err = captureStamp()
	if err != nil {
		return ErrUnsafe
	}
	if _, err = elapsed(start, end, reportingHorizon); err != nil {
		return ErrUnsafe
	}
	if report.ResultCode == "OUTCOME_UNKNOWN" {
		w.disableExecution()
	}
	return nil
}

func (w *conversationClient) claimApproved(claim actionprotocol.Claim, approval actionprotocol.ApprovalSnapshot) bool {
	raw, err := json.Marshal(claim)
	if err != nil {
		return false
	}
	validated, err := actionprotocol.DecodeClaim(raw)
	if err != nil || validated != claim || !w.bound(claim.Request.ServerID, claim.Request.OwnerUserID, claim.Request.ConnectorInstanceID) || claim.Request.ActorUserID != w.binding.OwnerUserID {
		return false
	}
	for _, entry := range approval.Entries {
		r := claim.Request
		if r.TargetID == entry.TargetID && r.TargetKey == entry.TargetKey && r.Operation == entry.Operation && r.ParameterSHA256 == entry.ParameterSHA256 && r.PolicyRevision == entry.PolicyRevision && r.CapabilityRevision == entry.CapabilityRevision && r.Parameters == entry.HTTP().Parameters {
			return true
		}
	}
	return false
}

var _ actionDelivery = (*actionremote.Client)(nil)
