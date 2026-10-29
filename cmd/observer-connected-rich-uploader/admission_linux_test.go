//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
	"github.com/braidenm/home-lab-observer/internal/connectedcredential"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/containerobs/model"
	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

func testPolicy(t *testing.T) (connectedprofile.RichProcessesConfig, connectedprofile.RichProcessesPolicy, [32]byte) {
	t.Helper()
	p := connectedprofile.RichProcessesPolicy{Version: "observer-connected-rich-policy/v1", Profile: connectedprofile.RichProcessesProfile, ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32), CollectorUID: 60101, UploaderUID: 60102, UploaderGID: 60102, SharedGID: 60103, DefinitionSHA256: connectedprofile.RichProcessesDefinitionSHA256, ProcessesEnabled: true}
	digest, err := connectedprofile.DigestRichProcessesPolicy(p)
	if err != nil {
		t.Fatal(err)
	}
	c := connectedprofile.RichProcessesConfig{Version: connectedprofile.RichProcessesVersion, State: "INSTALLED_READY", Profile: p.Profile, ServerID: p.ServerID, ConnectorID: p.ConnectorID, CollectorUID: p.CollectorUID, UploaderUID: p.UploaderUID, UploaderGID: p.UploaderGID, SharedGID: p.SharedGID, DefinitionSHA256: p.DefinitionSHA256, PolicySHA256: hex.EncodeToString(digest[:]), ArtifactSHA256: strings.Repeat("c", 64), PolicyGeneration: 1, Addresses: []string{"1.1.1.1"}}
	return c, p, digest
}

func TestPolicyGuardClosedBindingAndPermanentRefusal(t *testing.T) {
	c, p, digest := testPolicy(t)
	for _, change := range []string{"valid", "missing-policy", "missing-config", "processes-off", "containers-on", "policy-digest", "artifact", "generation", "addresses", "principal", "definition"} {
		t.Run(change, func(t *testing.T) {
			current, policy := c, p
			reads := 0
			g, err := newPolicyGuard(c, policyPorts{config: func() (connectedprofile.RichProcessesConfig, error) {
				reads++
				if change == "missing-config" {
					return current, errors.New("synthetic-private")
				}
				return current, nil
			}, policy: func() (connectedprofile.RichProcessesPolicy, error) {
				if change == "missing-policy" {
					return policy, errors.New("synthetic-private")
				}
				return policy, nil
			}, identity: func(connectedprofile.RichProcessesConfig) error {
				if change == "principal" {
					return connectedprofile.ErrUnsafe
				}
				return nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "processes-off":
				policy.ProcessesEnabled = false
			case "containers-on":
				policy.ContainersEnabled = true
			case "policy-digest":
				current.PolicySHA256 = strings.Repeat("d", 64)
			case "artifact":
				current.ArtifactSHA256 = strings.Repeat("e", 64)
			case "generation":
				current.PolicyGeneration++
			case "addresses":
				current.Addresses = []string{"8.8.8.8"}
			case "definition":
				policy.DefinitionSHA256 = strings.Repeat("f", 64)
			}
			err = g.Check(context.Background())
			if change == "valid" {
				if err != nil || g.digest != digest {
					t.Fatal("valid policy refused")
				}
				return
			}
			if err != uploadstate.ErrRecovery {
				t.Fatal("changed policy admitted or leaked error")
			}
			before := reads
			current, policy = c, p
			if g.Check(context.Background()) != uploadstate.ErrRecovery || reads != before {
				t.Fatal("policy refusal did not latch")
			}
		})
	}
}

type fixtureSource struct {
	body  []byte
	reads int
	hook  func()
}

func (s *fixtureSource) Read(context.Context, string) ([]byte, error) {
	s.reads++
	if s.hook != nil {
		s.hook()
	}
	return s.body, nil
}

type fixtureClock struct{ at time.Time }

func (c *fixtureClock) Now() time.Time { return c.at }

type fixtureLedger struct {
	record  uploadstate.Record
	commits int
}

func (l *fixtureLedger) Load(context.Context) (uploadstate.Record, error) {
	return uploadstate.CloneRecord(l.record), nil
}
func (l *fixtureLedger) Commit(_ context.Context, expected, next uploadstate.Record) error {
	if !reflect.DeepEqual(expected, l.record) {
		return uploadstate.ErrRecovery
	}
	l.commits++
	l.record = uploadstate.CloneRecord(next)
	return nil
}

type fixtureTransport struct{ sends int }

func (tr *fixtureTransport) Send(context.Context, uploadstate.Request) (uploadstate.Response, error) {
	tr.sends++
	return uploadstate.Response{Outcome: uploadstate.Retry}, nil
}

func richFixture(t *testing.T, server string, at time.Time) []byte {
	t.Helper()
	processes := make([]observation.Process, 200)
	for i := range processes {
		processes[i] = observation.Process{PID: int32(i + 1), Name: strings.Repeat("n", 128), State: "running", CPUPercent: 125, MemoryBytes: 4096}
	}
	raw := observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: at, Processes: observation.Section[[]observation.Process]{State: observation.Available, Data: &processes, Quality: observation.SectionQuality{Samples: 200, Total: 200}}}
	host, err := remoteprojection.EncodeNative(raw, remoteprojection.Identity{SourceID: server, Version: "0.2.0-canary.1", OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	workloads, err := workloadprojection.Encode(raw, model.Inventory{}, workloadprojection.Identity{SourceID: server, Version: "0.2.0-canary.1"}, workloadprojection.Consent{Processes: true})
	if err != nil {
		t.Fatal(err)
	}
	body, err := combinedprojection.Encode(host, workloads, server)
	if err != nil || len(body) <= 16384 {
		t.Fatal("large combined fixture failed")
	}
	return body
}

func TestPolicyRefusalProtectsExpiredPendingAndCredential(t *testing.T) {
	c, p, digest := testPolicy(t)
	valid := true
	g, err := newPolicyGuard(c, policyPorts{config: func() (connectedprofile.RichProcessesConfig, error) { return c, nil }, policy: func() (connectedprofile.RichProcessesPolicy, error) {
		if !valid {
			return p, connectedprofile.ErrUnsafe
		}
		return p, nil
	}, identity: func(connectedprofile.RichProcessesConfig) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	if g.Check(context.Background()) != nil {
		t.Fatal("policy refused")
	}
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	body := richFixture(t, c.ServerID, now)
	binding := uploadstate.Binding{ServerID: c.ServerID, ConnectorID: c.ConnectorID}
	profile := uploadstate.ProfileBinding{Kind: uploadstate.RichProcessesProfile, PolicyDigest: digest}
	l := &fixtureLedger{record: uploadstate.Record{Binding: binding, Profile: profile, Watermark: 1, Pending: &uploadstate.Pending{Sequence: 1, Body: body, Digest: sha256.Sum256(body), CollectedAt: now}}}
	source := &fixtureSource{body: body}
	tr := &fixtureTransport{}
	m, err := uploadstate.NewForProfile(binding, profile, guardedSource{source, g}, &fixtureClock{now.Add(3 * time.Minute)}, &observedTransport{delegate: tr, guard: g}, guardedLedger{l, g})
	if err != nil {
		t.Fatal(err)
	}
	before := uploadstate.CloneRecord(l.record)
	valid = false
	if _, err := m.Step(context.Background()); err != uploadstate.ErrRecovery || l.commits != 0 || source.reads != 0 || tr.sends != 0 || !reflect.DeepEqual(before, l.record) {
		t.Fatal("policy refusal retired/replaced/sent expired pending")
	}
	credential := credentialSource{record: connectedcredential.CredentialRecord{ServerID: c.ServerID, ConnectorID: c.ConnectorID, Secret: "synthetic-only"}, guard: g}
	if secret, err := credential.Credential(context.Background(), binding); err == nil || secret != "" {
		t.Fatal("refused policy exposed credential")
	}
}

func TestSourceAndTransportRecheckAfterInitialAdmission(t *testing.T) {
	c, p, _ := testPolicy(t)
	valid := true
	guard := func() *policyGuard {
		g, err := newPolicyGuard(c, policyPorts{config: func() (connectedprofile.RichProcessesConfig, error) { return c, nil }, policy: func() (connectedprofile.RichProcessesPolicy, error) {
			if !valid {
				return p, connectedprofile.ErrUnsafe
			}
			return p, nil
		}, identity: func(connectedprofile.RichProcessesConfig) error { return nil }})
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	s := &fixtureSource{body: []byte("synthetic-body"), hook: func() { valid = false }}
	if body, err := (guardedSource{s, guard()}).Read(context.Background(), c.ServerID); err != uploadstate.ErrRecovery || body != nil {
		t.Fatal("source policy changed while reading")
	}
	valid = true
	g := guard()
	if g.Check(context.Background()) != nil {
		t.Fatal("guard admission failed")
	}
	valid = false
	tr := &fixtureTransport{}
	observed := &observedTransport{delegate: tr, guard: g}
	if _, err := observed.Send(context.Background(), uploadstate.Request{}); err != uploadstate.ErrRecovery || tr.sends != 0 || observed.attempts != 0 {
		t.Fatal("policy changed before network send")
	}
	if bytes.Contains([]byte(uploadstate.ErrRecovery.Error()), []byte("synthetic")) {
		t.Fatal("policy error leaked value")
	}
}

type flipClock struct {
	at   time.Time
	flip func()
}

func (c flipClock) Now() time.Time { c.flip(); return c.at }

func TestPolicyChangedAfterLoadCannotRetirePending(t *testing.T) {
	c, p, digest := testPolicy(t)
	valid := true
	g, err := newPolicyGuard(c, policyPorts{config: func() (connectedprofile.RichProcessesConfig, error) { return c, nil }, policy: func() (connectedprofile.RichProcessesPolicy, error) {
		if !valid {
			return p, connectedprofile.ErrUnsafe
		}
		return p, nil
	}, identity: func(connectedprofile.RichProcessesConfig) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	body := richFixture(t, c.ServerID, at)
	binding := uploadstate.Binding{ServerID: c.ServerID, ConnectorID: c.ConnectorID}
	profile := uploadstate.ProfileBinding{Kind: uploadstate.RichProcessesProfile, PolicyDigest: digest}
	l := &fixtureLedger{record: uploadstate.Record{Binding: binding, Profile: profile, Watermark: 1, Pending: &uploadstate.Pending{Sequence: 1, Body: body, Digest: sha256.Sum256(body), CollectedAt: at}}}
	before := uploadstate.CloneRecord(l.record)
	tr := &fixtureTransport{}
	m, err := uploadstate.NewForProfile(binding, profile, guardedSource{&fixtureSource{body: body}, g}, flipClock{at: at.Add(3 * time.Minute), flip: func() { valid = false }}, &observedTransport{delegate: tr, guard: g}, guardedLedger{l, g})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Step(context.Background()); err != uploadstate.ErrRecovery || l.commits != 0 || tr.sends != 0 || !reflect.DeepEqual(l.record, before) {
		t.Fatal("policy changed after Load bypassed pending retirement gate")
	}
}

func TestCredentialLoadIsBehindPolicyAndBindingGate(t *testing.T) {
	c, p, _ := testPolicy(t)
	valid := true
	g, err := newPolicyGuard(c, policyPorts{config: func() (connectedprofile.RichProcessesConfig, error) { return c, nil }, policy: func() (connectedprofile.RichProcessesPolicy, error) {
		if !valid {
			return p, connectedprofile.ErrUnsafe
		}
		return p, nil
	}, identity: func(connectedprofile.RichProcessesConfig) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	reads := 0
	load := func(server, connector string) (connectedcredential.CredentialRecord, error) {
		reads++
		return connectedcredential.CredentialRecord{ServerID: server, ConnectorID: connector, Secret: "synthetic-only"}, nil
	}
	valid = false
	if _, err := g.loadCredential(context.Background(), load); err == nil || reads != 0 {
		t.Fatal("refused policy reached credential loader")
	}
}
