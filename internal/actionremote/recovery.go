package actionremote

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

func (c *Client) boundApproval(a *actionprotocol.Approval) bool {
	return a == nil || c != nil && a.ServerID == c.binding.ServerID && a.OwnerUserID == c.binding.OwnerUserID && a.ConnectorInstanceID == c.binding.ConnectorInstanceID
}

func (c *Client) Propose(ctx context.Context, actionID string, p actionprotocol.RecoveryProposal) (actionprotocol.RecoveryView, error) {
	body, err := json.Marshal(p)
	if err != nil || !id(actionID, "hla_") {
		return actionprotocol.RecoveryView{}, ErrInvalid
	}
	if _, err = actionprotocol.DecodeRecoveryProposal(body); err != nil {
		return actionprotocol.RecoveryView{}, ErrInvalid
	}
	status, raw, err := c.request(ctx, http.MethodPost, "/"+actionID+"/recovery/propose", body)
	if err != nil {
		return actionprotocol.RecoveryView{}, err
	}
	if status != http.StatusOK {
		return actionprotocol.RecoveryView{}, ErrUnavailable
	}
	v, err := actionprotocol.DecodeRecoveryView(raw)
	if err != nil || v.ActionID != actionID || v.RecoveryID == nil || v.RemoteReceiptDigest == nil || *v.RemoteReceiptDigest != p.RemoteReceiptDigest ||
		v.EvidenceKind == nil || *v.EvidenceKind != p.EvidenceKind || !c.boundApproval(v.CurrentApproval) {
		return actionprotocol.RecoveryView{}, ErrUnavailable
	}
	return v, nil
}

func (c *Client) RecoveryStatus(ctx context.Context, actionID string) (actionprotocol.RecoveryView, error) {
	if !id(actionID, "hla_") {
		return actionprotocol.RecoveryView{}, ErrInvalid
	}
	status, raw, err := c.request(ctx, http.MethodGet, "/"+actionID+"/recovery", nil)
	if err != nil {
		return actionprotocol.RecoveryView{}, err
	}
	if status != http.StatusOK {
		return actionprotocol.RecoveryView{}, ErrUnavailable
	}
	v, err := actionprotocol.DecodeRecoveryView(raw)
	if err != nil || v.ActionID != actionID || !c.boundApproval(v.CurrentApproval) {
		return actionprotocol.RecoveryView{}, ErrUnavailable
	}
	return v, nil
}

func (c *Client) RecoveryCheck(ctx context.Context, actionID string, r actionprotocol.RecoveryCheck) (actionprotocol.RecoveryDecision, error) {
	body, err := json.Marshal(r)
	if err != nil || !id(actionID, "hla_") {
		return actionprotocol.RecoveryDecision{}, ErrInvalid
	}
	if _, err = actionprotocol.DecodeRecoveryCheck(body); err != nil {
		return actionprotocol.RecoveryDecision{}, ErrInvalid
	}
	status, raw, err := c.request(ctx, http.MethodPost, "/"+actionID+"/recovery/current-check", body)
	if err != nil {
		return actionprotocol.RecoveryDecision{}, err
	}
	if status != http.StatusOK {
		return actionprotocol.RecoveryDecision{}, ErrUnavailable
	}
	d, err := actionprotocol.DecodeRecoveryDecision(raw)
	if err != nil {
		return actionprotocol.RecoveryDecision{}, ErrUnavailable
	}
	a := d.Approval
	if a.ActionID != actionID || !c.boundApproval(&a) || a.RecoveryID != r.RecoveryID || a.PrepareID != r.PrepareID ||
		a.ApprovalRevision != r.ApprovalRevision || a.RemoteReceiptDigest != r.RemoteReceiptDigest || d.CheckNonce != r.CheckNonce ||
		!equalDigest(a.CommittedACKDigest, r.CommittedACKDigest) {
		return actionprotocol.RecoveryDecision{}, ErrUnavailable
	}
	return d, nil
}

func (c *Client) Finalize(ctx context.Context, actionID string, f actionprotocol.FinalizeRequest) (actionprotocol.Finalization, error) {
	body, err := json.Marshal(f)
	if err != nil || !id(actionID, "hla_") {
		return actionprotocol.Finalization{}, ErrInvalid
	}
	if _, err = actionprotocol.DecodeFinalizeRequest(body); err != nil {
		return actionprotocol.Finalization{}, ErrInvalid
	}
	status, raw, err := c.request(ctx, http.MethodPost, "/"+actionID+"/recovery/finalize", body)
	if err != nil {
		return actionprotocol.Finalization{}, err
	}
	if status != http.StatusOK {
		return actionprotocol.Finalization{}, ErrUnavailable
	}
	finalized, err := actionprotocol.DecodeFinalization(raw)
	if err != nil || finalized.ActionID != actionID || finalized.RecoveryID != f.RecoveryID || finalized.PrepareID != f.PrepareID ||
		finalized.ApprovalRevision != f.ApprovalRevision || finalized.RemoteReceiptDigest != f.RemoteReceiptDigest || finalized.ACKDigest != f.ACKDigest {
		return actionprotocol.Finalization{}, ErrUnavailable
	}
	return finalized, nil
}

func equalDigest(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
