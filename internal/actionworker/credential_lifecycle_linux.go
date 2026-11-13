//go:build linux

package actionworker

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionprofile"
	"github.com/braidenm/home-lab-observer/internal/actionremote"
)

var ErrCredentialRecovery = errors.New("action_credential_recovery_required")

type credentialRemote interface {
	Exchange(context.Context, actionremote.EnrollmentSecret) (actionremote.CredentialDisclosure, error)
	Prepare(context.Context, actionremote.Credential) (actionremote.CredentialDisclosure, error)
	Activate(context.Context, actionremote.Credential) (actionremote.CredentialMetadata, error)
	Status(context.Context, actionremote.Credential) (actionremote.CredentialMetadata, error)
}
type credentialLifecycle struct {
	mu       sync.Mutex
	store    *credentialStore
	remote   credentialRemote
	binding  actionremote.Binding
	identity *actionprofile.Identity
}

func (*credentialLifecycle) String() string   { return "ActionCredentialLifecycle([redacted])" }
func (*credentialLifecycle) GoString() string { return "ActionCredentialLifecycle([redacted])" }
func openCredentialLifecycle(ctx context.Context, s Settings) (*credentialLifecycle, error) {
	i, err := workerIdentity(ctx)
	if err != nil {
		return nil, ErrCredentialRecovery
	}
	store, err := openCredentialStore(ctx, i, s.Binding)
	if err != nil {
		return nil, ErrCredentialRecovery
	}
	remote, err := actionremote.NewSetupClient(s.Origin, s.Binding)
	if err != nil {
		store.close()
		return nil, ErrCredentialRecovery
	}
	return &credentialLifecycle{store: store, remote: remote, binding: s.Binding, identity: &i}, nil
}
func (l *credentialLifecycle) close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if r, ok := l.remote.(interface{ Close() error }); ok {
		r.Close()
	}
	l.store.close()
}
func (l *credentialLifecycle) role(ctx context.Context) error {
	if l == nil || l.store == nil || l.remote == nil || ctx == nil || ctx.Err() != nil {
		return ErrCredentialRecovery
	}
	// Only the private fixture constructor omits the actual fixed-role witness.
	if l.identity != nil {
		i, err := workerIdentity(ctx)
		if err != nil || i != *l.identity {
			return ErrCredentialRecovery
		}
	}
	return nil
}
func (l *credentialLifecycle) Credential(ctx context.Context, b actionremote.Binding) (actionremote.Credential, error) {
	if l == nil {
		return actionremote.Credential{}, ErrCredentialRecovery
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.role(ctx) != nil || l.binding != b {
		return actionremote.Credential{}, ErrCredentialRecovery
	}
	h, raw, err := l.store.load(ctx)
	defer clear(raw)
	if err != nil || h.Phase != phaseReady {
		return actionremote.Credential{}, ErrCredentialRecovery
	}
	credential, err := decodeActive(raw, b, time.Now().UTC())
	if err != nil || ctx.Err() != nil {
		return actionremote.Credential{}, ErrCredentialRecovery
	}
	return credential, nil
}
func (l *credentialLifecycle) maintain(ctx context.Context) error {
	if l == nil {
		return ErrCredentialRecovery
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	op, err := newCredentialOperation(ctx)
	if err != nil {
		return ErrCredentialRecovery
	}
	defer op.close()
	return l.maintainOperation(op)
}
func (l *credentialLifecycle) maintainOperation(op *credentialOperation) error {
	if op.check() != nil || l.role(op.ctx) != nil {
		return ErrCredentialRecovery
	}
	h, raw, err := l.store.load(op.ctx)
	defer clear(raw)
	if err != nil {
		return ErrCredentialRecovery
	}
	switch h.Phase {
	case phaseReady:
		a, err := decodeActiveRecord(raw, l.binding)
		if err != nil {
			return ErrCredentialRecovery
		}
		now, err := op.now()
		expiry, _ := credentialExpiry(a.ExpiresAt)
		if err != nil || !now.at.Before(expiry) {
			return ErrCredentialRecovery
		}
		if expiry.Sub(now.at) > 7*24*time.Hour {
			return nil
		}
		// Persist the actual predecessor BEFORE the sole Prepare disclosure.
		h.Phase = phasePrepareAttempted
		h.PredecessorSecret = &a.AuthorizationSecret
		h.PredecessorExpiresAt = &a.ExpiresAt
		if l.store.handoff(op, h) != nil {
			return ErrCredentialRecovery
		}
		old, _ := actionremote.NewCredential(a.AuthorizationSecret)
		start, err := op.now()
		if err != nil {
			return ErrCredentialRecovery
		}
		d, err := l.remote.Prepare(op.ctx, old)
		if op.checkRemote(start) != nil || err != nil {
			return ErrCredentialRecovery
		}
		candidate, err := candidateFromDisclosure(d)
		if err != nil || candidate.State != actionremote.CredentialPrepared {
			return ErrCredentialRecovery
		}
		h.Phase = phasePrepared
		h.Candidate = &candidate
		if l.store.handoff(op, h) != nil {
			return ErrCredentialRecovery
		}
		return l.resolve(op, h)
	case phasePrepared, phaseActivateAttempted:
		return l.resolve(op, h)
	case phaseExchangeAttempted:
		if h.Candidate == nil {
			return ErrCredentialRecovery
		}
		m, err := l.status(op, *h.Candidate)
		if err != nil || !sameMetadata(m, *h.Candidate, actionremote.CredentialActive) {
			return ErrCredentialRecovery
		}
		return l.promote(op, h)
	default:
		return ErrCredentialRecovery
	}
}
func sameMetadata(m actionremote.CredentialMetadata, c candidateRecord, state actionremote.CredentialState) bool {
	n, _ := strconv.ParseInt(c.Revision, 10, 64)
	expiry, err := credentialExpiry(c.ExpiresAt)
	return err == nil && m.CredentialID == c.CredentialID && m.Revision == n && m.State == state && m.ExpiresAt.Equal(expiry)
}
func (l *credentialLifecycle) status(op *credentialOperation, c candidateRecord) (actionremote.CredentialMetadata, error) {
	secret, err := actionremote.NewCredential(c.AuthorizationSecret)
	start, clockErr := op.now()
	if err != nil || clockErr != nil {
		return actionremote.CredentialMetadata{}, ErrCredentialRecovery
	}
	m, err := l.remote.Status(op.ctx, secret)
	if op.checkRemote(start) != nil || err != nil {
		return actionremote.CredentialMetadata{}, ErrCredentialRecovery
	}
	return m, nil
}
func (l *credentialLifecycle) resolve(op *credentialOperation, h handoffDocument) error {
	old, err := predecessorRecord(h, l.binding)
	if err != nil || h.Candidate == nil {
		return ErrCredentialRecovery
	}
	// Exactly ONE sequential new-then-old pair. Neither error means INACTIVE.
	next, err := l.status(op, *h.Candidate)
	if err != nil {
		return ErrCredentialRecovery
	}
	prior, err := l.status(op, activeCandidate(old))
	if err != nil {
		return ErrCredentialRecovery
	}
	if h.Phase == phasePrepared {
		if !sameMetadata(next, *h.Candidate, actionremote.CredentialPrepared) || !sameMetadata(prior, activeCandidate(old), actionremote.CredentialActive) {
			return ErrCredentialRecovery
		}
		now, err := op.now()
		if err != nil || !now.at.Before(next.ExpiresAt) || !now.at.Before(prior.ExpiresAt) {
			return ErrCredentialRecovery
		}
		h.Phase = phaseActivateAttempted
		if l.store.handoff(op, h) != nil {
			return ErrCredentialRecovery
		}
		secret, _ := actionremote.NewCredential(h.Candidate.AuthorizationSecret)
		start, err := op.now()
		if err != nil {
			return ErrCredentialRecovery
		}
		next, err = l.remote.Activate(op.ctx, secret)
		if op.checkRemote(start) != nil || err != nil {
			return ErrCredentialRecovery
		}
	} else if prior.State != actionremote.CredentialInactive || prior.CredentialID != "" || prior.Revision != 0 || !prior.ExpiresAt.IsZero() {
		return ErrCredentialRecovery
	}
	return l.recordActiveAndPromote(op, h, next)
}
func (l *credentialLifecycle) recordActiveAndPromote(op *credentialOperation, h handoffDocument, m actionremote.CredentialMetadata) error {
	if h.Candidate == nil {
		return ErrCredentialRecovery
	}
	n, _ := strconv.ParseInt(h.Candidate.Revision, 10, 64)
	if m.State != actionremote.CredentialActive || m.CredentialID != h.Candidate.CredentialID || m.Revision != n {
		return ErrCredentialRecovery
	}
	// If this was already disclosed ACTIVE, its expiry is immutable as well.
	if h.Candidate.State == actionremote.CredentialActive && !sameMetadata(m, *h.Candidate, actionremote.CredentialActive) {
		return ErrCredentialRecovery
	}
	c, err := candidateFromDisclosure(actionremote.CredentialDisclosure{AuthorizationSecret: h.Candidate.AuthorizationSecret, Metadata: m})
	if err != nil {
		return ErrCredentialRecovery
	}
	h.Candidate = &c
	if l.store.handoff(op, h) != nil {
		return ErrCredentialRecovery
	}
	return l.promote(op, h)
}
func (l *credentialLifecycle) promote(op *credentialOperation, h handoffDocument) error {
	if h.Candidate == nil || h.Candidate.State != actionremote.CredentialActive {
		return ErrCredentialRecovery
	}
	now, err := op.now()
	expiry, _ := credentialExpiry(h.Candidate.ExpiresAt)
	if err != nil || !now.at.Before(expiry) {
		return ErrCredentialRecovery
	}
	// Candidate ACTIVE is durable before either file changes. A failed later
	// fsync poisons this owner; reopen validates the actual old/new witnesses.
	_, raw, err := l.store.load(op.ctx)
	defer clear(raw)
	if err != nil {
		return ErrCredentialRecovery
	}
	expected, err := candidateActiveBytes(*h.Candidate, l.binding)
	defer clear(expected)
	if err != nil {
		return ErrCredentialRecovery
	}
	if string(raw) != string(expected) && l.store.active(op, *h.Candidate) != nil {
		return ErrCredentialRecovery
	}
	a, _ := candidateActiveRecord(*h.Candidate, l.binding)
	ready, err := readyHandoff(l.binding, a)
	if err != nil || l.store.handoff(op, ready) != nil || op.check() != nil {
		return ErrCredentialRecovery
	}
	return nil
}
func (l *credentialLifecycle) exchange(op *credentialOperation, s actionremote.EnrollmentSecret) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.role(op.ctx) != nil || op.check() != nil {
		return ErrCredentialRecovery
	}
	h, raw, err := l.store.load(op.ctx)
	defer clear(raw)
	if err != nil || h.Phase != phaseUnenrolled {
		return ErrCredentialRecovery
	}
	h.Phase = phaseExchangeAttempted
	if l.store.handoff(op, h) != nil {
		return ErrCredentialRecovery
	}
	start, err := op.now()
	if err != nil {
		return ErrCredentialRecovery
	}
	d, err := l.remote.Exchange(op.ctx, s)
	if op.checkRemote(start) != nil || err != nil {
		return ErrCredentialRecovery
	}
	c, err := candidateFromDisclosure(d)
	if err != nil || c.State != actionremote.CredentialActive {
		return ErrCredentialRecovery
	}
	h.Candidate = &c
	if l.store.handoff(op, h) != nil {
		return ErrCredentialRecovery
	}
	return l.promote(op, h)
}
