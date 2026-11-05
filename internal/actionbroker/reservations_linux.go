//go:build linux

package actionbroker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionbudget"
	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionjournal"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

var (
	ErrBusy             = errors.New("action_broker_busy")
	ErrNoEligibleTarget = errors.New("action_broker_no_eligible_target")
	ErrRecoveryRequired = errors.New("action_broker_recovery_required")
)

// A reservation is one live pre-HTTP conversation. Its nonce is neither remote
// authority nor a durable resume key. Only the joined owning handler releases it.
type reservation struct {
	nonce        string
	policyHead   string
	targets      []string
	conversation *actionbudget.Conversation
	bound        *resolvedBinding
	claimTried   bool
}

type brokerState struct {
	executionClosed       atomic.Bool // One-way process fence; never a remote permission.
	mu                    sync.Mutex
	journal               *actionjournal.Journal
	policy                policyDocument
	clock                 *actionbudget.Clock
	readiness             *actionbudget.Readiness
	readyHead             string
	readyEntriesDigest    [32]byte
	readyPublicationID    string
	readySnapshotRevision string
	readySnapshotDigest   string
	control               *controlConversation
	quiescing             bool
	live                  map[string]*reservation
	recoveryRequired      bool
	afterControlSample    func() // Private fault observer; production is always nil.
}

func newBrokerState(ctx context.Context, p policyDocument, j *actionjournal.Journal, clock *actionbudget.Clock) (*brokerState, error) {
	if validatePolicy(p) != nil || j == nil || clock == nil {
		return nil, ErrPolicy
	}
	b := &brokerState{journal: j, policy: p, clock: clock, live: make(map[string]*reservation)}
	if _, err := b.snapshot(ctx); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *brokerState) snapshot(ctx context.Context) (actionjournal.AdmissionSnapshot, error) {
	s, err := b.journal.AdmissionSnapshot(ctx)
	if err != nil || s.Binding != (actionjournal.Binding{ServerID: b.policy.ServerID, InstallationID: b.policy.ConnectorInstanceID, OwnerID: b.policy.OwnerUserID}) {
		return actionjournal.AdmissionSnapshot{}, ErrRecoveryRequired
	}
	owned := make(map[string]bool)
	for _, live := range b.live {
		if live.bound != nil {
			owned[live.bound.key.ActionID] = true
		}
	}
	for _, record := range s.Records {
		if (record.HasIntent && record.TerminalAt.IsZero() || record.Active) && !owned[record.Key.ActionID] {
			b.recoveryRequired = true
		}
	}
	if b.recoveryRequired {
		return actionjournal.AdmissionSnapshot{}, ErrRecoveryRequired
	}
	return s, nil
}

func physicalMatch(key actioncore.Key, m mappingDocument) bool {
	return key.TargetID == m.TargetID || key.Metadata.EngineGeneration == m.EngineGeneration && key.Metadata.ContainerID == m.ContainerID
}

func (b *brokerState) eligible(s actionjournal.AdmissionSnapshot, except *reservation) []string {
	current := make(map[string]bool)
	for _, revision := range b.policy.Current {
		current[revision] = true
	}
	targets := make(map[string]bool)
	for _, mapping := range b.policy.Mappings {
		if !current[mapping.MappingRevision] {
			continue
		}
		blocked := false
		for _, live := range b.live {
			if live != except && live.bound != nil && physicalMatch(live.bound.key, mapping) {
				blocked = true
			}
		}
		for _, record := range s.Records {
			if !physicalMatch(record.Key, mapping) {
				continue
			}
			if record.HasIntent && record.TerminalAt.IsZero() ||
				(record.Code == actioncore.Started || record.Code == actioncore.Stopped || record.Code == actioncore.Restarted) &&
					s.ObservedAt.Before(record.TerminalAt.Add(30*time.Second)) {
				blocked = true
			}
		}
		if !blocked {
			targets[mapping.TargetID] = true
		}
	}
	result := make([]string, 0, len(targets))
	for target := range targets {
		result = append(result, target)
	}
	sort.Strings(result)
	return result
}

func (b *brokerState) reserve(ctx context.Context) (*reservation, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.quiescing || b.executionClosed.Load() {
		return nil, ErrRecoveryRequired
	}
	_, entriesDigest, entryError := policyEntries(b.policy)
	if entryError != nil || b.readyHead != b.policy.Revision || b.readyEntriesDigest != entriesDigest {
		b.readiness = nil
		return nil, actionbudget.ErrUnavailable
	}
	s, err := b.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	if len(b.live) >= 2 {
		return nil, ErrBusy
	}
	stored := make(map[string]bool)
	for _, record := range s.Records {
		stored[record.Key.ActionID] = true
	}
	logical := len(s.Records)
	for _, live := range b.live {
		if live.bound == nil || !stored[live.bound.key.ActionID] {
			logical++
		}
	}
	if logical >= 1000 {
		return nil, ErrBusy
	}
	targets := b.eligible(s, nil)
	if len(targets) == 0 {
		return nil, ErrNoEligibleTarget
	}
	conversation, err := b.clock.Begin(b.readiness)
	if err != nil {
		return nil, actionbudget.ErrUnavailable
	}
	nonce, err := b.freshNonce()
	if err != nil {
		return nil, err
	}
	r := &reservation{nonce: nonce, policyHead: b.policy.Revision, targets: targets, conversation: conversation}
	b.live[nonce] = r
	return r, nil
}

func (b *brokerState) freshNonce() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil || random == [16]byte{} {
		return "", actionbudget.ErrUnavailable
	}
	nonce := hex.EncodeToString(random[:])
	if b.live[nonce] != nil || b.control != nil && b.control.query.PublicationID == nonce {
		return "", actionbudget.ErrUnavailable
	}
	return nonce, nil
}

func (b *brokerState) releaseUnclaimed(ctx context.Context, r *reservation) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r == nil || b.live[r.nonce] != r || r.claimTried || r.conversation == nil || r.conversation.CheckDelivery(ctx) != nil {
		return ErrRecoveryRequired
	}
	delete(b.live, r.nonce)
	return nil
}

func (b *brokerState) bindClaim(ctx context.Context, r *reservation, claim actionprotocol.Claim, rtt time.Duration) (actioncore.Journal, *actionbudget.Execution, resolvedBinding, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	fail := func() (actioncore.Journal, *actionbudget.Execution, resolvedBinding, error) {
		return nil, nil, resolvedBinding{}, ErrRecoveryRequired
	}
	if r == nil || b.live[r.nonce] != r || r.claimTried || r.policyHead != b.policy.Revision || b.quiescing || b.executionClosed.Load() {
		return fail()
	}
	r.claimTried = true
	s, err := b.snapshot(ctx)
	if err != nil {
		return fail()
	}
	for _, record := range s.Records {
		if record.Key.ActionID == claim.Request.ActionID {
			return fail() // A newly claimed conversation must never resume old work.
		}
	}
	resolved, err := resolveReceipt(b.policy, claim.Request, true)
	if err != nil || !containsTarget(r.targets, resolved.key.TargetID) || !containsTarget(b.eligible(s, r), resolved.key.TargetID) {
		return fail()
	}
	if r.conversation.CheckDelivery(ctx) != nil {
		return fail()
	}
	execution, err := r.conversation.AcceptClaim(claim, rtt, resolved.binding.ContainerID)
	if err != nil || ctx.Err() != nil {
		return fail()
	}
	bound, err := b.journal.BindExecution(resolved.key, resolved.remoteDigest)
	if err != nil {
		return fail()
	}
	r.bound = &resolved
	return bound, execution, resolved, nil
}

func containsTarget(targets []string, target string) bool {
	index := sort.SearchStrings(targets, target)
	return index < len(targets) && targets[index] == target
}

// finish is called only after the owning core invocation and its Attempt have
// joined. Failure without a retained known terminal is unconfirmed, not FAILED.
func (b *brokerState) finish(r *reservation) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r == nil || b.live[r.nonce] != r || !r.claimTried {
		return ErrRecoveryRequired
	}
	s, err := b.journal.AdmissionSnapshot(context.Background())
	if err != nil {
		b.recoveryRequired = true
		return ErrRecoveryRequired
	}
	confirmed := false
	for _, record := range s.Records {
		if r.bound != nil && record.Key.ActionID == r.bound.key.ActionID {
			if record.Active {
				return ErrBusy // Do not release a still-owned journal handle.
			}
			confirmed = !record.TerminalAt.IsZero()
		}
	}
	if !confirmed {
		b.recoveryRequired = true
	}
	delete(b.live, r.nonce)
	return nil
}
