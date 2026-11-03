package actionprotocol

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

type Claim struct {
	Request                 RequestReceipt `json:"request"`
	ExecutionLeaseID        string         `json:"execution_lease_id"`
	ClaimCredentialID       string         `json:"claim_credential_id"`
	ClaimCredentialRevision string         `json:"claim_credential_revision"`
	ClaimedAt               string         `json:"claimed_at"`
	LeaseExpiresAt          string         `json:"lease_expires_at"`
	CorrelationID           string         `json:"correlation_id"`
}

type ExecutionDecision struct {
	Request            RequestReceipt `json:"request"`
	ExecutionLeaseID   string         `json:"execution_lease_id"`
	Checkpoint         string         `json:"checkpoint"`
	CheckNonce         string         `json:"check_nonce"`
	CredentialID       string         `json:"credential_id"`
	CredentialRevision string         `json:"credential_revision"`
	CheckedAt          string         `json:"checked_at"`
	Decision           string         `json:"decision"`
}

type Report struct {
	Request          RequestReceipt  `json:"request"`
	ExecutionLeaseID string          `json:"execution_lease_id"`
	ResultCode       actioncore.Code `json:"result_code"`
	EvidenceKind     string          `json:"evidence_kind"`
	CorrelationID    string          `json:"correlation_id"`
}

type ResultReceipt struct {
	Request                     RequestReceipt  `json:"request"`
	ExecutionLeaseID            string          `json:"execution_lease_id"`
	ReportingCredentialID       string          `json:"reporting_credential_id"`
	ReportingCredentialRevision string          `json:"reporting_credential_revision"`
	ResultCode                  actioncore.Code `json:"result_code"`
	RecordedAt                  string          `json:"recorded_at"`
	CorrelationID               string          `json:"correlation_id"`
}

type RetainedStatus struct {
	ActionID  string          `json:"action_id"`
	State     string          `json:"state"`
	Request   *RequestReceipt `json:"request"`
	Claim     *Claim          `json:"claim"`
	Result    *ResultReceipt  `json:"result"`
	LaneState string          `json:"lane_state"`
}

func fields(raw []byte, names ...string) (map[string]json.RawMessage, error) {
	if strictJSON(raw) != nil {
		return nil, ErrInvalid
	}
	return objectFields(raw, names...)
}

func nonce(s string) bool { return hexValue(s, 32) && s != "00000000000000000000000000000000" }

func DecodeClaim(raw []byte) (Claim, error) {
	var c Claim
	object, err := fields(raw, "request", "execution_lease_id", "claim_credential_id", "claim_credential_revision", "claimed_at", "lease_expires_at", "correlation_id")
	if err != nil {
		return c, ErrInvalid
	}
	r, err := DecodeRequest(object["request"])
	if err != nil || json.Unmarshal(raw, &c) != nil {
		return Claim{}, ErrInvalid
	}
	c.Request = r
	created, e1 := ParseHTTPTime(r.RequestedAt)
	expires, e2 := ParseHTTPTime(r.ExpiresAt)
	claimed, e3 := ParseHTTPTime(c.ClaimedAt)
	lease, e4 := ParseHTTPTime(c.LeaseExpiresAt)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || claimed.Before(created) || lease.After(expires) ||
		lease.Sub(claimed) != 120*time.Second || !identifier(c.ExecutionLeaseID, "hle_") ||
		!identifier(c.ClaimCredentialID, "hlac_") || !revision(c.ClaimCredentialRevision) || !correlation(c.CorrelationID) {
		return Claim{}, ErrInvalid
	}
	return c, nil
}

func DecodeExecutionDecision(raw []byte) (ExecutionDecision, error) {
	var d ExecutionDecision
	object, err := fields(raw, "request", "execution_lease_id", "checkpoint", "check_nonce", "credential_id", "credential_revision", "checked_at", "decision")
	if err != nil {
		return d, ErrInvalid
	}
	r, err := DecodeRequest(object["request"])
	if err != nil || json.Unmarshal(raw, &d) != nil {
		return ExecutionDecision{}, ErrInvalid
	}
	d.Request = r
	_, err = ParseHTTPTime(d.CheckedAt)
	if err != nil || !identifier(d.ExecutionLeaseID, "hle_") || (d.Checkpoint != "1" && d.Checkpoint != "2") ||
		!nonce(d.CheckNonce) || !identifier(d.CredentialID, "hlac_") || !revision(d.CredentialRevision) ||
		(d.Decision != "PERMITTED" && d.Decision != "DENIED") {
		return ExecutionDecision{}, ErrInvalid
	}
	return d, nil
}

func MatchExecutionDecision(c Claim, checkpoint, challenge string, d ExecutionDecision) error {
	checked, e1 := ParseHTTPTime(d.CheckedAt)
	claimed, e2 := ParseHTTPTime(c.ClaimedAt)
	expires, e3 := ParseHTTPTime(c.LeaseExpiresAt)
	if e1 != nil || e2 != nil || e3 != nil || d.Request != c.Request || d.ExecutionLeaseID != c.ExecutionLeaseID ||
		d.Checkpoint != checkpoint || d.CheckNonce != challenge || checked.Before(claimed) || !checked.Before(expires) ||
		d.Decision != "PERMITTED" || !identifier(d.CredentialID, "hlac_") || !revision(d.CredentialRevision) {
		return ErrInvalid
	}
	return nil
}

func resultCode(operation actioncore.Operation, code actioncore.Code) bool {
	switch code {
	case actioncore.StateConflict, actioncore.OutcomeUnknown:
		return true
	case actioncore.Started, actioncore.AlreadyRunning:
		return operation == actioncore.Start
	case actioncore.Stopped, actioncore.AlreadyStopped:
		return operation == actioncore.Stop
	case actioncore.Restarted:
		return operation == actioncore.Restart
	}
	return false
}

func DecodeReport(raw []byte) (Report, error) {
	var r Report
	object, err := fields(raw, "request", "execution_lease_id", "result_code", "evidence_kind", "correlation_id")
	if err != nil {
		return r, ErrInvalid
	}
	request, err := DecodeRequest(object["request"])
	if err != nil || json.Unmarshal(raw, &r) != nil {
		return Report{}, ErrInvalid
	}
	r.Request = request
	if !identifier(r.ExecutionLeaseID, "hle_") || !correlation(r.CorrelationID) || !resultCode(r.Request.Operation, r.ResultCode) ||
		(r.ResultCode == actioncore.OutcomeUnknown && r.EvidenceKind != "QUIESCED_INTENT") ||
		(r.ResultCode != actioncore.OutcomeUnknown && r.EvidenceKind != "RETAINED_TERMINAL") {
		return Report{}, ErrInvalid
	}
	return r, nil
}

func DecodeResultReceipt(raw []byte) (ResultReceipt, error) {
	var r ResultReceipt
	object, err := fields(raw, "request", "execution_lease_id", "reporting_credential_id", "reporting_credential_revision", "result_code", "recorded_at", "correlation_id")
	if err != nil {
		return r, ErrInvalid
	}
	request, err := DecodeRequest(object["request"])
	if err != nil || json.Unmarshal(raw, &r) != nil {
		return ResultReceipt{}, ErrInvalid
	}
	r.Request = request
	recorded, e1 := ParseHTTPTime(r.RecordedAt)
	created, e2 := ParseHTTPTime(request.RequestedAt)
	expires, e3 := ParseHTTPTime(request.ExpiresAt)
	if e1 != nil || e2 != nil || e3 != nil || recorded.Before(created) || !recorded.Before(expires) ||
		!identifier(r.ExecutionLeaseID, "hle_") || !identifier(r.ReportingCredentialID, "hlac_") ||
		!revision(r.ReportingCredentialRevision) || !resultCode(request.Operation, r.ResultCode) || !correlation(r.CorrelationID) {
		return ResultReceipt{}, ErrInvalid
	}
	return r, nil
}

func MatchResultReceipt(c Claim, r ResultReceipt) error {
	recorded, e1 := ParseHTTPTime(r.RecordedAt)
	claimed, e2 := ParseHTTPTime(c.ClaimedAt)
	expires, e3 := ParseHTTPTime(c.LeaseExpiresAt)
	if e1 != nil || e2 != nil || e3 != nil || r.Request != c.Request || r.ExecutionLeaseID != c.ExecutionLeaseID ||
		recorded.Before(claimed) || !recorded.Before(expires) {
		return ErrInvalid
	}
	return nil
}

func isNull(raw []byte) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

func DecodeRetainedStatus(raw []byte) (RetainedStatus, error) {
	var s RetainedStatus
	object, err := fields(raw, "action_id", "state", "request", "claim", "result", "lane_state")
	if err != nil || json.Unmarshal(raw, &s) != nil || !identifier(s.ActionID, "hla_") {
		return RetainedStatus{}, ErrInvalid
	}
	if !isNull(object["request"]) {
		r, err := DecodeRequest(object["request"])
		if err != nil || r.ActionID != s.ActionID {
			return RetainedStatus{}, ErrInvalid
		}
		s.Request = &r
	}
	if !isNull(object["claim"]) {
		c, err := DecodeClaim(object["claim"])
		if err != nil || s.Request == nil || c.Request != *s.Request {
			return RetainedStatus{}, ErrInvalid
		}
		s.Claim = &c
	}
	if !isNull(object["result"]) {
		r, err := DecodeResultReceipt(object["result"])
		if err != nil || s.Claim == nil || MatchResultReceipt(*s.Claim, r) != nil {
			return RetainedStatus{}, ErrInvalid
		}
		s.Result = &r
	}
	valid := false
	switch s.State {
	case "MISSING":
		valid = s.Request == nil && s.Claim == nil && s.Result == nil && s.LaneState == "MISSING"
	case "REQUESTED":
		valid = s.Request != nil && s.Claim == nil && s.Result == nil && s.LaneState == "HELD"
	case "CLAIMED":
		valid = s.Request != nil && s.Claim != nil && s.Result == nil && s.LaneState == "HELD"
	case "EXPIRED_UNCONFIRMED":
		valid = s.Request != nil && s.Claim != nil && s.Result == nil && (s.LaneState == "HELD" || s.LaneState == "RELEASED")
	case "EXPIRED", "CANCELED":
		valid = s.Request != nil && s.Claim == nil && s.Result == nil && s.LaneState == "RELEASED"
	case "SUCCEEDED":
		valid = s.Result != nil && s.Result.ResultCode != actioncore.StateConflict && s.Result.ResultCode != actioncore.OutcomeUnknown && s.LaneState == "RELEASED"
	case "FAILED":
		valid = s.Result != nil && s.Result.ResultCode == actioncore.StateConflict && s.LaneState == "RELEASED"
	case "OUTCOME_UNKNOWN":
		// Operational release never rewrites the original outcome. This decoder
		// grants no recovery/dispatch authority; consumers still require HELD
		// for a proposal and actual finalization evidence for two-sided readback.
		valid = s.Result != nil && s.Result.ResultCode == actioncore.OutcomeUnknown && (s.LaneState == "HELD" || s.LaneState == "RELEASED")
	}
	if !valid {
		return RetainedStatus{}, ErrInvalid
	}
	return s, nil
}
