package uploadstate

import (
	"bytes"
	"context"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
)

func nativeBody(t *testing.T) []byte {
	t.Helper()
	raw := observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: testNow}
	raw.CPU = observation.Section[observation.CPU]{State: observation.Available, Data: &observation.CPU{LogicalCPUs: 4, UsagePercent: 12}}
	raw.Memory.State = observation.Unknown
	raw.Uptime.State = observation.Unknown
	raw.Filesystems.State = observation.Unknown
	body, err := remoteprojection.EncodeNative(raw, remoteprojection.Identity{SourceID: testBinding.ServerID, Version: "0.1.0", OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestSchemaTransitionRetainsPendingBytesAndSequence(t *testing.T) {
	for _, nativeFirst := range []bool{false, true} {
		h := newHarness(t)
		legacy, native := bytes.Clone(h.s.body), nativeBody(t)
		if nativeFirst {
			h.s.body = native
		}
		h.tr.err = privateError
		step(t, h, Retry, nil)
		original := bytes.Clone(h.l.record.Pending.Body)
		if nativeFirst {
			h.s.body = legacy
		} else {
			h.s.body = native
		}
		if ValidateRecord(h.l.record, testBinding) != nil {
			t.Fatal("existing pending schema refused")
		}
		var err error
		h.m, err = New(testBinding, h.s, h.c, h.tr, h.l)
		if err != nil {
			t.Fatal(err)
		}
		h.tr.err = nil
		step(t, h, Acknowledged, nil)
		for _, request := range h.tr.requests {
			if request.Sequence != 1 || !bytes.Equal(request.Body, original) {
				t.Fatal("schema transition changed pending bytes or reused sequence")
			}
		}
		h.tr.response.Sequence = 2
		step(t, h, Acknowledged, nil)
		last := h.tr.requests[len(h.tr.requests)-1]
		if last.Sequence != 2 || !bytes.Equal(last.Body, h.s.body) || h.l.record.Watermark != 2 {
			t.Fatal("new profile did not retain monotonic allocation")
		}
		step(t, h, Idle, nil)
	}
}

func TestNativeCredentialRejectionRemainsTerminal(t *testing.T) {
	h := newHarness(t)
	h.s.body = nativeBody(t)
	h.tr.response = Response{Outcome: CredentialRejected}
	step(t, h, CredentialRejected, nil)
	if ValidateRecord(h.l.record, testBinding) != nil {
		t.Fatal("native terminal record invalid")
	}
	h.m, _ = New(testBinding, h.s, h.c, h.tr, h.l)
	_, _ = h.m.Step(context.Background())
	if len(h.tr.requests) != 1 {
		t.Fatal("revoked native upload was retried")
	}
}
