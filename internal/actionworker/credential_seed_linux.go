//go:build linux

package actionworker

import "context"

// SeedEnrollment initializes an empty, prepared store under the worker role.
// It does not accept a secret, connect to the platform, or recover old state.
func SeedEnrollment(ctx context.Context) error {
	op, err := newCredentialOperation(ctx)
	if err != nil {
		return ErrCredentialRecovery
	}
	defer op.close()
	i, err := workerIdentity(op.ctx)
	if err != nil {
		return ErrCredentialRecovery
	}
	settings, err := LoadSettings(op.ctx)
	if err != nil {
		return ErrCredentialRecovery
	}
	s, err := openCredentialStore(op.ctx, i, settings.Binding)
	if err != nil {
		return ErrCredentialRecovery
	}
	defer s.close()
	return s.seed(op)
}

func (s *credentialStore) seed(op *credentialOperation) error {
	if s.loaded || s.current(op.ctx) != nil {
		return ErrCredentialRecovery
	}
	for _, name := range []string{"handoff.json", "active.json"} {
		f, err := s.read(op.ctx, name, true)
		clear(f.raw)
		if err != nil || f.present {
			return ErrCredentialRecovery
		}
	}
	s.loaded = true
	if s.handoff(op, freshHandoff(s.binding)) != nil {
		return ErrCredentialRecovery
	}
	return nil
}
