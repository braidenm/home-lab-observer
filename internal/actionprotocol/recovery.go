package actionprotocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"time"
)

type RecoveryProposal struct {
	ProposalID          string `json:"proposal_id"`
	RemoteReceiptDigest string `json:"remote_receipt_digest"`
	EvidenceKind        string `json:"evidence_kind"`
}
type PrepareRequest struct {
	RecoveryID               string  `json:"recovery_id"`
	ExpectedLaneRevision     string  `json:"expected_lane_revision"`
	ExpectedApprovalRevision *string `json:"expected_approval_revision"`
	RemoteReceiptDigest      string  `json:"remote_receipt_digest"`
	EvidenceKind             string  `json:"evidence_kind"`
	AcknowledgeUncertainty   bool    `json:"acknowledge_uncertainty"`
	CommittedACKDigest       *string `json:"committed_ack_digest"`
}
type Approval struct {
	RecoveryID          string  `json:"recovery_id"`
	PrepareID           string  `json:"prepare_id"`
	OwnerDecisionID     string  `json:"owner_decision_id"`
	ApprovalRevision    string  `json:"approval_revision"`
	PreviousPrepareID   *string `json:"previous_prepare_id"`
	ServerID            string  `json:"server_id"`
	OwnerUserID         string  `json:"owner_user_id"`
	ConnectorInstanceID string  `json:"connector_instance_id"`
	ActionID            string  `json:"action_id"`
	RemoteReceiptDigest string  `json:"remote_receipt_digest"`
	EvidenceKind        string  `json:"evidence_kind"`
	LaneRevision        string  `json:"lane_revision"`
	CommittedACKDigest  *string `json:"committed_ack_digest"`
	PreparedAt          string  `json:"prepared_at"`
	ExpiresAt           string  `json:"expires_at"`
}
type RecoveryCheck struct {
	RecoveryID          string  `json:"recovery_id"`
	PrepareID           string  `json:"prepare_id"`
	ApprovalRevision    string  `json:"approval_revision"`
	RemoteReceiptDigest string  `json:"remote_receipt_digest"`
	CheckNonce          string  `json:"check_nonce"`
	CommittedACKDigest  *string `json:"committed_ack_digest"`
}
type RecoveryDecision struct {
	Approval           Approval `json:"approval"`
	CheckNonce         string   `json:"check_nonce"`
	CredentialID       string   `json:"credential_id"`
	CredentialRevision string   `json:"credential_revision"`
	CheckedAt          string   `json:"checked_at"`
	Decision           string   `json:"decision"`
}
type FinalizeRequest struct {
	RecoveryID          string `json:"recovery_id"`
	PrepareID           string `json:"prepare_id"`
	ApprovalRevision    string `json:"approval_revision"`
	RemoteReceiptDigest string `json:"remote_receipt_digest"`
	ACKDigest           string `json:"ack_digest"`
}
type Finalization struct {
	RecoveryID           string `json:"recovery_id"`
	PrepareID            string `json:"prepare_id"`
	OwnerDecisionID      string `json:"owner_decision_id"`
	ApprovalRevision     string `json:"approval_revision"`
	ActionID             string `json:"action_id"`
	RemoteReceiptDigest  string `json:"remote_receipt_digest"`
	ACKDigest            string `json:"ack_digest"`
	ReleasedLaneRevision string `json:"released_lane_revision"`
	FinalizedAt          string `json:"finalized_at"`
}
type RecoveryView struct {
	ActionID            string        `json:"action_id"`
	RecoveryID          *string       `json:"recovery_id"`
	RemoteReceiptDigest *string       `json:"remote_receipt_digest"`
	EvidenceKind        *string       `json:"evidence_kind"`
	LaneRevision        string        `json:"lane_revision"`
	State               string        `json:"state"`
	CurrentApproval     *Approval     `json:"current_approval"`
	Finalization        *Finalization `json:"finalization"`
	RemainingApprovals  int           `json:"remaining_approvals"`
}

func evidenceKind(s string) bool {
	return s == "MISSING_RECORD" || s == "ADMITTED_WITHOUT_INTENT" || s == "INTENT_WITHOUT_TERMINAL" || s == "RETAINED_KNOWN_TERMINAL"
}
func approvalRevision(s string) bool { return len(s) == 1 && s[0] >= '1' && s[0] <= '8' }
func optionalDigest(s *string) bool  { return s == nil || hexValue(*s, 64) }
func equalOptional(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// ProposalID is public immutable correlation, never evidence or authority.
func ProposalID(digest, evidence string) (string, error) {
	if !hexValue(digest, 64) || !evidenceKind(evidence) {
		return "", ErrInvalid
	}
	h := sha256.Sum256([]byte("observer-recovery-proposal/v1\n" + digest + "\n" + evidence))
	return hex.EncodeToString(h[:16]), nil
}

func DecodeRecoveryProposal(raw []byte) (RecoveryProposal, error) {
	var p RecoveryProposal
	_, err := fields(raw, "proposal_id", "remote_receipt_digest", "evidence_kind")
	if err != nil || json.Unmarshal(raw, &p) != nil {
		return RecoveryProposal{}, ErrInvalid
	}
	id, err := ProposalID(p.RemoteReceiptDigest, p.EvidenceKind)
	if err != nil || id != p.ProposalID {
		return RecoveryProposal{}, ErrInvalid
	}
	return p, nil
}
func DecodePrepareRequest(raw []byte) (PrepareRequest, error) {
	var p PrepareRequest
	_, err := fields(raw, "recovery_id", "expected_lane_revision", "expected_approval_revision", "remote_receipt_digest", "evidence_kind", "acknowledge_uncertainty", "committed_ack_digest")
	if err != nil || json.Unmarshal(raw, &p) != nil || !nonce(p.RecoveryID) || !revision(p.ExpectedLaneRevision) ||
		p.ExpectedApprovalRevision != nil && !approvalRevision(*p.ExpectedApprovalRevision) || !hexValue(p.RemoteReceiptDigest, 64) ||
		!evidenceKind(p.EvidenceKind) || !p.AcknowledgeUncertainty || !optionalDigest(p.CommittedACKDigest) || p.ExpectedApprovalRevision == nil && p.CommittedACKDigest != nil {
		return PrepareRequest{}, ErrInvalid
	}
	return p, nil
}
func DecodeApproval(raw []byte) (Approval, error) {
	var a Approval
	_, err := fields(raw, "recovery_id", "prepare_id", "owner_decision_id", "approval_revision", "previous_prepare_id", "server_id", "owner_user_id", "connector_instance_id", "action_id", "remote_receipt_digest", "evidence_kind", "lane_revision", "committed_ack_digest", "prepared_at", "expires_at")
	if err != nil || json.Unmarshal(raw, &a) != nil || validateApproval(a) != nil {
		return Approval{}, ErrInvalid
	}
	return a, nil
}
func validateApproval(a Approval) error {
	prepared, e1 := ParseHTTPTime(a.PreparedAt)
	expires, e2 := ParseHTTPTime(a.ExpiresAt)
	if !nonce(a.RecoveryID) || !nonce(a.PrepareID) || !nonce(a.OwnerDecisionID) || !approvalRevision(a.ApprovalRevision) ||
		(a.ApprovalRevision == "1") != (a.PreviousPrepareID == nil) || a.PreviousPrepareID != nil && (!nonce(*a.PreviousPrepareID) || *a.PreviousPrepareID == a.PrepareID) ||
		!identifier(a.ServerID, "srv_") || !identifier(a.OwnerUserID, "usr_") || !identifier(a.ConnectorInstanceID, "agent_") || !identifier(a.ActionID, "hla_") ||
		!hexValue(a.RemoteReceiptDigest, 64) || !evidenceKind(a.EvidenceKind) || !revision(a.LaneRevision) || !optionalDigest(a.CommittedACKDigest) ||
		a.ApprovalRevision == "1" && a.CommittedACKDigest != nil || e1 != nil || e2 != nil || !expires.After(prepared) || expires.Sub(prepared) > 5*time.Minute {
		return ErrInvalid
	}
	return nil
}
func DecodeRecoveryCheck(raw []byte) (RecoveryCheck, error) {
	var c RecoveryCheck
	_, err := fields(raw, "recovery_id", "prepare_id", "approval_revision", "remote_receipt_digest", "check_nonce", "committed_ack_digest")
	if err != nil || json.Unmarshal(raw, &c) != nil || !nonce(c.RecoveryID) || !nonce(c.PrepareID) || !approvalRevision(c.ApprovalRevision) || !hexValue(c.RemoteReceiptDigest, 64) || !nonce(c.CheckNonce) || !optionalDigest(c.CommittedACKDigest) {
		return RecoveryCheck{}, ErrInvalid
	}
	return c, nil
}
func DecodeRecoveryDecision(raw []byte) (RecoveryDecision, error) {
	var d RecoveryDecision
	o, err := fields(raw, "approval", "check_nonce", "credential_id", "credential_revision", "checked_at", "decision")
	if err != nil || json.Unmarshal(raw, &d) != nil {
		return RecoveryDecision{}, ErrInvalid
	}
	a, err := DecodeApproval(o["approval"])
	if err != nil {
		return RecoveryDecision{}, ErrInvalid
	}
	d.Approval = a
	checked, e1 := ParseHTTPTime(d.CheckedAt)
	prepared, e2 := ParseHTTPTime(a.PreparedAt)
	if e1 != nil || e2 != nil || checked.Before(prepared) || !nonce(d.CheckNonce) || !identifier(d.CredentialID, "hlac_") || !revision(d.CredentialRevision) || d.Decision != "PERMITTED" && d.Decision != "DENIED" {
		return RecoveryDecision{}, ErrInvalid
	}
	return d, nil
}
func MatchRecoveryDecision(a Approval, challenge string, d RecoveryDecision) error {
	checked, e1 := ParseHTTPTime(d.CheckedAt)
	expires, e2 := ParseHTTPTime(a.ExpiresAt)
	prepared, e3 := ParseHTTPTime(a.PreparedAt)
	if validateApproval(a) != nil || !sameApproval(a, d.Approval) || !nonce(challenge) || d.CheckNonce != challenge || !identifier(d.CredentialID, "hlac_") || !revision(d.CredentialRevision) ||
		e1 != nil || e2 != nil || e3 != nil || checked.Before(prepared) || !checked.Before(expires) || d.Decision != "PERMITTED" {
		return ErrInvalid
	}
	return nil
}
func sameApproval(a, b Approval) bool {
	// Pointers represent nullable VALUES, not identity of parser allocations.
	x, y := a, b
	x.PreviousPrepareID = nil
	y.PreviousPrepareID = nil
	x.CommittedACKDigest = nil
	y.CommittedACKDigest = nil
	return x == y && equalOptional(a.PreviousPrepareID, b.PreviousPrepareID) && equalOptional(a.CommittedACKDigest, b.CommittedACKDigest)
}
func DecodeFinalizeRequest(raw []byte) (FinalizeRequest, error) {
	var f FinalizeRequest
	_, err := fields(raw, "recovery_id", "prepare_id", "approval_revision", "remote_receipt_digest", "ack_digest")
	if err != nil || json.Unmarshal(raw, &f) != nil || !nonce(f.RecoveryID) || !nonce(f.PrepareID) || !approvalRevision(f.ApprovalRevision) || !hexValue(f.RemoteReceiptDigest, 64) || !hexValue(f.ACKDigest, 64) {
		return FinalizeRequest{}, ErrInvalid
	}
	return f, nil
}
func DecodeFinalization(raw []byte) (Finalization, error) {
	var f Finalization
	_, err := fields(raw, "recovery_id", "prepare_id", "owner_decision_id", "approval_revision", "action_id", "remote_receipt_digest", "ack_digest", "released_lane_revision", "finalized_at")
	if err != nil || json.Unmarshal(raw, &f) != nil {
		return Finalization{}, ErrInvalid
	}
	_, err = ParseHTTPTime(f.FinalizedAt)
	if err != nil || !nonce(f.RecoveryID) || !nonce(f.PrepareID) || !nonce(f.OwnerDecisionID) || !approvalRevision(f.ApprovalRevision) || !identifier(f.ActionID, "hla_") || !hexValue(f.RemoteReceiptDigest, 64) || !hexValue(f.ACKDigest, 64) || !revision(f.ReleasedLaneRevision) {
		return Finalization{}, ErrInvalid
	}
	return f, nil
}
func MatchFinalization(a Approval, ack string, f Finalization) error {
	when, e1 := ParseHTTPTime(f.FinalizedAt)
	prepared, e2 := ParseHTTPTime(a.PreparedAt)
	lane, err := strconv.ParseUint(a.LaneRevision, 10, 64)
	released, err2 := strconv.ParseUint(f.ReleasedLaneRevision, 10, 64)
	// Duplicate lookup may occur after expiry; compare the ORIGINAL receipt time.
	expires, e3 := ParseHTTPTime(a.ExpiresAt)
	if validateApproval(a) != nil || !hexValue(ack, 64) || f.RecoveryID != a.RecoveryID || f.PrepareID != a.PrepareID || f.OwnerDecisionID != a.OwnerDecisionID || f.ApprovalRevision != a.ApprovalRevision || f.ActionID != a.ActionID || f.RemoteReceiptDigest != a.RemoteReceiptDigest || f.ACKDigest != ack ||
		e1 != nil || e2 != nil || e3 != nil || when.Before(prepared) || !when.Before(expires) || err != nil || err2 != nil || !revision(f.ReleasedLaneRevision) || lane >= 9223372036854775807 || released != lane+1 {
		return ErrInvalid
	}
	return nil
}
func DecodeRecoveryView(raw []byte) (RecoveryView, error) {
	var v RecoveryView
	o, err := fields(raw, "action_id", "recovery_id", "remote_receipt_digest", "evidence_kind", "lane_revision", "state", "current_approval", "finalization", "remaining_approvals")
	if err != nil || json.Unmarshal(raw, &v) != nil || !identifier(v.ActionID, "hla_") || !revision(v.LaneRevision) || isNull(o["remaining_approvals"]) || v.RemainingApprovals < 0 || v.RemainingApprovals > 8 {
		return RecoveryView{}, ErrInvalid
	}
	if !isNull(o["current_approval"]) {
		a, e := DecodeApproval(o["current_approval"])
		if e != nil {
			return RecoveryView{}, ErrInvalid
		}
		v.CurrentApproval = &a
	}
	if !isNull(o["finalization"]) {
		f, e := DecodeFinalization(o["finalization"])
		if e != nil {
			return RecoveryView{}, ErrInvalid
		}
		v.Finalization = &f
	}
	if v.State == "NONE" {
		if v.RecoveryID != nil || v.RemoteReceiptDigest != nil || v.EvidenceKind != nil || v.CurrentApproval != nil || v.Finalization != nil || v.RemainingApprovals != 8 {
			return RecoveryView{}, ErrInvalid
		}
		return v, nil
	}
	if v.RecoveryID == nil || !nonce(*v.RecoveryID) || v.RemoteReceiptDigest == nil || !hexValue(*v.RemoteReceiptDigest, 64) || v.EvidenceKind == nil || !evidenceKind(*v.EvidenceKind) {
		return RecoveryView{}, ErrInvalid
	}
	if v.CurrentApproval != nil {
		a := v.CurrentApproval
		used, _ := strconv.Atoi(a.ApprovalRevision)
		if a.ActionID != v.ActionID || a.RecoveryID != *v.RecoveryID || a.RemoteReceiptDigest != *v.RemoteReceiptDigest || a.EvidenceKind != *v.EvidenceKind || v.RemainingApprovals != 8-used {
			return RecoveryView{}, ErrInvalid
		}
	}
	switch v.State {
	case "AWAITING_OWNER":
		if v.CurrentApproval != nil || v.Finalization != nil || v.RemainingApprovals != 8 {
			return RecoveryView{}, ErrInvalid
		}
	case "PREPARED":
		if v.CurrentApproval == nil || v.Finalization != nil || v.LaneRevision != v.CurrentApproval.LaneRevision {
			return RecoveryView{}, ErrInvalid
		}
	case "FINALIZED":
		if v.CurrentApproval == nil || v.Finalization == nil || MatchFinalization(*v.CurrentApproval, v.Finalization.ACKDigest, *v.Finalization) != nil || v.LaneRevision != v.Finalization.ReleasedLaneRevision {
			return RecoveryView{}, ErrInvalid
		}
	default:
		return RecoveryView{}, ErrInvalid
	}
	return v, nil
}
func MatchRecoveryView(r RequestReceipt, digest string, v RecoveryView) error {
	actual, err := ReceiptDigest(r)
	if err != nil || hex.EncodeToString(actual[:]) != digest || v.ActionID != r.ActionID || v.RemoteReceiptDigest != nil && *v.RemoteReceiptDigest != digest {
		return ErrInvalid
	}
	if a := v.CurrentApproval; a != nil && (a.ServerID != r.ServerID || a.OwnerUserID != r.OwnerUserID || a.ConnectorInstanceID != r.ConnectorInstanceID) {
		return ErrInvalid
	}
	return nil
}
