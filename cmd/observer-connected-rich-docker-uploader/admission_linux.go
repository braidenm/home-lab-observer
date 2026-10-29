//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/hex"

	"github.com/braidenm/home-lab-observer/internal/connectedcredential"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

// Private test ports cannot change a production pathname, profile or authority.
type policyPorts struct {
	config   func() (connectedprofile.RichProcessesDockerConfig, error)
	policy   func() (connectedprofile.RichProcessesDockerPolicy, error)
	identity func(connectedprofile.RichProcessesDockerConfig) error
}

// One serialized worker owns the guard. A failed immutable policy never resumes
// merely because files later look valid again; operator recovery is explicit.
type policyGuard struct {
	expected []byte
	binding  uploadstate.Binding
	digest   [32]byte
	ports    policyPorts
	failed   bool
}

func newPolicyGuard(c connectedprofile.RichProcessesDockerConfig, p policyPorts) (*policyGuard, error) {
	configuration, err := connectedprofile.EncodeRichProcessesDockerConfig(c)
	if err != nil || p.config == nil || p.policy == nil || p.identity == nil {
		return nil, uploadstate.ErrRecovery
	}
	digest, err := hex.DecodeString(c.PolicySHA256)
	if err != nil || len(digest) != 32 {
		return nil, uploadstate.ErrRecovery
	}
	g := &policyGuard{expected: configuration, binding: uploadstate.Binding{ServerID: c.ServerID, ConnectorID: c.ConnectorID}, ports: p}
	copy(g.digest[:], digest)
	if g.digest == [32]byte{} {
		return nil, uploadstate.ErrRecovery
	}
	return g, nil
}

func (g *policyGuard) Check(ctx context.Context) error {
	if g == nil || g.failed {
		return uploadstate.ErrRecovery
	}
	if ctx == nil || ctx.Err() != nil {
		return uploadstate.ErrCanceled
	}
	c, err := g.ports.config()
	if err == nil {
		var p connectedprofile.RichProcessesDockerPolicy
		p, err = g.ports.policy()
		if err == nil {
			err = connectedprofile.VerifyRichProcessesDockerBinding(c, p)
		}
		if err == nil {
			err = g.ports.identity(c)
		}
	}
	current, encodeErr := connectedprofile.EncodeRichProcessesDockerConfig(c)
	if err != nil || encodeErr != nil || !bytes.Equal(current, g.expected) {
		g.failed = true
		return uploadstate.ErrRecovery
	}
	if ctx.Err() != nil {
		return uploadstate.ErrCanceled
	}
	return nil
}

func (g *policyGuard) loadCredential(ctx context.Context, load func(string, string) (connectedcredential.CredentialRecord, error)) (connectedcredential.CredentialRecord, error) {
	if g.Check(ctx) != nil || load == nil {
		return connectedcredential.CredentialRecord{}, connectedcredential.ErrUnsafe
	}
	record, err := load(g.binding.ServerID, g.binding.ConnectorID)
	if err != nil || record.ServerID != g.binding.ServerID || record.ConnectorID != g.binding.ConnectorID || g.Check(ctx) != nil {
		return connectedcredential.CredentialRecord{}, connectedcredential.ErrUnsafe
	}
	return record, nil
}

type credentialSource struct {
	record connectedcredential.CredentialRecord
	guard  *policyGuard
}

func (s credentialSource) Credential(ctx context.Context, b uploadstate.Binding) (string, error) {
	if s.guard.Check(ctx) != nil || s.record.ServerID != b.ServerID || s.record.ConnectorID != b.ConnectorID {
		return "", connectedcredential.ErrUnsafe
	}
	return s.record.Secret, nil
}

type guardedSource struct {
	delegate uploadstate.Source
	guard    *policyGuard
}

func (s guardedSource) Read(ctx context.Context, server string) ([]byte, error) {
	if err := s.guard.Check(ctx); err != nil {
		return nil, err
	}
	body, err := s.delegate.Read(ctx, server)
	if checked := s.guard.Check(ctx); checked != nil {
		return nil, checked
	}
	return body, err
}

// Commit gating includes pending expiry/retirement: no consent refusal may be
// translated into dropping pending data and admitting a replacement in one Step.
type guardedLedger struct {
	delegate uploadstate.Ledger
	guard    *policyGuard
}

func (l guardedLedger) Load(ctx context.Context) (uploadstate.Record, error) {
	if err := l.guard.Check(ctx); err != nil {
		return uploadstate.Record{}, err
	}
	r, err := l.delegate.Load(ctx)
	if checked := l.guard.Check(ctx); checked != nil {
		return uploadstate.Record{}, checked
	}
	return r, err
}
func (l guardedLedger) Commit(ctx context.Context, previous, next uploadstate.Record) error {
	if err := l.guard.Check(ctx); err != nil {
		return err
	}
	return l.delegate.Commit(ctx, previous, next)
}
