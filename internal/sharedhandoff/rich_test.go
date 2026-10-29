package sharedhandoff

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
	"github.com/braidenm/home-lab-observer/internal/containerobs/model"
	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

var richTestPolicy = RichProcessesPolicy{Policy: Policy{CollectorUID: 61001, SharedGID: 61011, UploaderUID: 61002, ServerID: "srv_0123456789abcdef0123456789abcdef"}, PolicyDigest: sha256.Sum256([]byte("synthetic-policy"))}

func richTestBody(t *testing.T, consent workloadprojection.Consent) []byte {
	t.Helper()
	processes := make([]observation.Process, 200)
	for i := range processes {
		processes[i] = observation.Process{PID: int32(i + 1), Name: fmt.Sprintf("fixture-%03d-", i) + strings.Repeat("n", 100), State: "running", CPUPercent: 101, MemoryBytes: uint64(i + 1), UptimeSeconds: 1}
	}
	raw := observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC), Processes: observation.Section[[]observation.Process]{State: observation.Available, Data: &processes, Quality: observation.SectionQuality{Samples: 200, Total: 200}}}
	host, err := remoteprojection.EncodeNative(raw, remoteprojection.Identity{SourceID: richTestPolicy.ServerID, Version: "0.2.0-canary.1", OS: "linux"})
	if err != nil {
		t.Fatal("host fixture refused")
	}
	workloads, err := workloadprojection.Encode(raw, model.Inventory{SchemaVersion: model.SchemaVersion, SupportState: model.SupportDisabled}, workloadprojection.Identity{SourceID: richTestPolicy.ServerID, Version: "0.2.0-canary.1"}, consent)
	if err != nil {
		t.Fatal("workload fixture refused")
	}
	body, err := combinedprojection.Encode(host, workloads, richTestPolicy.ServerID)
	if err != nil {
		t.Fatal("combined fixture refused")
	}
	return body
}

func TestRichHandoffBindsCanonicalBodyAndExactPolicy(t *testing.T) {
	body := richTestBody(t, workloadprojection.Consent{Processes: true})
	if len(body) <= remoteprojection.MaxBytes {
		t.Fatal("fixture must exercise rich-only capacity")
	}
	record, err := encodeRichProcesses(body, richTestPolicy)
	if err != nil || len(record) > RichProcessesMaxBytes {
		t.Fatal("rich handoff refused")
	}
	got, err := decodeRichProcesses(record, richTestPolicy)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatal("exact producer body changed")
	}
	wrong := richTestPolicy
	wrong.PolicyDigest[0]++
	if _, err := decodeRichProcesses(record, wrong); err != ErrInvalid {
		t.Fatal("wrong policy accepted")
	}
	wrong = richTestPolicy
	wrong.ServerID = "srv_ffffffffffffffffffffffffffffffff"
	if _, err := decodeRichProcesses(record, wrong); err != ErrInvalid {
		t.Fatal("wrong server accepted")
	}
	for _, invalid := range [][]byte{body, append(record, ' '), bytes.Replace(record, []byte(`"profile":`), []byte(`"unknown":true,"profile":`), 1), bytes.Repeat([]byte("x"), RichProcessesMaxBytes+1)} {
		if _, err := decodeRichProcesses(invalid, richTestPolicy); err != ErrInvalid {
			t.Fatal("noncanonical or unbound record accepted")
		}
	}
}

func TestRichHandoffDoesNotInferApprovalFromBody(t *testing.T) {
	for _, consent := range []workloadprojection.Consent{{}, {Processes: true, Containers: true}, {Containers: true}} {
		if _, err := encodeRichProcesses(richTestBody(t, consent), richTestPolicy); err != ErrInvalid {
			t.Fatal("different collection groups accepted")
		}
	}
	zero := richTestPolicy
	zero.PolicyDigest = [32]byte{}
	if _, err := encodeRichProcesses(richTestBody(t, workloadprojection.Consent{Processes: true}), zero); err != ErrInvalid {
		t.Fatal("missing installed policy accepted")
	}
}
