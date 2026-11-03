package actionprotocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

const HTTPTimeLayout = "2006-01-02T15:04:05.000000Z"

type Grace struct {
	Seconds uint8
	Null    bool
}

func (g Grace) MarshalJSON() ([]byte, error) {
	if g.Null && g.Seconds == 0 {
		return []byte("null"), nil
	}
	if g.Null || g.Seconds < 1 || g.Seconds > 30 {
		return nil, ErrInvalid
	}
	return []byte(strconv.Itoa(int(g.Seconds))), nil
}

func (g *Grace) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if bytes.Equal(raw, []byte("null")) {
		*g = Grace{Null: true}
		return nil
	}
	n, err := strconv.ParseUint(string(raw), 10, 8)
	if err != nil || n < 1 || n > 30 || strconv.FormatUint(n, 10) != string(raw) {
		return ErrInvalid
	}
	*g = Grace{Seconds: uint8(n)}
	return nil
}

type Parameters struct {
	FormatVersion    int   `json:"format_version"`
	StopGraceSeconds Grace `json:"stop_grace_seconds"`
}

// RequestReceipt's field order is the immutable full-sixteen identity preimage.
// Private Engine identity and the distinct core fingerprint never appear here.
type RequestReceipt struct {
	ActionID            string               `json:"action_id"`
	ServerID            string               `json:"server_id"`
	OwnerUserID         string               `json:"owner_user_id"`
	ActorUserID         string               `json:"actor_user_id"`
	ConnectorInstanceID string               `json:"connector_instance_id"`
	Operation           actioncore.Operation `json:"operation"`
	TargetID            string               `json:"target_id"`
	TargetKey           string               `json:"target_key"`
	IdempotencyKeyHash  string               `json:"idempotency_key_hash"`
	PolicyRevision      string               `json:"policy_revision"`
	CapabilityRevision  string               `json:"capability_revision"`
	Parameters          Parameters           `json:"parameters"`
	ParameterSHA256     string               `json:"parameter_sha256"`
	RequestedAt         string               `json:"requested_at"`
	ExpiresAt           string               `json:"expires_at"`
	CorrelationID       string               `json:"correlation_id"`
}

func hexValue(s string, length int) bool {
	if len(s) != length {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func identifier(s, prefix string) bool {
	return strings.HasPrefix(s, prefix) && hexValue(strings.TrimPrefix(s, prefix), 32)
}

func revision(s string) bool {
	if len(s) == 0 || len(s) > 19 || s[0] < '1' || s[0] > '9' {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return err == nil && n <= math.MaxInt64
}

func ParseHTTPTime(s string) (time.Time, error) {
	t, err := time.Parse(HTTPTimeLayout, s)
	if err != nil || len(s) != 27 || t.IsZero() || t.UTC().Format(HTTPTimeLayout) != s {
		return time.Time{}, ErrInvalid
	}
	return t, nil
}

func correlation(s string) bool {
	if len(s) < 1 || len(s) > 128 {
		return false
	}
	for index, c := range s {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			continue
		}
		if index == 0 || !strings.ContainsRune("._:-", c) {
			return false
		}
	}
	return true
}

func targetKey(s string) bool {
	if len(s) == 0 || len(s) > 48 || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func ParameterDigest(operation actioncore.Operation, p Parameters) (string, error) {
	if p.FormatVersion != 1 {
		return "", ErrInvalid
	}
	preimage := `{"version":1,"operation":"` + string(operation) + `","parameters":`
	switch operation {
	case actioncore.Start:
		if !p.StopGraceSeconds.Null || p.StopGraceSeconds.Seconds != 0 {
			return "", ErrInvalid
		}
		preimage += `{}}`
	case actioncore.Stop, actioncore.Restart:
		if p.StopGraceSeconds.Null || p.StopGraceSeconds.Seconds < 1 || p.StopGraceSeconds.Seconds > 30 {
			return "", ErrInvalid
		}
		preimage += `{"stop_grace_seconds":` + strconv.Itoa(int(p.StopGraceSeconds.Seconds)) + `}}`
	default:
		return "", ErrInvalid
	}
	digest := sha256.Sum256([]byte(preimage))
	return hex.EncodeToString(digest[:]), nil
}

func ValidateRequest(r RequestReceipt) error {
	if !identifier(r.ActionID, "hla_") || !identifier(r.ServerID, "srv_") || !identifier(r.OwnerUserID, "usr_") ||
		r.ActorUserID != r.OwnerUserID || !identifier(r.ConnectorInstanceID, "agent_") || !identifier(r.TargetID, "hlt_") ||
		!targetKey(r.TargetKey) || !hexValue(r.IdempotencyKeyHash, 64) || !revision(r.PolicyRevision) ||
		!revision(r.CapabilityRevision) || !hexValue(r.ParameterSHA256, 64) || !correlation(r.CorrelationID) {
		return ErrInvalid
	}
	created, e1 := ParseHTTPTime(r.RequestedAt)
	expires, e2 := ParseHTTPTime(r.ExpiresAt)
	expected, e3 := ParameterDigest(r.Operation, r.Parameters)
	if e1 != nil || e2 != nil || e3 != nil || expires.Sub(created) != 300*time.Second || expected != r.ParameterSHA256 {
		return ErrInvalid
	}
	return nil
}

func DecodeRequest(raw []byte) (RequestReceipt, error) {
	var r RequestReceipt
	if strictJSON(raw) != nil {
		return r, ErrInvalid
	}
	object, err := objectFields(raw, "action_id", "server_id", "owner_user_id", "actor_user_id", "connector_instance_id",
		"operation", "target_id", "target_key", "idempotency_key_hash", "policy_revision", "capability_revision",
		"parameters", "parameter_sha256", "requested_at", "expires_at", "correlation_id")
	if err != nil {
		return r, ErrInvalid
	}
	if _, err = objectFields(object["parameters"], "format_version", "stop_grace_seconds"); err != nil ||
		json.Unmarshal(raw, &r) != nil || ValidateRequest(r) != nil {
		return RequestReceipt{}, ErrInvalid
	}
	return r, nil
}

func CanonicalRequest(r RequestReceipt) ([]byte, error) {
	if ValidateRequest(r) != nil {
		return nil, ErrInvalid
	}
	raw, err := json.Marshal(r)
	if err != nil || len(raw) > MaxEnvelopeBytes {
		return nil, ErrInvalid
	}
	return raw, nil
}

func ReceiptDigest(r RequestReceipt) ([32]byte, error) {
	raw, err := CanonicalRequest(r)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(append([]byte("platform-container-receipt/v1\n"), raw...)), nil
}
