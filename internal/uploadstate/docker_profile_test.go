package uploadstate

import (
	"bytes"
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
	"github.com/braidenm/home-lab-observer/internal/containerobs/model"
	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

var dockerProfile = ProfileBinding{Kind: RichProcessesDockerProfile, PolicyDigest: sha256.Sum256([]byte("synthetic-docker-policy"))}

func dockerBody(t *testing.T, at time.Time) []byte {
	t.Helper()
	processes := []observation.Process{{PID: 7, Name: "fixture-process", State: "running", CPUPercent: 101, MemoryBytes: 23}}
	raw := observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: at, Processes: observation.Section[[]observation.Process]{State: observation.Available, Data: &processes, Quality: observation.SectionQuality{Samples: 1, Total: 1}}}
	containers := model.Inventory{SchemaVersion: model.SchemaVersion, ObservedAt: &at, SupportState: model.SupportSupported, CollectionState: model.CollectionOK, Freshness: model.FreshnessCurrent, TotalCount: 1, ReturnedCount: 1,
		Items: []model.Container{{IDAlias: "ctr_" + strings.Repeat("a", 16), Name: "fixture-container", Image: "example.invalid/fixture:1", State: model.StateExited, MetricsState: model.MetricsNotRunning}}}
	host, err := remoteprojection.EncodeNative(raw, remoteprojection.Identity{SourceID: testBinding.ServerID, Version: "0.3.0-canary.1", OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	workloads, err := workloadprojection.Encode(raw, containers, workloadprojection.Identity{SourceID: testBinding.ServerID, Version: "0.3.0-canary.1"}, workloadprojection.Consent{Processes: true, Containers: true})
	if err != nil {
		t.Fatal(err)
	}
	body, err := combinedprojection.Encode(host, workloads, testBinding.ServerID)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func newDockerHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.s.body = dockerBody(t, testNow)
	h.l.record.Profile = dockerProfile
	var err error
	h.m, err = NewForProfile(testBinding, dockerProfile, h.s, h.c, h.tr, h.l)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestDockerProfileIsDistinctAndBothGroupsAreRequired(t *testing.T) {
	if NumericProfile != 0 || RichProcessesProfile != 1 || RichProcessesDockerProfile != 2 {
		t.Fatal("profile identifiers changed")
	}
	valid := dockerBody(t, testNow)
	for _, profile := range []ProfileBinding{{}, richProfile} {
		r := Record{Binding: testBinding, Profile: profile, Watermark: 1, Pending: &Pending{Sequence: 1, Body: valid, Digest: sha256.Sum256(valid), CollectedAt: testNow}}
		if ValidateRecordForProfile(r, testBinding, profile) != ErrRecovery {
			t.Fatal("older profile admitted Docker body")
		}
	}
	for _, groups := range []workloadprojection.Consent{{}, {Processes: true}, {Containers: true}} {
		h := newDockerHarness(t)
		h.s.body = richBody(t, testNow, groups)
		step(t, h, "", ErrSnapshot)
		if h.l.commits != 0 || len(h.tr.requests) != 0 {
			t.Fatal("unapproved groups persisted or sent")
		}
	}
	h := newDockerHarness(t)
	if _, err := NewForProfile(testBinding, ProfileBinding{Kind: RichProcessesDockerProfile}, h.s, h.c, h.tr, h.l); err != ErrConfig {
		t.Fatal("unbound Docker profile admitted")
	}
}

func TestDockerRetryReopenAckAndExpiryKeepExactBoundRequest(t *testing.T) {
	h := newDockerHarness(t)
	h.tr.response = Response{Outcome: Retry}
	step(t, h, Retry, nil)
	held := clone(h.l.record)
	h.s.body = dockerBody(t, testNow.Add(time.Second))
	var err error
	h.m, err = NewForProfile(testBinding, dockerProfile, h.s, h.c, h.tr, h.l)
	if err != nil {
		t.Fatal(err)
	}
	h.tr.response = Response{Acknowledged, testBinding.ServerID, 1}
	step(t, h, Acknowledged, nil)
	if len(h.tr.requests) != 2 || !bytes.Equal(h.tr.requests[1].Body, held.Pending.Body) || h.tr.requests[1].Sequence != 1 || h.l.record.Profile != dockerProfile || h.l.record.Watermark != 1 {
		t.Fatal("Docker retry identity changed")
	}
	h.c.at = testNow.Add(MaxAge + time.Second)
	h.s.body = dockerBody(t, h.c.at)
	h.tr.response = Response{Acknowledged, testBinding.ServerID, 2}
	step(t, h, Acknowledged, nil)
	if h.l.record.Watermark != 2 {
		t.Fatal("Docker sequence reused")
	}
}

func TestDockerBindingMismatchLatchesBeforePendingExpiryOrTransport(t *testing.T) {
	for _, mutate := range []func(*Record){func(r *Record) { r.Profile.Kind = RichProcessesProfile }, func(r *Record) { r.Profile.PolicyDigest[0]++ }, func(r *Record) {
		r.Pending.Body = richBody(t, testNow, workloadprojection.Consent{Processes: true})
		r.Pending.Digest = sha256.Sum256(r.Pending.Body)
	}} {
		h := newDockerHarness(t)
		h.tr.response = Response{Outcome: Retry}
		step(t, h, Retry, nil)
		mutate(&h.l.record)
		h.m, _ = NewForProfile(testBinding, dockerProfile, h.s, h.c, h.tr, h.l)
		h.c.at = testNow.Add(MaxAge + time.Second)
		step(t, h, "", ErrRecovery)
		step(t, h, "", ErrRecovery)
		if h.l.commits != 1 || len(h.tr.requests) != 1 {
			t.Fatal("mismatched pending retired or sent")
		}
	}
}
