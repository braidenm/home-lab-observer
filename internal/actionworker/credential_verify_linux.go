//go:build linux

package actionworker

import (
	"context"

	"github.com/braidenm/home-lab-observer/internal/actionremote"
)

// VerifyEnrollment performs one status-only check of the exact durable active
// credential. It never claims, renews or changes local state.
func VerifyEnrollment(ctx context.Context) error {
	op, err := newCredentialOperation(ctx)
	if err != nil {
		return ErrCredentialRecovery
	}
	defer op.close()
	settings, err := LoadSettings(op.ctx)
	if err != nil {
		return ErrCredentialRecovery
	}
	l, err := openCredentialLifecycle(op.ctx, settings)
	if err != nil {
		return ErrCredentialRecovery
	}
	defer l.close()
	return l.verifyEnrollment(op)
}

func (l *credentialLifecycle) verifyEnrollment(op *credentialOperation) error {
	if op.check() != nil || l.role(op.ctx) != nil {
		return ErrCredentialRecovery
	}
	h, raw, err := l.store.load(op.ctx)
	defer clear(raw)
	if err != nil || h.Phase != phaseReady {
		return ErrCredentialRecovery
	}
	a, err := decodeActiveRecord(raw, l.binding)
	if err != nil {
		return ErrCredentialRecovery
	}
	// The read-only remote result must match the local durable record and
	// remain unexpired when the reply is actually available.
	c := activeCandidate(a)
	m, err := l.status(op, c)
	now, clockErr := op.now()
	if err != nil || clockErr != nil || !sameMetadata(m, c, actionremote.CredentialActive) || !now.at.Before(m.ExpiresAt) {
		return ErrCredentialRecovery
	}
	return nil
}
