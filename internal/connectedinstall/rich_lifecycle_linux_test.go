//go:build linux

package connectedinstall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedprocess"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func richLifecycleConfig(t *testing.T, b connectedprofile.Config) connectedprofile.RichProcessesConfig {
	t.Helper()
	p, err := connectedprofile.NewRichProcessesPolicy(b.ServerID, b.ConnectorID, b.CollectorUID, b.UploaderUID, b.UploaderGID, b.SharedGID)
	if err != nil {
		t.Fatal("policy fixture refused")
	}
	digest, err := connectedprofile.DigestRichProcessesPolicy(p)
	if err != nil {
		t.Fatal("policy fixture digest refused")
	}
	return connectedprofile.RichProcessesConfig{Version: connectedprofile.RichProcessesVersion, State: b.State, Profile: connectedprofile.RichProcessesProfile, ServerID: b.ServerID, ConnectorID: b.ConnectorID, CollectorUID: b.CollectorUID, UploaderUID: b.UploaderUID, UploaderGID: b.UploaderGID, SharedGID: b.SharedGID, DefinitionSHA256: connectedprofile.RichProcessesDefinitionSHA256, PolicySHA256: hex.EncodeToString(digest[:]), ArtifactSHA256: b.ArtifactSHA256, PolicyGeneration: b.PolicyGeneration, Addresses: b.Addresses}
}

func TestRichActivationUsesRichConfigDigestAndClosedTargets(t *testing.T) {
	for _, failure := range []string{"", "publish:commit"} {
		f := newActivationFixture(t)
		f.fail = failure
		c := richLifecycleConfig(t, f.config)
		ops := f.operations()
		numeric := ops
		rich := richActivationOperations(c)
		ops.pair, ops.configBytes = richProcessesPair, rich.configBytes
		collector, uploader, _, _ := richProcessesPair.units()
		translate := func(unit string) string {
			switch unit {
			case collector:
				return collectorUnit
			case uploader:
				return uploaderUnit
			default:
				t.Fatal("rich activation targeted non-rich unit")
				return ""
			}
		}
		ops.inspect = func(ctx context.Context, unit string) (workerInstance, error) {
			return numeric.inspect(ctx, translate(unit))
		}
		ops.service = func(ctx context.Context, verb, unit string) error { return numeric.service(ctx, verb, translate(unit)) }
		ops.policy = func(ctx context.Context, unit string, addresses []string) error {
			return numeric.policy(ctx, translate(unit), addresses)
		}
		ops.audit = func(ctx context.Context, pid int, path string) (connectedprocess.Identity, error) {
			if path != richProcessesPair.uploaderExecutable(c.ArtifactSHA256) {
				t.Fatal("numeric executable used for rich activation")
			}
			return numeric.audit(ctx, pid, numericPair.uploaderExecutable(c.ArtifactSHA256))
		}
		err := activateStoppedWith(context.Background(), f.config, ops)
		if failure == "" && err != nil {
			t.Fatal("rich startup sequence refused")
		}
		if failure != "" && err == nil {
			t.Fatal("failed rich commit reported success")
		}
		encoded, _ := connectedprofile.EncodeRichProcessesConfig(c)
		sum := sha256.Sum256(encoded)
		if f.request.ConfigSHA256 != hex.EncodeToString(sum[:]) {
			t.Fatal("activation did not bind canonical rich policy/config")
		}
		if failure != "" && (!hasEvent(f.events, "stop:uploader") || !hasEvent(f.events, "stop:collector")) {
			t.Fatal("rich cleanup skipped joined workers")
		}
	}
}

func TestRichActivationConfigAdapterRefusesBindingDrift(t *testing.T) {
	b := principalFixture()
	c := richLifecycleConfig(t, b)
	ops := richActivationOperations(c)
	encoded, err := ops.encodeConfig(b)
	want, _ := connectedprofile.EncodeRichProcessesConfig(c)
	if err != nil || !bytes.Equal(encoded, want) {
		t.Fatal("rich configuration binding changed")
	}
	changed := b
	changed.PolicyGeneration++
	if _, err := ops.encodeConfig(changed); err != ErrUnsafe {
		t.Fatal("network generation drift ignored")
	}
	changed = b
	changed.SharedGID++
	if _, err := ops.encodeConfig(changed); err != ErrUnsafe {
		t.Fatal("principal drift ignored")
	}
	ops.pair = workerPair(255)
	if _, err := ops.encodeConfig(b); err != ErrUnsafe {
		t.Fatal("unknown profile adapter accepted")
	}
}
