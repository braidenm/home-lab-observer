//go:build linux

package actionbroker

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionbudget"
	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionipc"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

type acceptedInvocation struct {
	conn        *net.UnixConn
	reservation *reservation
	claim       actionprotocol.Claim
	journal     actioncore.Journal
	execution   *actionbudget.Execution
	resolved    resolvedBinding
}

func (b *brokerState) readinessCurrent() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, digest, err := policyEntries(b.policy)
	if err != nil || b.quiescing || b.executionClosed.Load() || b.readyHead != b.policy.Revision || b.readyEntriesDigest != digest {
		return false
	}
	_, err = b.clock.Begin(b.readiness)
	return err == nil
}

func (b *brokerState) approvalFor(r *reservation) (actionprotocol.ApprovalSnapshot, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r == nil || b.live[r.nonce] != r || b.quiescing || b.executionClosed.Load() || r.policyHead != b.policy.Revision {
		return actionprotocol.ApprovalSnapshot{}, ErrRecoveryRequired
	}
	entries, _, err := policyEntries(b.policy)
	if err != nil {
		return actionprotocol.ApprovalSnapshot{}, err
	}
	local := make([]actionprotocol.LocalEntry, 0, len(entries))
	for _, e := range entries {
		if containsTarget(r.targets, e.TargetID) {
			local = append(local, actionprotocol.LocalFromEntry(e))
		}
	}
	return actionprotocol.ApprovalSnapshot{ServerID: b.policy.ServerID, OwnerUserID: b.policy.OwnerUserID, ConnectorInstanceID: b.policy.ConnectorInstanceID, Entries: local}, nil
}

func (b *brokerState) abandon(r *reservation) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r != nil && b.live[r.nonce] == r {
		// A timeout, EOF or malformed reply can hide a committed remote claim.
		// No local record is invented, and no old nonce can resume it later.
		b.recoveryRequired = true
		delete(b.live, r.nonce)
	}
}

func writeLocal(ctx context.Context, c *net.UnixConn, kind, nonce, ordinal string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return actionipc.ErrFrame
	}
	return actionipc.Write(ctx, c, actionprotocol.Envelope{Version: actionprotocol.WorkerVersion, Kind: kind, ConversationNonce: nonce, Ordinal: ordinal, Payload: raw})
}

// readWorkerInvocation is called only after the fixed-path listener has proven
// the worker's actual peer/profile. A greeting is not that proof. Control and
// NO_CLAIM always close; only a fresh validated execution transfers this socket
// and its owned reservation to the joining invocation handler.
func (b *brokerState) readWorkerInvocation(ctx context.Context, c *net.UnixConn) (_ *acceptedInvocation, result error) {
	if readWorkerBeginning(ctx, c) != nil {
		c.Close()
		return nil, ErrRecoveryRequired
	}
	return b.selectWorkerInvocation(ctx, c)
}

func readWorkerBeginning(ctx context.Context, c *net.UnixConn) error {
	begin, err := actionipc.Read(ctx, c)
	if err != nil || begin.Version != actionprotocol.WorkerVersion || begin.Kind != "BEGIN" {
		return ErrRecoveryRequired
	}
	return nil
}

// Selection begins only AFTER the authenticated stateless greeting. Runnable
// consumers additionally classify the owned registry scope before calling it.
func (b *brokerState) selectWorkerInvocation(ctx context.Context, c *net.UnixConn) (_ *acceptedInvocation, result error) {
	transferred := false
	defer func() {
		if !transferred {
			c.Close()
		}
	}()
	if !b.readinessCurrent() {
		bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		control, err := b.beginControl(bounded)
		if err != nil {
			return nil, err
		}
		defer b.cancelControl(control)
		if writeLocal(bounded, c, "SNAPSHOT_QUERY", control.query.PublicationID, "1", control.query) != nil {
			return nil, ErrRecoveryRequired
		}
		reply, err := actionipc.Read(bounded, c)
		if err != nil || reply.Version != actionprotocol.WorkerVersion || reply.Kind != "SNAPSHOT_REPLY" || reply.Ordinal != "2" || reply.ConversationNonce != control.query.PublicationID {
			return nil, ErrRecoveryRequired
		}
		value, err := actionprotocol.DecodeSnapshotReply(reply.Payload)
		if err != nil {
			return nil, ErrRecoveryRequired
		}
		return nil, b.completeControl(bounded, control, value)
	}
	r, err := b.reserve(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if result != nil {
			b.abandon(r)
		}
	}()
	approval, err := b.approvalFor(r)
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	deliveryCtx, cancel, err := r.conversation.DeliveryContext(ctx)
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	defer cancel()
	if r.conversation.CheckDelivery(deliveryCtx) != nil || writeLocal(deliveryCtx, c, "APPROVAL_SNAPSHOT", r.nonce, "1", approval) != nil || r.conversation.CheckDelivery(deliveryCtx) != nil {
		return nil, ErrRecoveryRequired
	}
	delivery, err := actionipc.ReadClaimDelivery(deliveryCtx, c)
	if err != nil || r.conversation.CheckDelivery(deliveryCtx) != nil || delivery.Version != actionprotocol.WorkerVersion || delivery.ConversationNonce != r.nonce || delivery.Ordinal != "2" {
		return nil, ErrRecoveryRequired
	}
	if delivery.Kind == "NO_CLAIM" {
		return nil, b.releaseUnclaimed(deliveryCtx, r)
	}
	if delivery.Kind != "CLAIMED_EXECUTE" {
		return nil, ErrRecoveryRequired
	}
	claimed, err := actionprotocol.DecodeClaimedExecute(delivery.Payload)
	if err != nil {
		return nil, ErrRecoveryRequired
	}
	rtt, err := strconv.ParseInt(claimed.ClaimRTTNanos, 10, 64)
	if err != nil || r.conversation.CheckDelivery(deliveryCtx) != nil {
		return nil, ErrRecoveryRequired
	}
	journal, execution, resolved, err := b.bindClaim(deliveryCtx, r, claimed.Claim, time.Duration(rtt))
	if err != nil {
		return nil, err
	}
	transferred = true
	return &acceptedInvocation{conn: c, reservation: r, claim: claimed.Claim, journal: journal, execution: execution, resolved: resolved}, nil
}
