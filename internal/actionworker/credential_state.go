package actionworker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/braidenm/home-lab-observer/internal/actionremote"
)

type handoffPhase string

const (
	phaseUnenrolled        handoffPhase = "UNENROLLED"
	phaseExchangeAttempted handoffPhase = "EXCHANGE_ATTEMPTED"
	phaseReady             handoffPhase = "READY"
	phasePrepareAttempted  handoffPhase = "PREPARE_ATTEMPTED"
	phasePrepared          handoffPhase = "PREPARED"
	phaseActivateAttempted handoffPhase = "ACTIVATE_ATTEMPTED"
)
const credentialTime = "2006-01-02T15:04:05.000000Z"

// Private file representations: their only JSON encoder is the bounded store
// codec. They must never become diagnostic/HTTP/event DTOs.
type activeRecord struct {
	Version             string                       `json:"version"`
	ServerID            string                       `json:"server_id"`
	OwnerUserID         string                       `json:"owner_user_id"`
	ConnectorInstanceID string                       `json:"connector_instance_id"`
	CredentialID        string                       `json:"credential_id"`
	Revision            string                       `json:"credential_revision"`
	State               actionremote.CredentialState `json:"state"`
	ExpiresAt           string                       `json:"expires_at"`
	AuthorizationSecret string                       `json:"authorization_secret"`
}

func (activeRecord) String() string   { return "ActionActiveRecord([redacted])" }
func (activeRecord) GoString() string { return "ActionActiveRecord([redacted])" }

type candidateRecord struct {
	CredentialID        string                       `json:"credential_id"`
	Revision            string                       `json:"revision"`
	State               actionremote.CredentialState `json:"state"`
	ExpiresAt           string                       `json:"expires_at"`
	AuthorizationSecret string                       `json:"authorization_secret"`
}

func (candidateRecord) String() string   { return "ActionCandidate([redacted])" }
func (candidateRecord) GoString() string { return "ActionCandidate([redacted])" }

type handoffDocument struct {
	Version                  string           `json:"version"`
	ServerID                 string           `json:"server_id"`
	OwnerUserID              string           `json:"owner_user_id"`
	ConnectorInstanceID      string           `json:"connector_instance_id"`
	Phase                    handoffPhase     `json:"phase"`
	ActiveCredentialID       *string          `json:"active_credential_id"`
	ActiveCredentialRevision *string          `json:"active_credential_revision"`
	ActiveSHA256             *string          `json:"active_sha256"`
	PredecessorSecret        *string          `json:"predecessor_secret"`
	PredecessorExpiresAt     *string          `json:"predecessor_expires_at"`
	Candidate                *candidateRecord `json:"candidate"`
}

func (handoffDocument) String() string   { return "ActionHandoff([redacted])" }
func (handoffDocument) GoString() string { return "ActionHandoff([redacted])" }

func freshHandoff(b actionremote.Binding) handoffDocument {
	return handoffDocument{Version: "observer-action-handoff/v1", ServerID: b.ServerID, OwnerUserID: b.OwnerUserID, ConnectorInstanceID: b.ConnectorInstanceID, Phase: phaseUnenrolled}
}
func positiveRevision(s string) bool {
	n, err := strconv.ParseInt(s, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == s
}
func credentialExpiry(s string) (time.Time, error) {
	at, err := time.Parse(credentialTime, s)
	if err != nil || at.Year() < 1 || at.Format(credentialTime) != s {
		return time.Time{}, ErrUnsafe
	}
	return at, nil
}
func validCandidate(c candidateRecord) bool {
	_, err := actionremote.NewCredential(c.AuthorizationSecret)
	_, expiry := credentialExpiry(c.ExpiresAt)
	return err == nil && pairedID(c.CredentialID, "hlac_") && strings.HasPrefix(c.AuthorizationSecret, c.CredentialID+".") && positiveRevision(c.Revision) && expiry == nil && (c.State == actionremote.CredentialActive || c.State == actionremote.CredentialPrepared)
}
func candidateFromDisclosure(d actionremote.CredentialDisclosure) (candidateRecord, error) {
	if d.Metadata.Revision <= 0 || d.Metadata.ExpiresAt.Year() < 1 || d.Metadata.ExpiresAt.Year() > 9999 || d.Metadata.ExpiresAt.Nanosecond()%1000 != 0 {
		return candidateRecord{}, ErrUnsafe
	}
	c := candidateRecord{d.Metadata.CredentialID, strconv.FormatInt(d.Metadata.Revision, 10), d.Metadata.State, d.Metadata.ExpiresAt.UTC().Format(credentialTime), d.AuthorizationSecret}
	if !validCandidate(c) {
		return candidateRecord{}, ErrUnsafe
	}
	return c, nil
}
func candidateActiveRecord(c candidateRecord, b actionremote.Binding) (activeRecord, error) {
	if !validCandidate(c) || c.State != actionremote.CredentialActive || !pairedID(b.ServerID, "srv_") || !pairedID(b.OwnerUserID, "usr_") || !pairedID(b.ConnectorInstanceID, "agent_") {
		return activeRecord{}, ErrUnsafe
	}
	return activeRecord{"observer-action-credential/v1", b.ServerID, b.OwnerUserID, b.ConnectorInstanceID, c.CredentialID, c.Revision, c.State, c.ExpiresAt, c.AuthorizationSecret}, nil
}
func candidateActiveBytes(c candidateRecord, b actionremote.Binding) ([]byte, error) {
	a, err := candidateActiveRecord(c, b)
	if err != nil {
		return nil, err
	}
	return json.Marshal(a)
}
func activeCandidate(a activeRecord) candidateRecord {
	return candidateRecord{a.CredentialID, a.Revision, a.State, a.ExpiresAt, a.AuthorizationSecret}
}
func decodeActiveRecord(raw []byte, b actionremote.Binding) (activeRecord, error) {
	var a activeRecord
	if !boundedCredentialJSON(raw) || json.Unmarshal(raw, &a) != nil || a.Version != "observer-action-credential/v1" || a.ServerID != b.ServerID || a.OwnerUserID != b.OwnerUserID || a.ConnectorInstanceID != b.ConnectorInstanceID {
		return activeRecord{}, ErrUnsafe
	}
	canonical, err := candidateActiveBytes(activeCandidate(a), b)
	if err != nil || !bytes.Equal(raw, canonical) {
		return activeRecord{}, ErrUnsafe
	}
	return a, nil
}
func credentialSHA(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func readyHandoff(b actionremote.Binding, a activeRecord) (handoffDocument, error) {
	raw, err := candidateActiveBytes(activeCandidate(a), b)
	if err != nil || a.ServerID != b.ServerID || a.OwnerUserID != b.OwnerUserID || a.ConnectorInstanceID != b.ConnectorInstanceID || a.Version != "observer-action-credential/v1" {
		return handoffDocument{}, ErrUnsafe
	}
	h := freshHandoff(b)
	h.Phase = phaseReady
	id, revision, hash := a.CredentialID, a.Revision, credentialSHA(raw)
	h.ActiveCredentialID = &id
	h.ActiveCredentialRevision = &revision
	h.ActiveSHA256 = &hash
	return h, nil
}
func predecessorRecord(h handoffDocument, b actionremote.Binding) (activeRecord, error) {
	if h.ActiveCredentialID == nil || h.ActiveCredentialRevision == nil || h.ActiveSHA256 == nil || h.PredecessorSecret == nil || h.PredecessorExpiresAt == nil {
		return activeRecord{}, ErrUnsafe
	}
	a, err := candidateActiveRecord(candidateRecord{*h.ActiveCredentialID, *h.ActiveCredentialRevision, actionremote.CredentialActive, *h.PredecessorExpiresAt, *h.PredecessorSecret}, b)
	if err != nil {
		return activeRecord{}, ErrUnsafe
	}
	raw, _ := json.Marshal(a)
	if credentialSHA(raw) != *h.ActiveSHA256 {
		return activeRecord{}, ErrUnsafe
	}
	return a, nil
}
func validateHandoff(h handoffDocument, b actionremote.Binding) error {
	if h.Version != "observer-action-handoff/v1" || h.ServerID != b.ServerID || h.OwnerUserID != b.OwnerUserID || h.ConnectorInstanceID != b.ConnectorInstanceID || !pairedID(b.ServerID, "srv_") || !pairedID(b.OwnerUserID, "usr_") || !pairedID(b.ConnectorInstanceID, "agent_") {
		return ErrUnsafe
	}
	count := 0
	for _, v := range []*string{h.ActiveCredentialID, h.ActiveCredentialRevision, h.ActiveSHA256} {
		if v != nil {
			count++
		}
	}
	if count != 0 && count != 3 {
		return ErrUnsafe
	}
	if count == 3 {
		decoded, err := hex.DecodeString(*h.ActiveSHA256)
		if !pairedID(*h.ActiveCredentialID, "hlac_") || !positiveRevision(*h.ActiveCredentialRevision) || err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != *h.ActiveSHA256 {
			return ErrUnsafe
		}
	}
	if h.Candidate != nil && !validCandidate(*h.Candidate) {
		return ErrUnsafe
	}
	initial := h.Phase == phaseUnenrolled || h.Phase == phaseExchangeAttempted
	if initial || h.Phase == phaseReady {
		if h.PredecessorSecret != nil || h.PredecessorExpiresAt != nil {
			return ErrUnsafe
		}
	} else if _, err := predecessorRecord(h, b); err != nil {
		return ErrUnsafe
	}
	switch h.Phase {
	case phaseUnenrolled:
		if count != 0 || h.Candidate != nil {
			return ErrUnsafe
		}
	case phaseExchangeAttempted:
		if count != 0 || h.Candidate != nil && h.Candidate.State != actionremote.CredentialActive {
			return ErrUnsafe
		}
	case phaseReady:
		if count != 3 || h.Candidate != nil {
			return ErrUnsafe
		}
	case phasePrepareAttempted:
		if count != 3 || h.Candidate != nil {
			return ErrUnsafe
		}
	case phasePrepared, phaseActivateAttempted:
		if count != 3 || h.Candidate == nil || h.Phase == phasePrepared && h.Candidate.State != actionremote.CredentialPrepared {
			return ErrUnsafe
		}
		old, _ := strconv.ParseInt(*h.ActiveCredentialRevision, 10, 64)
		next, _ := strconv.ParseInt(h.Candidate.Revision, 10, 64)
		if next <= old || h.Candidate.CredentialID == *h.ActiveCredentialID {
			return ErrUnsafe
		}
	default:
		return ErrUnsafe
	}
	return nil
}
func encodeHandoff(h handoffDocument, b actionremote.Binding) ([]byte, error) {
	if validateHandoff(h, b) != nil {
		return nil, ErrUnsafe
	}
	raw, err := json.Marshal(h)
	if err != nil || len(raw) > documentLimit {
		return nil, ErrUnsafe
	}
	return raw, nil
}
func decodeHandoff(raw []byte, b actionremote.Binding) (handoffDocument, error) {
	var h handoffDocument
	if !boundedCredentialJSON(raw) || json.Unmarshal(raw, &h) != nil {
		return handoffDocument{}, ErrUnsafe
	}
	canonical, err := encodeHandoff(h, b)
	if err != nil || !bytes.Equal(raw, canonical) {
		return handoffDocument{}, ErrUnsafe
	}
	return h, nil
}
func validateActiveWitness(h handoffDocument, raw []byte, b actionremote.Binding) error {
	if validateHandoff(h, b) != nil {
		return ErrUnsafe
	}
	if h.Phase == phaseUnenrolled || h.Phase == phaseExchangeAttempted {
		if len(raw) == 0 {
			return nil
		}
	} else if len(raw) == 0 {
		return ErrUnsafe
	}
	if len(raw) > 0 {
		a, err := decodeActiveRecord(raw, b)
		if err != nil {
			return ErrUnsafe
		}
		if h.ActiveSHA256 != nil && h.ActiveCredentialID != nil && h.ActiveCredentialRevision != nil && credentialSHA(raw) == *h.ActiveSHA256 && a.CredentialID == *h.ActiveCredentialID && a.Revision == *h.ActiveCredentialRevision {
			return nil
		}
		if (h.Phase == phaseExchangeAttempted || h.Phase == phaseActivateAttempted) && h.Candidate != nil && h.Candidate.State == actionremote.CredentialActive {
			expected, err := candidateActiveBytes(*h.Candidate, b)
			if err == nil && bytes.Equal(expected, raw) {
				return nil
			}
		}
	}
	return ErrUnsafe
}
func boundedCredentialJSON(raw []byte) bool {
	if len(raw) == 0 || len(raw) > documentLimit || !utf8.Valid(raw) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	depth := 0
	for {
		token, err := d.Token()
		if err == io.EOF {
			return depth == 0
		}
		if err != nil {
			return false
		}
		switch token {
		case json.Delim('{'), json.Delim('['):
			depth++
			if depth > 8 {
				return false
			}
		case json.Delim('}'), json.Delim(']'):
			depth--
		}
	}
}
