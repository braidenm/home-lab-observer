package actionprotocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

type Entry struct {
	TargetID           string               `json:"target_id"`
	TargetKey          string               `json:"target_key"`
	Operation          actioncore.Operation `json:"operation"`
	Parameters         Parameters           `json:"parameters"`
	ParameterSHA256    string               `json:"parameter_sha256"`
	PolicyRevision     string               `json:"policy_revision"`
	CapabilityRevision string               `json:"capability_revision"`
}

type Publication struct {
	PublicationID            string  `json:"publication_id"`
	ExpectedSnapshotRevision *string `json:"expected_snapshot_revision"`
	Entries                  []Entry `json:"entries"`
}

type SnapshotReceipt struct {
	PublicationID    string `json:"publication_id"`
	SnapshotRevision string `json:"snapshot_revision"`
	SnapshotDigest   string `json:"snapshot_digest"`
	AdvertisedAt     string `json:"advertised_at"`
	ExpiresAt        string `json:"expires_at"`
}

type EntryView struct {
	Entry
	CooldownUntil *string `json:"cooldown_until"`
}

type SnapshotView struct {
	ServerID            string      `json:"server_id"`
	OwnerUserID         string      `json:"owner_user_id"`
	ConnectorInstanceID string      `json:"connector_instance_id"`
	Readiness           string      `json:"readiness"`
	SnapshotRevision    *string     `json:"snapshot_revision"`
	SnapshotDigest      *string     `json:"snapshot_digest"`
	AdvertisedAt        *string     `json:"advertised_at"`
	ExpiresAt           *string     `json:"expires_at"`
	Entries             []EntryView `json:"entries"`
}

func entryFields(view bool) []string {
	names := []string{"target_id", "target_key", "operation", "parameters", "parameter_sha256", "policy_revision", "capability_revision"}
	if view {
		names = append(names, "cooldown_until")
	}
	return names
}

func validEntry(e Entry) bool {
	hash, err := ParameterDigest(e.Operation, e.Parameters)
	return err == nil && hash == e.ParameterSHA256 && identifier(e.TargetID, "hlt_") &&
		len(e.TargetKey) == 20 && strings.HasPrefix(e.TargetKey, "ctr_") && hexValue(e.TargetKey[4:], 16) &&
		revision(e.PolicyRevision) && revision(e.CapabilityRevision)
}

// ValidateEntries never sorts, truncates or otherwise changes broker approval.
func ValidateEntries(entries []Entry) error {
	if entries == nil || len(entries) > 32 {
		return ErrInvalid
	}
	aliases, targets := map[string]string{}, map[string]string{}
	previous := ""
	for _, entry := range entries {
		key := entry.TargetID + "\x00" + string(entry.Operation)
		if !validEntry(entry) || key <= previous {
			return ErrInvalid
		}
		if owner, exists := aliases[entry.TargetKey]; exists && owner != entry.TargetID {
			return ErrInvalid
		}
		if alias, exists := targets[entry.TargetID]; exists && alias != entry.TargetKey {
			return ErrInvalid
		}
		aliases[entry.TargetKey], targets[entry.TargetID], previous = entry.TargetID, entry.TargetKey, key
	}
	return nil
}

func decodeEntry(raw []byte, view bool) (EntryView, error) {
	object, err := fields(raw, entryFields(view)...)
	var e EntryView
	if err != nil {
		return e, ErrInvalid
	}
	if _, err = objectFields(object["parameters"], "format_version", "stop_grace_seconds"); err != nil || json.Unmarshal(raw, &e) != nil || !validEntry(e.Entry) {
		return EntryView{}, ErrInvalid
	}
	if view && e.CooldownUntil != nil {
		if _, err = ParseHTTPTime(*e.CooldownUntil); err != nil {
			return EntryView{}, ErrInvalid
		}
	}
	return e, nil
}

func DecodeEntry(raw []byte) (Entry, error) {
	e, err := decodeEntry(raw, false)
	return e.Entry, err
}

func decodeEntries(raw []byte) ([]Entry, error) {
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil || values == nil || len(values) > 32 {
		return nil, ErrInvalid
	}
	entries := make([]Entry, 0, len(values))
	for _, value := range values {
		e, err := DecodeEntry(value)
		if err != nil {
			return nil, ErrInvalid
		}
		entries = append(entries, e)
	}
	return entries, ValidateEntries(entries)
}

func DecodePublication(raw []byte) (Publication, error) {
	var p Publication
	object, err := fields(raw, "publication_id", "expected_snapshot_revision", "entries")
	if err != nil || json.Unmarshal(raw, &p) != nil || !hexValue(p.PublicationID, 32) ||
		(p.ExpectedSnapshotRevision != nil && !revision(*p.ExpectedSnapshotRevision)) {
		return Publication{}, ErrInvalid
	}
	p.Entries, err = decodeEntries(object["entries"])
	if err != nil {
		return Publication{}, ErrInvalid
	}
	return p, nil
}

func validSnapshotReceipt(r SnapshotReceipt) bool {
	start, e1 := ParseHTTPTime(r.AdvertisedAt)
	end, e2 := ParseHTTPTime(r.ExpiresAt)
	return hexValue(r.PublicationID, 32) && revision(r.SnapshotRevision) && hexValue(r.SnapshotDigest, 64) &&
		e1 == nil && e2 == nil && end.Sub(start) == 60*time.Second
}

func DecodeSnapshotReceipt(raw []byte) (SnapshotReceipt, error) {
	var r SnapshotReceipt
	if _, err := fields(raw, "publication_id", "snapshot_revision", "snapshot_digest", "advertised_at", "expires_at"); err != nil ||
		json.Unmarshal(raw, &r) != nil || !validSnapshotReceipt(r) {
		return SnapshotReceipt{}, ErrInvalid
	}
	return r, nil
}

// SnapshotDigest uses the paired fixed order; cooldowns and publication input
// are deliberately absent. A matching digest supplies no actor permission.
func SnapshotDigest(server, owner, instance string, r SnapshotReceipt, entries []Entry) (string, error) {
	if !identifier(server, "srv_") || !identifier(owner, "usr_") || !identifier(instance, "agent_") ||
		!validSnapshotReceipt(r) || ValidateEntries(entries) != nil {
		return "", ErrInvalid
	}
	preimage := struct {
		ServerID            string  `json:"server_id"`
		OwnerUserID         string  `json:"owner_user_id"`
		ConnectorInstanceID string  `json:"connector_instance_id"`
		SnapshotRevision    string  `json:"snapshot_revision"`
		AdvertisedAt        string  `json:"advertised_at"`
		ExpiresAt           string  `json:"expires_at"`
		Entries             []Entry `json:"entries"`
	}{server, owner, instance, r.SnapshotRevision, r.AdvertisedAt, r.ExpiresAt, entries}
	raw, err := json.Marshal(preimage)
	if err != nil || len(raw) > MaxEnvelopeBytes {
		return "", ErrInvalid
	}
	digest := sha256.Sum256(append([]byte("platform-container-snapshot/v1\n"), raw...))
	return hex.EncodeToString(digest[:]), nil
}

func DecodeSnapshotView(raw []byte) (SnapshotView, error) {
	var v SnapshotView
	object, err := fields(raw, "server_id", "owner_user_id", "connector_instance_id", "readiness", "snapshot_revision", "snapshot_digest", "advertised_at", "expires_at", "entries")
	if err != nil || json.Unmarshal(raw, &v) != nil || !identifier(v.ServerID, "srv_") || !identifier(v.OwnerUserID, "usr_") ||
		!identifier(v.ConnectorInstanceID, "agent_") || v.Entries == nil || len(v.Entries) > 32 {
		return SnapshotView{}, ErrInvalid
	}
	var values []json.RawMessage
	if json.Unmarshal(object["entries"], &values) != nil {
		return SnapshotView{}, ErrInvalid
	}
	entries := make([]Entry, 0, len(values))
	for i, value := range values {
		e, err := decodeEntry(value, true)
		if err != nil {
			return SnapshotView{}, ErrInvalid
		}
		v.Entries[i] = e
		entries = append(entries, e.Entry)
	}
	if ValidateEntries(entries) != nil {
		return SnapshotView{}, ErrInvalid
	}
	if v.SnapshotRevision == nil {
		if v.SnapshotDigest != nil || v.AdvertisedAt != nil || v.ExpiresAt != nil || v.Readiness != "UNAVAILABLE" || len(entries) != 0 {
			return SnapshotView{}, ErrInvalid
		}
		return v, nil
	}
	if v.SnapshotDigest == nil || v.AdvertisedAt == nil || v.ExpiresAt == nil {
		return SnapshotView{}, ErrInvalid
	}
	r := SnapshotReceipt{strings.Repeat("0", 32), *v.SnapshotRevision, *v.SnapshotDigest, *v.AdvertisedAt, *v.ExpiresAt}
	if !validSnapshotReceipt(r) {
		return SnapshotView{}, ErrInvalid
	}
	if v.Readiness == "UNAVAILABLE" {
		if len(entries) != 0 {
			return SnapshotView{}, ErrInvalid
		}
		return v, nil
	}
	if !(v.Readiness == "READY" && len(entries) > 0 || v.Readiness == "EMPTY" && len(entries) == 0) {
		return SnapshotView{}, ErrInvalid
	}
	digest, err := SnapshotDigest(v.ServerID, v.OwnerUserID, v.ConnectorInstanceID, r, entries)
	if err != nil || digest != r.SnapshotDigest {
		return SnapshotView{}, ErrInvalid
	}
	return v, nil
}
