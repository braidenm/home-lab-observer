//go:build linux

package actionworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionipc"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
	"github.com/braidenm/home-lab-observer/internal/actionremote"
)

// This private port contains only the five code-owned recovery operations. It
// cannot prepare owner approval, initiate root administration or select a URL.
type recoveryDelivery interface {
	Retained(context.Context, string) (actionprotocol.RetainedStatus, error)
	Propose(context.Context, string, actionprotocol.RecoveryProposal) (actionprotocol.RecoveryView, error)
	RecoveryStatus(context.Context, string) (actionprotocol.RecoveryView, error)
	RecoveryCheck(context.Context, string, actionprotocol.RecoveryCheck) (actionprotocol.RecoveryDecision, error)
	Finalize(context.Context, string, actionprotocol.FinalizeRequest) (actionprotocol.Finalization, error)
}

type recoveryRelay struct {
	action, digest string
	request        *actionprotocol.RequestReceipt
	approval       *actionprotocol.Approval
}

func (w *conversationClient) relayRecovery(parent context.Context, c *net.UnixConn, first actionprotocol.Envelope) error {
	q, err := actionprotocol.DecodeRecoveryQuery(first.Payload)
	if err != nil {
		return ErrUnsafe
	}
	if q.Operation != "STATUS" {
		// An uncertain recovery request must never silently return to claiming.
		w.disableExecution()
	}
	if first.Ordinal != "1" || q.Operation != "STATUS" && q.Operation != "RETAINED" {
		return ErrUnsafe
	}
	remote, ok := w.remote.(recoveryDelivery)
	if !ok {
		return ErrUnsafe
	}
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	origin, err := captureStamp()
	if err != nil {
		return ErrUnsafe
	}
	s := recoveryRelay{action: q.ActionID, digest: q.RemoteReceiptDigest}
	message := first
	for ordinal := 1; ordinal <= 5; ordinal++ {
		start, err := captureStamp()
		if err != nil || ctx.Err() != nil {
			return ErrUnsafe
		}
		if _, err = elapsed(origin, start, 90*time.Second); err != nil {
			return ErrUnsafe
		}
		leg, stop := context.WithTimeout(ctx, 5*time.Second)
		reply := s.deliver(leg, w, remote, q)
		end, sampleErr := captureStamp()
		if sampleErr != nil || relayInBudget(leg, origin, start, end) != nil {
			stop()
			return ErrUnsafe
		}
		writeErr := sendLocal(leg, c, "RECOVERY_REPLY", first.ConversationNonce, message.Ordinal, reply)
		end, sampleErr = captureStamp()
		stop()
		if writeErr != nil || sampleErr != nil || relayInBudget(ctx, origin, start, end) != nil {
			return ErrUnsafe
		}
		// Only uncertain FINALIZE permits one lookup, never a retry. Every
		// other error closes this connection after its finite null-body reply.
		if q.Operation == "PROPOSE" || q.Operation == "STATUS" && (ordinal == 1 || ordinal == 5) || q.Operation == "FINALIZE" && reply.Outcome == "OK" || ordinal == 5 || reply.Outcome != "OK" && q.Operation != "FINALIZE" {
			return nil
		}
		message, err = actionipc.ReadRecoveryContinuation(ctx, c)
		received, sampleErr := captureStamp()
		if err != nil || sampleErr != nil || ctx.Err() != nil {
			return ErrUnsafe
		}
		if _, err := elapsed(origin, received, 90*time.Second); err != nil {
			return ErrUnsafe
		}
		if message.ConversationNonce != first.ConversationNonce || message.Ordinal != strconv.Itoa(ordinal+1) {
			return ErrUnsafe
		}
		next, err := actionprotocol.DecodeRecoveryQuery(message.Payload)
		if err != nil || next.ActionID != s.action || next.RemoteReceiptDigest != s.digest || !nextRecoveryOperation(q.Operation, next.Operation, ordinal) {
			return ErrUnsafe
		}
		q = next
	}
	return ErrUnsafe
}

// The caller passes its actual post-I/O native sample. Cancellation checked
// here also covers cancellation concurrent with sampling, before another send.
func relayInBudget(ctx context.Context, origin, start, end nativeStamp) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrUnsafe
	}
	if _, err := elapsed(origin, end, 90*time.Second); err != nil {
		return ErrUnsafe
	}
	if _, err := elapsed(start, end, 5*time.Second); err != nil {
		return ErrUnsafe
	}
	return nil
}

func nextRecoveryOperation(previous, next string, ordinal int) bool {
	switch ordinal {
	case 1:
		return previous == "RETAINED" && (next == "PROPOSE" || next == "STATUS")
	case 2:
		return previous == "STATUS" && next == "CHECK"
	case 3:
		return previous == "CHECK" && next == "FINALIZE"
	case 4:
		return previous == "FINALIZE" && next == "STATUS"
	}
	return false
}

func recoveryOutcome(err error) string {
	switch {
	case errors.Is(err, actionremote.ErrDenied):
		return "DENIED"
	case errors.Is(err, actionremote.ErrConflict):
		return "CONFLICT"
	default:
		return "UNAVAILABLE"
	}
}

func (s *recoveryRelay) deliver(ctx context.Context, w *conversationClient, remote recoveryDelivery, q actionprotocol.RecoveryQuery) actionprotocol.RecoveryReply {
	reply := actionprotocol.RecoveryReply{Operation: q.Operation, ActionID: s.action, RemoteReceiptDigest: s.digest, Outcome: "UNAVAILABLE", Body: json.RawMessage("null")}
	var body any
	var err error
	switch q.Operation {
	case "RETAINED":
		var value actionprotocol.RetainedStatus
		value, err = remote.Retained(ctx, s.action)
		if err == nil && (value.Request == nil || !w.bound(value.Request.ServerID, value.Request.OwnerUserID, value.Request.ConnectorInstanceID) || value.Request.ActorUserID != w.binding.OwnerUserID) {
			err = ErrUnsafe
		}
		if err == nil {
			s.request = value.Request
			body = value
		}
	case "PROPOSE", "STATUS":
		var value actionprotocol.RecoveryView
		if q.Operation == "PROPOSE" {
			p, decodeErr := actionprotocol.DecodeRecoveryProposal(q.Body)
			if decodeErr != nil || s.request == nil {
				return reply
			}
			value, err = remote.Propose(ctx, s.action, p)
		} else {
			value, err = remote.RecoveryStatus(ctx, s.action)
		}
		if err == nil && (value.CurrentApproval != nil && !w.bound(value.CurrentApproval.ServerID, value.CurrentApproval.OwnerUserID, value.CurrentApproval.ConnectorInstanceID) || s.request != nil && actionprotocol.MatchRecoveryView(*s.request, s.digest, value) != nil) {
			err = ErrUnsafe
		}
		if err == nil {
			s.approval = value.CurrentApproval
			body = value
		}
	case "CHECK":
		check, decodeErr := actionprotocol.DecodeRecoveryCheck(q.Body)
		if decodeErr != nil || !s.matchesCheck(check) {
			return reply
		}
		var value actionprotocol.RecoveryDecision
		value, err = remote.RecoveryCheck(ctx, s.action, check)
		if err == nil && actionprotocol.MatchRecoveryDecision(*s.approval, check.CheckNonce, value) != nil {
			// A denial is finite failure, never permission to finalize.
			err = actionremote.ErrDenied
		}
		body = value
	case "FINALIZE":
		f, decodeErr := actionprotocol.DecodeFinalizeRequest(q.Body)
		if decodeErr != nil || s.approval == nil || f.RecoveryID != s.approval.RecoveryID || f.PrepareID != s.approval.PrepareID || f.ApprovalRevision != s.approval.ApprovalRevision || f.RemoteReceiptDigest != s.digest || s.approval.CommittedACKDigest != nil && f.ACKDigest != *s.approval.CommittedACKDigest {
			return reply
		}
		var value actionprotocol.Finalization
		value, err = remote.Finalize(ctx, s.action, f)
		if err == nil && actionprotocol.MatchFinalization(*s.approval, f.ACKDigest, value) != nil {
			err = ErrUnsafe
		}
		body = value
	default:
		return reply
	}
	if err != nil {
		reply.Outcome = recoveryOutcome(err)
		return reply
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return reply
	}
	reply.Outcome, reply.Body = "OK", raw
	encoded, err := json.Marshal(reply)
	if err != nil {
		reply.Outcome, reply.Body = "UNAVAILABLE", json.RawMessage("null")
		return reply
	}
	validated, err := actionprotocol.DecodeRecoveryReply(encoded)
	if err != nil {
		reply.Outcome, reply.Body = "UNAVAILABLE", json.RawMessage("null")
		return reply
	}
	return validated
}

func (s *recoveryRelay) matchesCheck(c actionprotocol.RecoveryCheck) bool {
	a := s.approval
	if a == nil || c.RecoveryID != a.RecoveryID || c.PrepareID != a.PrepareID || c.ApprovalRevision != a.ApprovalRevision || c.RemoteReceiptDigest != s.digest {
		return false
	}
	left, _ := json.Marshal(c.CommittedACKDigest)
	right, _ := json.Marshal(a.CommittedACKDigest)
	return bytes.Equal(left, right)
}

var _ recoveryDelivery = (*actionremote.Client)(nil)
