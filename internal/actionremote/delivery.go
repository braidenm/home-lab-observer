package actionremote

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

func (c *Client) bound(request actionprotocol.RequestReceipt) bool {
	return c != nil && request.ServerID == c.binding.ServerID && request.OwnerUserID == c.binding.OwnerUserID &&
		request.ActorUserID == c.binding.OwnerUserID && request.ConnectorInstanceID == c.binding.ConnectorInstanceID
}

// Claim returns (nil,nil) ONLY for a verified empty HTTP204. An uncertain
// response may have consumed a claim and must stop the worker's new claims.
func (c *Client) Claim(ctx context.Context, nonce string, eligible []string) (*actionprotocol.Claim, error) {
	body, err := json.Marshal(actionprotocol.ClaimRequest{ClaimNonce: nonce, EligibleTargetIDs: eligible})
	if err != nil {
		return nil, ErrInvalid
	}
	if _, err = actionprotocol.DecodeClaimRequest(body); err != nil {
		return nil, ErrInvalid
	}
	status, raw, err := c.request(ctx, http.MethodPost, "/claim", body)
	if err != nil {
		if errors.Is(err, ErrCredential) || errors.Is(err, ErrDenied) || errors.Is(err, ErrConflict) || errors.Is(err, ErrBusy) {
			return nil, err
		}
		return nil, ErrClaimUnconfirmed
	}
	if status == http.StatusNoContent {
		return nil, nil
	}
	if status != http.StatusOK {
		return nil, ErrClaimUnconfirmed
	}
	response, err := actionprotocol.DecodeClaimResponse(raw)
	if err != nil || response.ClaimNonce != nonce || !c.bound(response.Claim.Request) {
		return nil, ErrClaimUnconfirmed
	}
	eligibleTarget := false
	for _, target := range eligible {
		if target == response.Claim.Request.TargetID {
			eligibleTarget = true
		}
	}
	if !eligibleTarget {
		return nil, ErrClaimUnconfirmed
	}
	return &response.Claim, nil
}

func (c *Client) Check(ctx context.Context, request actionprotocol.CheckRequest) (actionprotocol.ExecutionDecision, error) {
	body, err := json.Marshal(request)
	if err != nil || !c.bound(request.Request) {
		return actionprotocol.ExecutionDecision{}, ErrInvalid
	}
	if _, err = actionprotocol.DecodeCheckRequest(body); err != nil {
		return actionprotocol.ExecutionDecision{}, ErrInvalid
	}
	status, raw, err := c.request(ctx, http.MethodPost, "/current-check", body)
	if err != nil {
		return actionprotocol.ExecutionDecision{}, err
	}
	if status != http.StatusOK {
		return actionprotocol.ExecutionDecision{}, ErrUnavailable
	}
	decision, err := actionprotocol.DecodeExecutionDecision(raw)
	if err != nil || decision.Request != request.Request || decision.ExecutionLeaseID != request.ExecutionLeaseID ||
		decision.Checkpoint != request.Checkpoint || decision.CheckNonce != request.CheckNonce {
		return actionprotocol.ExecutionDecision{}, ErrUnavailable
	}
	return decision, nil
}

func (c *Client) Report(ctx context.Context, actionID string, report actionprotocol.Report) (actionprotocol.ResultReceipt, error) {
	body, err := json.Marshal(report)
	if err != nil || !id(actionID, "hla_") || actionID != report.Request.ActionID || !c.bound(report.Request) {
		return actionprotocol.ResultReceipt{}, ErrInvalid
	}
	if _, err = actionprotocol.DecodeReport(body); err != nil {
		return actionprotocol.ResultReceipt{}, ErrInvalid
	}
	status, raw, err := c.requestCorrelated(ctx, http.MethodPost, "/"+actionID+"/report-result", body, report.CorrelationID)
	if err != nil {
		return actionprotocol.ResultReceipt{}, err
	}
	if status != http.StatusOK {
		return actionprotocol.ResultReceipt{}, ErrUnavailable
	}
	receipt, err := actionprotocol.DecodeResultReceipt(raw)
	if err != nil || receipt.Request != report.Request || receipt.ExecutionLeaseID != report.ExecutionLeaseID ||
		receipt.ResultCode != report.ResultCode || receipt.CorrelationID != report.CorrelationID {
		return actionprotocol.ResultReceipt{}, ErrUnavailable
	}
	return receipt, nil
}

func (c *Client) Retained(ctx context.Context, actionID string) (actionprotocol.RetainedStatus, error) {
	if !id(actionID, "hla_") {
		return actionprotocol.RetainedStatus{}, ErrInvalid
	}
	status, raw, err := c.request(ctx, http.MethodGet, "/"+actionID+"/retained-status", nil)
	if err != nil {
		return actionprotocol.RetainedStatus{}, err
	}
	if status != http.StatusOK {
		return actionprotocol.RetainedStatus{}, ErrUnavailable
	}
	retained, err := actionprotocol.DecodeRetainedStatus(raw)
	if err != nil || retained.ActionID != actionID || retained.Request != nil && !c.bound(*retained.Request) {
		return actionprotocol.RetainedStatus{}, ErrUnavailable
	}
	return retained, nil
}
