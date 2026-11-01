package actioncore

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"
	"time"
)

// JournalMetadata is private structural evidence, never authentication or a wire DTO.
type JournalMetadata struct {
	Request                       Request
	EngineGeneration, ContainerID string
}

// Field order/names/version are the original fingerprint preimage, not a new codec.
type journalRecord struct {
	Version                                                                                       int
	ActionID, ServerID, InstallationID, OwnerID, ActorID, TargetID, EngineGeneration, ContainerID string
	Operation                                                                                     Operation
	Grace                                                                                         uint8
	Policy, Capability                                                                            uint64
	Created, Expires                                                                              string
}

func journalRecordFor(r Request, b Binding) journalRecord {
	return journalRecord{1, r.ActionID, r.ServerID, r.InstallationID, r.OwnerID, r.ActorID, r.TargetID, b.EngineGeneration, b.ContainerID, r.Operation, r.GraceSeconds, r.PolicyRevision, r.CapabilityRevision, r.CreatedAt.UTC().Format(time.RFC3339Nano), r.ExpiresAt.UTC().Format(time.RFC3339Nano)}
}

// NewJournalKey verifies structure/binding; callers still require fresh authority.
func NewJournalKey(r Request, b Binding) (Key, error) {
	if !valid(r, b, r.CreatedAt) {
		return Key{}, ErrIdentityConflict
	}
	k := Key{ActionID: r.ActionID, TargetID: r.TargetID, Fingerprint: fingerprint(r, b), Metadata: JournalMetadata{r, b.EngineGeneration, b.ContainerID}}
	if _, err := CanonicalJournalMetadata(k); err != nil {
		return Key{}, err
	}
	return k, nil
}

func CanonicalJournalMetadata(k Key) ([]byte, error) {
	m := k.Metadata
	r := m.Request
	if k.ActionID != r.ActionID || k.TargetID != r.TargetID || !identifier(r.ActionID, "hla_") || !identifier(r.TargetID, "hlt_") || !identifier(r.ServerID, "srv_") || !identifier(r.InstallationID, "agent_") || !identifier(r.OwnerID, "usr_") || !identifier(r.ActorID, "usr_") || !hex(m.EngineGeneration, 32) || !hex(m.ContainerID, 64) || r.PolicyRevision == 0 || r.CapabilityRevision == 0 || r.CreatedAt.IsZero() || !r.ExpiresAt.After(r.CreatedAt) || r.ExpiresAt.Sub(r.CreatedAt) > 5*time.Minute {
		return nil, ErrIdentityConflict
	}
	if (r.Operation == Start && r.GraceSeconds != 0) || ((r.Operation == Stop || r.Operation == Restart) && (r.GraceSeconds < 1 || r.GraceSeconds > 30)) || (r.Operation != Start && r.Operation != Stop && r.Operation != Restart) {
		return nil, ErrIdentityConflict
	}
	record := journalRecordFor(r, Binding{EngineGeneration: m.EngineGeneration, ContainerID: m.ContainerID})
	for _, s := range []string{record.Created, record.Expires} {
		p, e := time.Parse(time.RFC3339Nano, s)
		if e != nil || len(s) > 30 || p.UTC().Format(time.RFC3339Nano) != s {
			return nil, ErrIdentityConflict
		}
	}
	raw, _ := json.Marshal(record)
	if len(raw) > 2048 || sha256.Sum256(raw) != k.Fingerprint {
		return nil, ErrIdentityConflict
	}
	return raw, nil
}

func ParseJournalMetadata(raw []byte) (Key, error) {
	if len(raw) == 0 || len(raw) > 2048 {
		return Key{}, ErrIdentityConflict
	}
	var v journalRecord
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&v) != nil || d.Decode(new(any)) != io.EOF || v.Version != 1 {
		return Key{}, ErrIdentityConflict
	}
	created, e := time.Parse(time.RFC3339Nano, v.Created)
	if e != nil {
		return Key{}, ErrIdentityConflict
	}
	expires, e := time.Parse(time.RFC3339Nano, v.Expires)
	if e != nil {
		return Key{}, ErrIdentityConflict
	}
	r := Request{ActionID: v.ActionID, ServerID: v.ServerID, InstallationID: v.InstallationID, OwnerID: v.OwnerID, ActorID: v.ActorID, TargetID: v.TargetID, Operation: v.Operation, GraceSeconds: v.Grace, PolicyRevision: v.Policy, CapabilityRevision: v.Capability, CreatedAt: created, ExpiresAt: expires}
	k := Key{ActionID: v.ActionID, TargetID: v.TargetID, Fingerprint: sha256.Sum256(raw), Metadata: JournalMetadata{r, v.EngineGeneration, v.ContainerID}}
	encoded, e := CanonicalJournalMetadata(k)
	if e != nil || !bytes.Equal(encoded, raw) {
		return Key{}, ErrIdentityConflict
	}
	return k, nil
}
