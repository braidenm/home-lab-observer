//go:build linux

package actionbroker

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"sort"

	"github.com/braidenm/home-lab-observer/internal/actionbudget"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

type controlConversation struct {
	query         actionprotocol.SnapshotQuery
	head          string
	entriesDigest [32]byte
	send          actionbudget.Stamp
}

func policyEntries(p policyDocument) ([]actionprotocol.Entry, [32]byte, error) {
	if validatePolicy(p) != nil {
		return nil, [32]byte{}, ErrPolicy
	}
	current := make(map[string]bool)
	for _, revision := range p.Current {
		current[revision] = true
	}
	entries := make([]actionprotocol.Entry, 0, len(p.Current))
	for _, m := range p.Mappings {
		if !current[m.MappingRevision] {
			continue
		}
		hash, err := actionprotocol.ParameterDigest(m.Operation, m.Parameters)
		if err != nil {
			return nil, [32]byte{}, ErrPolicy
		}
		entries = append(entries, actionprotocol.Entry{TargetID: m.TargetID, TargetKey: displayAlias(m.ContainerID), Operation: m.Operation,
			Parameters: m.Parameters, ParameterSHA256: hash, PolicyRevision: m.PolicyRevision, CapabilityRevision: m.CapabilityRevision})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].TargetID+"\x00"+string(entries[i].Operation) < entries[j].TargetID+"\x00"+string(entries[j].Operation)
	})
	if actionprotocol.ValidateEntries(entries) != nil {
		return nil, [32]byte{}, ErrPolicy
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return nil, [32]byte{}, ErrPolicy
	}
	return entries, sha256.Sum256(raw), nil
}

func (b *brokerState) beginControl(ctx context.Context) (*controlConversation, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.quiescing || b.executionClosed.Load() {
		return nil, ErrRecoveryRequired
	}
	if b.control != nil {
		return nil, ErrBusy
	}
	s, err := b.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	// FULL or unsafe storage must not publish a usable capability set either.
	if len(s.Records)+len(b.live) >= 1000 {
		return nil, ErrBusy
	}
	entries, digest, err := policyEntries(b.policy)
	if err != nil {
		return nil, err
	}
	nonce, err := b.freshNonce()
	if err != nil {
		return nil, err
	}
	stamp, err := b.clock.Capture()
	if err != nil {
		return nil, err
	}
	c := &controlConversation{query: actionprotocol.SnapshotQuery{ServerID: b.policy.ServerID, OwnerUserID: b.policy.OwnerUserID,
		ConnectorInstanceID: b.policy.ConnectorInstanceID, PublicationID: nonce, Entries: entries}, head: b.policy.Revision, entriesDigest: digest, send: stamp}
	b.readiness = nil
	b.control = c
	return c, nil
}

func (b *brokerState) completeControl(ctx context.Context, c *controlConversation, reply actionprotocol.SnapshotReply) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c == nil || b.control != c {
		return ErrRecoveryRequired
	}
	defer func() { b.control = nil }()
	b.readiness = nil
	if ctx == nil || ctx.Err() != nil || b.quiescing || b.executionClosed.Load() || c.head != b.policy.Revision {
		return ErrRecoveryRequired
	}
	_, digest, err := policyEntries(b.policy)
	if err != nil || digest != c.entriesDigest || actionprotocol.MatchSnapshotReply(c.query, reply) != nil || reply.Receipt == nil {
		return ErrRecoveryRequired
	}
	recv, err := b.clock.Capture()
	if err != nil {
		return ErrRecoveryRequired
	}
	server, err := actionprotocol.ParseHTTPTime(reply.Receipt.AdvertisedAt)
	if err != nil {
		return ErrRecoveryRequired
	}
	ready, err := b.clock.ObserveControlTime(server, c.send, recv)
	if err != nil {
		return ErrRecoveryRequired
	}
	if b.afterControlSample != nil {
		b.afterControlSample()
	}
	if ctx.Err() != nil {
		return ErrRecoveryRequired
	}
	b.readiness = ready
	b.readyHead = c.head
	b.readyEntriesDigest = digest
	b.readyPublicationID = reply.Receipt.PublicationID
	b.readySnapshotRevision = reply.Receipt.SnapshotRevision
	b.readySnapshotDigest = reply.Receipt.SnapshotDigest
	return nil
}

func (b *brokerState) cancelControl(c *controlConversation) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c != nil && b.control == c {
		b.control = nil
		b.readiness = nil
	}
}
