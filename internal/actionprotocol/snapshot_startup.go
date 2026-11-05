package actionprotocol

import (
	"bytes"
	"encoding/json"
)

// SnapshotQuery is a control-only publication request. Its ID is never a claim
// or execution nonce; the broker keeps native time and local policy state private.
type SnapshotQuery struct {
	ServerID            string  `json:"server_id"`
	OwnerUserID         string  `json:"owner_user_id"`
	ConnectorInstanceID string  `json:"connector_instance_id"`
	PublicationID       string  `json:"publication_id"`
	Entries             []Entry `json:"entries"`
}

type SnapshotReply struct {
	Outcome string           `json:"outcome"`
	Receipt *SnapshotReceipt `json:"receipt"`
}

func ValidateSnapshotQuery(q SnapshotQuery) error {
	if !identifier(q.ServerID, "srv_") || !identifier(q.OwnerUserID, "usr_") ||
		!identifier(q.ConnectorInstanceID, "agent_") || !nonce(q.PublicationID) || ValidateEntries(q.Entries) != nil {
		return ErrInvalid
	}
	return nil
}

func DecodeSnapshotQuery(raw []byte) (SnapshotQuery, error) {
	var q SnapshotQuery
	object, err := fields(raw, "server_id", "owner_user_id", "connector_instance_id", "publication_id", "entries")
	if err != nil || json.Unmarshal(raw, &q) != nil {
		return SnapshotQuery{}, ErrInvalid
	}
	q.Entries, err = decodeEntries(object["entries"])
	if err != nil || ValidateSnapshotQuery(q) != nil {
		return SnapshotQuery{}, ErrInvalid
	}
	return q, nil
}

func DecodeSnapshotReply(raw []byte) (SnapshotReply, error) {
	var r SnapshotReply
	object, err := fields(raw, "outcome", "receipt")
	if err != nil || json.Unmarshal(raw, &r) != nil {
		return SnapshotReply{}, ErrInvalid
	}
	if r.Outcome == "OK" {
		receipt, err := DecodeSnapshotReceipt(object["receipt"])
		if err != nil {
			return SnapshotReply{}, ErrInvalid
		}
		r.Receipt = &receipt
		return r, nil
	}
	if (r.Outcome != "DENIED" && r.Outcome != "UNAVAILABLE" && r.Outcome != "CONFLICT") ||
		!bytes.Equal(bytes.TrimSpace(object["receipt"]), []byte("null")) {
		return SnapshotReply{}, ErrInvalid
	}
	return r, nil
}

// MatchSnapshotReply proves only exact public publication correlation. The
// broker must separately enforce peer authentication, fresh timing and policy.
func MatchSnapshotReply(q SnapshotQuery, r SnapshotReply) error {
	if ValidateSnapshotQuery(q) != nil || r.Outcome != "OK" || r.Receipt == nil || r.Receipt.PublicationID != q.PublicationID {
		return ErrInvalid
	}
	digest, err := SnapshotDigest(q.ServerID, q.OwnerUserID, q.ConnectorInstanceID, *r.Receipt, q.Entries)
	if err != nil || digest != r.Receipt.SnapshotDigest {
		return ErrInvalid
	}
	return nil
}

func MatchSnapshotHead(q SnapshotQuery, v SnapshotView) error {
	if ValidateSnapshotQuery(q) != nil || q.ServerID != v.ServerID || q.OwnerUserID != v.OwnerUserID || q.ConnectorInstanceID != v.ConnectorInstanceID {
		return ErrInvalid
	}
	// The first connected profile cannot replace its public approvals on restart.
	// SnapshotHead already decoded the complete view; retain explicit nil checks
	// here so a partial typed view cannot panic or become initial publication.
	if v.SnapshotRevision == nil {
		if v.SnapshotDigest != nil || v.AdvertisedAt != nil || v.ExpiresAt != nil || v.Readiness != "UNAVAILABLE" || len(v.Entries) != 0 {
			return ErrInvalid
		}
		return nil
	}
	if v.SnapshotDigest == nil || v.AdvertisedAt == nil || v.ExpiresAt == nil {
		return ErrInvalid
	}
	// Publication ID and observational cooldowns are not in this preimage. An
	// expired head hides its entries but retains the authenticated digest/times.
	r := SnapshotReceipt{"00000000000000000000000000000000", *v.SnapshotRevision, *v.SnapshotDigest, *v.AdvertisedAt, *v.ExpiresAt}
	digest, err := SnapshotDigest(q.ServerID, q.OwnerUserID, q.ConnectorInstanceID, r, q.Entries)
	if err != nil || digest != *v.SnapshotDigest {
		return ErrInvalid
	}
	return nil
}
