package actionprotocol

import (
	"bytes"
	"encoding/json"
	"strconv"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

const WorkerVersion = "observer-action-local/v1"
const AdminVersion = "observer-action-admin/v1"

type Envelope struct {
	Version           string          `json:"version"`
	Kind              string          `json:"kind"`
	ConversationNonce string          `json:"conversationNonce"`
	Ordinal           string          `json:"ordinal"`
	Payload           json.RawMessage `json:"payload"`
}

type LocalParameters struct {
	FormatVersion    int   `json:"formatVersion"`
	StopGraceSeconds Grace `json:"stopGraceSeconds"`
}
type LocalEntry struct {
	TargetID           string               `json:"targetId"`
	TargetKey          string               `json:"targetKey"`
	Operation          actioncore.Operation `json:"operation"`
	Parameters         LocalParameters      `json:"parameters"`
	ParameterSHA256    string               `json:"parameterSha256"`
	PolicyRevision     string               `json:"policyRevision"`
	CapabilityRevision string               `json:"capabilityRevision"`
}

func (e LocalEntry) HTTP() Entry {
	return Entry{e.TargetID, e.TargetKey, e.Operation, Parameters{e.Parameters.FormatVersion, e.Parameters.StopGraceSeconds}, e.ParameterSHA256, e.PolicyRevision, e.CapabilityRevision}
}
func LocalFromEntry(e Entry) LocalEntry {
	return LocalEntry{e.TargetID, e.TargetKey, e.Operation, LocalParameters{e.Parameters.FormatVersion, e.Parameters.StopGraceSeconds}, e.ParameterSHA256, e.PolicyRevision, e.CapabilityRevision}
}

type ApprovalSnapshot struct {
	ServerID            string       `json:"serverId"`
	OwnerUserID         string       `json:"ownerUserId"`
	ConnectorInstanceID string       `json:"connectorInstanceId"`
	Entries             []LocalEntry `json:"entries"`
}
type ClaimedExecute struct {
	Claim         Claim  `json:"claim"`
	ClaimRTTNanos string `json:"claimRttNanos"`
}
type RetainedLookup struct {
	ActionID            string `json:"action_id"`
	RemoteReceiptDigest string `json:"remote_receipt_digest"`
}
type RecoveryQuery struct {
	Operation           string          `json:"operation"`
	ActionID            string          `json:"action_id"`
	RemoteReceiptDigest string          `json:"remote_receipt_digest"`
	Body                json.RawMessage `json:"body"`
}
type RecoveryReply struct {
	Operation           string          `json:"operation"`
	ActionID            string          `json:"action_id"`
	RemoteReceiptDigest string          `json:"remote_receipt_digest"`
	Outcome             string          `json:"outcome"`
	Body                json.RawMessage `json:"body"`
}
type RecoveryInit struct {
	Request             RequestReceipt `json:"request"`
	RemoteReceiptDigest string         `json:"remote_receipt_digest"`
	MappingRevision     string         `json:"mapping_revision"`
}
type RecoveryStatus struct {
	Request             RequestReceipt `json:"request"`
	RemoteReceiptDigest string         `json:"remote_receipt_digest"`
}
type RecoveryContinue struct {
	Request             RequestReceipt `json:"request"`
	RemoteReceiptDigest string         `json:"remote_receipt_digest"`
	RecoveryID          string         `json:"recovery_id"`
	PrepareID           string         `json:"prepare_id"`
	ApprovalRevision    string         `json:"approval_revision"`
}
type RecoveryState struct {
	ActionID            string        `json:"action_id"`
	RemoteReceiptDigest string        `json:"remote_receipt_digest"`
	LocalState          string        `json:"local_state"`
	EvidenceKind        *string       `json:"evidence_kind"`
	MappingRevision     *string       `json:"mapping_revision"`
	ObservedState       *string       `json:"observed_state"`
	ObservedAt          *string       `json:"observed_at"`
	ACKDigest           *string       `json:"ack_digest"`
	Remote              *RecoveryView `json:"remote"`
}
type Refused struct {
	Code string `json:"code"`
}

func DecodeApprovalSnapshot(raw []byte) (ApprovalSnapshot, error) {
	var s ApprovalSnapshot
	o, err := fields(raw, "serverId", "ownerUserId", "connectorInstanceId", "entries")
	if err != nil || json.Unmarshal(raw, &s) != nil || !identifier(s.ServerID, "srv_") || !identifier(s.OwnerUserID, "usr_") || !identifier(s.ConnectorInstanceID, "agent_") || s.Entries == nil || len(s.Entries) > 32 {
		return ApprovalSnapshot{}, ErrInvalid
	}
	var entries []json.RawMessage
	if json.Unmarshal(o["entries"], &entries) != nil {
		return ApprovalSnapshot{}, ErrInvalid
	}
	http := make([]Entry, len(entries))
	for index, raw := range entries {
		e, err := fields(raw, "targetId", "targetKey", "operation", "parameters", "parameterSha256", "policyRevision", "capabilityRevision")
		if err != nil {
			return ApprovalSnapshot{}, ErrInvalid
		}
		if _, err = fields(e["parameters"], "formatVersion", "stopGraceSeconds"); err != nil {
			return ApprovalSnapshot{}, ErrInvalid
		}
		http[index] = s.Entries[index].HTTP()
	}
	if ValidateEntries(http) != nil {
		return ApprovalSnapshot{}, ErrInvalid
	}
	return s, nil
}
func DecodeClaimedExecute(raw []byte) (ClaimedExecute, error) {
	var e ClaimedExecute
	o, err := fields(raw, "claim", "claimRttNanos")
	if err != nil || json.Unmarshal(raw, &e) != nil {
		return ClaimedExecute{}, ErrInvalid
	}
	e.Claim, err = DecodeClaim(o["claim"])
	nanos, e2 := strconv.ParseInt(e.ClaimRTTNanos, 10, 64)
	if err != nil || e2 != nil || nanos < 0 || nanos > int64(30*time.Second) || strconv.FormatInt(nanos, 10) != e.ClaimRTTNanos {
		return ClaimedExecute{}, ErrInvalid
	}
	return e, nil
}
func DecodeRetainedLookup(raw []byte) (RetainedLookup, error) {
	var l RetainedLookup
	_, err := fields(raw, "action_id", "remote_receipt_digest")
	if err != nil || json.Unmarshal(raw, &l) != nil || !identifier(l.ActionID, "hla_") || !hexValue(l.RemoteReceiptDigest, 64) {
		return RetainedLookup{}, ErrInvalid
	}
	return l, nil
}
func canonicalRecoveryBody(operation, action, digest string, raw []byte, reply bool) (any, error) {
	if !identifier(action, "hla_") || !hexValue(digest, 64) {
		return nil, ErrInvalid
	}
	if !reply {
		switch operation {
		case "RETAINED", "STATUS":
			if isNull(raw) {
				return nil, nil
			}
		case "PROPOSE":
			p, e := DecodeRecoveryProposal(raw)
			if e == nil && p.RemoteReceiptDigest == digest {
				return p, nil
			}
		case "CHECK":
			c, e := DecodeRecoveryCheck(raw)
			if e == nil && c.RemoteReceiptDigest == digest {
				return c, nil
			}
		case "FINALIZE":
			f, e := DecodeFinalizeRequest(raw)
			if e == nil && f.RemoteReceiptDigest == digest {
				return f, nil
			}
		}
		return nil, ErrInvalid
	}
	switch operation {
	case "RETAINED":
		s, e := DecodeRetainedStatus(raw)
		if e == nil && s.ActionID == action && s.Request != nil {
			d, e := ReceiptDigest(*s.Request)
			if e == nil && digestString(d) == digest {
				return s, nil
			}
		}
	case "PROPOSE", "STATUS":
		v, e := DecodeRecoveryView(raw)
		if e == nil && v.ActionID == action && (v.RemoteReceiptDigest == nil || *v.RemoteReceiptDigest == digest) {
			return v, nil
		}
	case "CHECK":
		d, e := DecodeRecoveryDecision(raw)
		if e == nil && d.Approval.ActionID == action && d.Approval.RemoteReceiptDigest == digest {
			return d, nil
		}
	case "FINALIZE":
		f, e := DecodeFinalization(raw)
		if e == nil && f.ActionID == action && f.RemoteReceiptDigest == digest {
			return f, nil
		}
	}
	return nil, ErrInvalid
}
func digestString(d [32]byte) string {
	const digits = "0123456789abcdef"
	var result [64]byte
	for i, b := range d {
		result[2*i] = digits[b>>4]
		result[2*i+1] = digits[b&15]
	}
	return string(result[:])
}
func DecodeRecoveryQuery(raw []byte) (RecoveryQuery, error) {
	var q RecoveryQuery
	_, err := fields(raw, "operation", "action_id", "remote_receipt_digest", "body")
	if err != nil || json.Unmarshal(raw, &q) != nil {
		return RecoveryQuery{}, ErrInvalid
	}
	body, err := canonicalRecoveryBody(q.Operation, q.ActionID, q.RemoteReceiptDigest, q.Body, false)
	if err != nil {
		return RecoveryQuery{}, ErrInvalid
	}
	q.Body, _ = json.Marshal(body)
	return q, nil
}
func DecodeRecoveryReply(raw []byte) (RecoveryReply, error) {
	var r RecoveryReply
	_, err := fields(raw, "operation", "action_id", "remote_receipt_digest", "outcome", "body")
	if err != nil || json.Unmarshal(raw, &r) != nil || !identifier(r.ActionID, "hla_") || !hexValue(r.RemoteReceiptDigest, 64) {
		return RecoveryReply{}, ErrInvalid
	}
	if r.Operation != "RETAINED" && r.Operation != "STATUS" && r.Operation != "PROPOSE" && r.Operation != "CHECK" && r.Operation != "FINALIZE" {
		return RecoveryReply{}, ErrInvalid
	}
	if r.Outcome != "OK" {
		if r.Outcome != "DENIED" && r.Outcome != "UNAVAILABLE" && r.Outcome != "CONFLICT" && r.Outcome != "FULL" || !isNull(r.Body) {
			return RecoveryReply{}, ErrInvalid
		}
		r.Body = json.RawMessage("null")
		return r, nil
	}
	body, err := canonicalRecoveryBody(r.Operation, r.ActionID, r.RemoteReceiptDigest, r.Body, true)
	if err != nil {
		return RecoveryReply{}, ErrInvalid
	}
	r.Body, _ = json.Marshal(body)
	return r, nil
}
func decodeAdminCommand(kind string, raw []byte) (any, error) {
	names := []string{"request", "remote_receipt_digest"}
	if kind == "RECOVERY_INIT" {
		names = append(names, "mapping_revision")
	}
	if kind == "RECOVERY_CONTINUE" {
		names = append(names, "recovery_id", "prepare_id", "approval_revision")
	}
	o, err := fields(raw, names...)
	if err != nil {
		return nil, ErrInvalid
	}
	r, err := DecodeRequest(o["request"])
	var digest string
	if err != nil || json.Unmarshal(o["remote_receipt_digest"], &digest) != nil {
		return nil, ErrInvalid
	}
	actual, err := ReceiptDigest(r)
	if err != nil || digestString(actual) != digest {
		return nil, ErrInvalid
	}
	switch kind {
	case "RECOVERY_INIT":
		var c RecoveryInit
		if json.Unmarshal(raw, &c) == nil && revision(c.MappingRevision) {
			return c, nil
		}
	case "RECOVERY_STATUS":
		return RecoveryStatus{r, digest}, nil
	case "RECOVERY_CONTINUE":
		var c RecoveryContinue
		if json.Unmarshal(raw, &c) == nil && nonce(c.RecoveryID) && nonce(c.PrepareID) && approvalRevision(c.ApprovalRevision) {
			return c, nil
		}
	}
	return nil, ErrInvalid
}
func DecodeRecoveryState(raw []byte) (RecoveryState, error) {
	var s RecoveryState
	o, err := fields(raw, "action_id", "remote_receipt_digest", "local_state", "evidence_kind", "mapping_revision", "observed_state", "observed_at", "ack_digest", "remote")
	if err != nil || json.Unmarshal(raw, &s) != nil || !identifier(s.ActionID, "hla_") || !hexValue(s.RemoteReceiptDigest, 64) ||
		s.EvidenceKind != nil && !evidenceKind(*s.EvidenceKind) || s.MappingRevision != nil && !revision(*s.MappingRevision) || !optionalDigest(s.ACKDigest) || (s.ObservedAt == nil) != (s.ObservedState == nil) {
		return RecoveryState{}, ErrInvalid
	}
	if s.ObservedAt != nil {
		at, err := time.Parse("2006-01-02T15:04:05.000000000Z", *s.ObservedAt)
		if err != nil || at.Format("2006-01-02T15:04:05.000000000Z") != *s.ObservedAt || *s.ObservedState != "CREATED" && *s.ObservedState != "RUNNING" && *s.ObservedState != "EXITED" {
			return RecoveryState{}, ErrInvalid
		}
	}
	if !isNull(o["remote"]) {
		v, e := DecodeRecoveryView(o["remote"])
		if e != nil || v.ActionID != s.ActionID || v.RemoteReceiptDigest != nil && *v.RemoteReceiptDigest != s.RemoteReceiptDigest {
			return RecoveryState{}, ErrInvalid
		}
		s.Remote = &v
	}
	switch s.LocalState {
	case "UNPREPARED", "RECOVERY_REQUIRED":
	case "QUIESCED":
		if s.EvidenceKind == nil || s.MappingRevision == nil || s.ObservedState == nil || s.ACKDigest != nil {
			return RecoveryState{}, ErrInvalid
		}
	case "ACKED":
		if s.EvidenceKind == nil || s.MappingRevision == nil || s.ObservedState == nil || s.ACKDigest == nil {
			return RecoveryState{}, ErrInvalid
		}
	default:
		return RecoveryState{}, ErrInvalid
	}
	return s, nil
}

func canonicalPayload(e Envelope) (any, error) {
	if e.Version == AdminVersion {
		switch e.Kind {
		case "HELLO":
			if e.Ordinal == "1" && bytes.Equal(e.Payload, []byte("{}")) {
				return struct{}{}, nil
			}
		case "RECOVERY_INIT", "RECOVERY_STATUS", "RECOVERY_CONTINUE":
			if e.Ordinal == "2" {
				return decodeAdminCommand(e.Kind, e.Payload)
			}
		case "RECOVERY_STATE":
			if e.Ordinal == "3" {
				return DecodeRecoveryState(e.Payload)
			}
		case "REFUSED":
			var r Refused
			_, err := fields(e.Payload, "code")
			if e.Ordinal != "3" || err != nil || json.Unmarshal(e.Payload, &r) != nil {
				return nil, ErrInvalid
			}
			switch r.Code {
			case "INVALID", "BUSY", "BINDING_MISMATCH", "MAPPING_UNAVAILABLE", "NOT_QUIESCED", "INSPECTION_UNAVAILABLE", "REMOTE_UNAVAILABLE", "OWNER_REQUIRED", "FULL", "RECOVERY_REQUIRED":
				return r, nil
			}
		}
		return nil, ErrInvalid
	}
	if e.Version != WorkerVersion {
		return nil, ErrInvalid
	}
	switch e.Kind {
	case "BEGIN":
		if e.Ordinal == "1" && e.ConversationNonce == "00000000000000000000000000000000" && bytes.Equal(e.Payload, []byte("{}")) {
			return struct{}{}, nil
		}
	case "APPROVAL_SNAPSHOT":
		if e.Ordinal == "1" {
			return DecodeApprovalSnapshot(e.Payload)
		}
	case "CLAIMED_EXECUTE":
		if e.Ordinal == "2" {
			return DecodeClaimedExecute(e.Payload)
		}
	case "NO_CLAIM":
		if e.Ordinal == "2" && bytes.Equal(e.Payload, []byte("{}")) {
			return struct{}{}, nil
		}
	case "AUTHORITY_CHALLENGE":
		if e.Ordinal == "3" || e.Ordinal == "4" {
			return DecodeCheckRequest(e.Payload)
		}
	case "AUTHORITY_REPLY":
		if e.Ordinal == "3" || e.Ordinal == "4" {
			return DecodeExecutionDecision(e.Payload)
		}
	case "RESULT":
		if e.Ordinal == "5" {
			return DecodeReport(e.Payload)
		}
	case "RETAINED_LOOKUP":
		if e.Ordinal == "1" {
			return DecodeRetainedLookup(e.Payload)
		}
	case "RECOVERY_QUERY":
		if e.Ordinal >= "1" && e.Ordinal <= "5" && len(e.Ordinal) == 1 {
			return DecodeRecoveryQuery(e.Payload)
		}
	case "RECOVERY_REPLY":
		if e.Ordinal >= "1" && e.Ordinal <= "5" && len(e.Ordinal) == 1 {
			return DecodeRecoveryReply(e.Payload)
		}
	case "SNAPSHOT_QUERY":
		if e.Ordinal == "1" {
			q, err := DecodeSnapshotQuery(e.Payload)
			if err == nil && q.PublicationID == e.ConversationNonce {
				return q, nil
			}
		}
	case "SNAPSHOT_REPLY":
		if e.Ordinal == "2" {
			r, err := DecodeSnapshotReply(e.Payload)
			if err == nil && (r.Receipt == nil || r.Receipt.PublicationID == e.ConversationNonce) {
				return r, nil
			}
		}
	}
	return nil, ErrInvalid
}
func EncodeEnvelope(e Envelope) ([]byte, error) {
	if !nonce(e.ConversationNonce) && !(e.Version == WorkerVersion && e.Kind == "BEGIN" && e.ConversationNonce == "00000000000000000000000000000000") {
		return nil, ErrInvalid
	}
	payload, err := canonicalPayload(e)
	if err != nil {
		return nil, ErrInvalid
	}
	e.Payload, err = json.Marshal(payload)
	if err != nil {
		return nil, ErrInvalid
	}
	raw, err := json.Marshal(e)
	if err != nil || strictJSON(raw) != nil {
		return nil, ErrInvalid
	}
	return raw, nil
}
func DecodeEnvelope(raw []byte) (Envelope, error) {
	var e Envelope
	if _, err := fields(raw, "version", "kind", "conversationNonce", "ordinal", "payload"); err != nil || json.Unmarshal(raw, &e) != nil {
		return Envelope{}, ErrInvalid
	}
	canonical, err := EncodeEnvelope(e)
	if err != nil || !bytes.Equal(raw, canonical) {
		return Envelope{}, ErrInvalid
	}
	return e, nil
}
