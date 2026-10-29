//go:build linux

package uploadledger

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
	"github.com/braidenm/home-lab-observer/internal/containerobs/model"
	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

var dockerRich = uploadstate.ProfileBinding{Kind: uploadstate.RichProcessesDockerProfile, PolicyDigest: sha256.Sum256([]byte("synthetic-docker-profile-policy"))}

func dockerPending(t *testing.T, seq int64, offset time.Duration) uploadstate.Record {
	t.Helper()
	observed := at.Add(offset)
	processes := make([]observation.Process, workloadprojection.MaxProcesses)
	for i := range processes {
		processes[i] = observation.Process{PID: int32(i + 1), Name: fmt.Sprintf("%03d", i) + strings.Repeat("<", 125), State: "running", CPUPercent: 409600, MemoryBytes: remoteprojection.MaxExactInteger, UptimeSeconds: remoteprojection.MaxExactInteger}
	}
	raw := observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: observed, Processes: observation.Section[[]observation.Process]{State: observation.Available, Data: &processes, Quality: observation.SectionQuality{Samples: len(processes), Total: len(processes)}}}
	containers := make([]model.Container, workloadprojection.MaxContainers)
	for i := range containers {
		containers[i] = model.Container{IDAlias: fmt.Sprintf("ctr_%016x", i+1), Name: strings.Repeat("<", 128), Image: strings.Repeat("<", 240), State: model.StateExited, MetricsState: model.MetricsNotRunning}
	}
	docker := model.Inventory{SchemaVersion: model.SchemaVersion, ObservedAt: &observed, SupportState: model.SupportSupported, CollectionState: model.CollectionOK, Freshness: model.FreshnessCurrent, TotalCount: len(containers), ReturnedCount: len(containers), Items: containers}
	host, err := remoteprojection.EncodeNative(raw, remoteprojection.Identity{SourceID: binding.ServerID, Version: "0.3.0-canary.1", OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	workloads, err := workloadprojection.Encode(raw, docker, workloadprojection.Identity{SourceID: binding.ServerID, Version: "0.3.0-canary.1"}, workloadprojection.Consent{Processes: true, Containers: true})
	if err != nil {
		t.Fatal(err)
	}
	body, err := combinedprojection.Encode(host, workloads, binding.ServerID)
	if err != nil || len(body) <= 2*remoteprojection.MaxBytes {
		t.Fatal("maximal Docker wire fixture invalid or too small")
	}
	return uploadstate.Record{Binding: binding, Profile: dockerRich, Watermark: seq, Pending: &uploadstate.Pending{Sequence: seq, Body: body, Digest: sha256.Sum256(body), CollectedAt: observed}}
}

func TestDockerLedgerUsesSchemaThreeAndPreservesExactRequestAcrossReopen(t *testing.T) {
	dir := directory(t)
	l, err := ProvisionForProfile(context.Background(), dir, binding, dockerRich)
	if err != nil {
		t.Fatal("Docker schema provisioning refused", err)
	}
	initial := load(t, l)
	if initial.Profile != dockerRich || limits(dockerRich) != (storageLimits{128, 3, 294912, 524288, 2097152}) {
		t.Fatal("Docker limits/profile differ")
	}
	next := dockerPending(t, math.MaxInt64, 0)
	if l.Commit(context.Background(), initial, next) != nil {
		t.Fatal("large Docker pending refused")
	}
	if l.Close() != nil {
		t.Fatal("close refused")
	}
	l, err = OpenExistingForProfile(context.Background(), dir, binding, dockerRich)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	got := load(t, l)
	if !sameRecord(got, next) || !bytes.Equal(got.Pending.Body, next.Pending.Body) {
		t.Fatal("Docker pending identity/bytes changed")
	}
	ack := uploadstate.Record{Binding: binding, Profile: dockerRich, Watermark: math.MaxInt64, HasAck: true, LastAck: next.Pending.Digest, Stopped: uploadstate.Exhausted}
	if l.Commit(context.Background(), got, ack) != nil || !sameRecord(load(t, l), ack) {
		t.Fatal("maximum sequence acknowledgement failed")
	}
}

func TestDockerLedgerRejectsProfileMutationAndOldBodiesBeforeCommit(t *testing.T) {
	for _, changed := range []string{"kind", "policy", "numeric", "processes"} {
		dir := directory(t)
		l, err := ProvisionForProfile(context.Background(), dir, binding, dockerRich)
		if err != nil {
			t.Fatal(err)
		}
		initial := load(t, l)
		next := dockerPending(t, 1, 0)
		switch changed {
		case "kind":
			next.Profile.Kind = uploadstate.RichProcessesProfile
		case "policy":
			next.Profile.PolicyDigest[0]++
		case "numeric":
			next.Pending = pending(t, 1, 0).Pending
		case "processes":
			next.Pending = richPending(t, 1, 0).Pending
		}
		if l.Commit(context.Background(), initial, next) != ErrRecovery {
			t.Fatal("mismatched Docker state adopted")
		}
		if _, err := l.Load(context.Background()); err != ErrRecovery {
			t.Fatal("invalid CAS did not latch recovery")
		}
		l.Close()
		reopened, err := OpenExistingForProfile(context.Background(), dir, binding, dockerRich)
		if err != nil {
			t.Fatal(err)
		}
		if !sameRecord(load(t, reopened), initial) {
			t.Fatal("invalid CAS mutated stored state")
		}
		reopened.Close()
	}
}
