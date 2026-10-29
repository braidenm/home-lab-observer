package uploadstate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
	"github.com/braidenm/home-lab-observer/internal/containerobs/model"
	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

var richProfile = ProfileBinding{Kind: RichProcessesProfile, PolicyDigest: sha256.Sum256([]byte("synthetic-fixed-policy"))}

func richBody(t *testing.T, at time.Time, consent workloadprojection.Consent) []byte {
	t.Helper()
	processes := make([]observation.Process, workloadprojection.MaxProcesses)
	for i := range processes {
		processes[i] = observation.Process{PID: int32(i + 1), Name: fmt.Sprintf("process-%03d-", i) + strings.Repeat("n", 100), State: "running", CPUPercent: 101, MemoryBytes: uint64(i + 1), UptimeSeconds: 1}
	}
	raw := observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: at,
		Processes: observation.Section[[]observation.Process]{State: observation.Available, Data: &processes, Quality: observation.SectionQuality{Samples: len(processes), Total: len(processes)}}}
	host, err := remoteprojection.EncodeNative(raw, remoteprojection.Identity{SourceID: testBinding.ServerID, Version: "0.2.0-canary.1", OS: "linux"})
	if err != nil {
		t.Fatal("synthetic host refused")
	}
	docker := model.Inventory{SchemaVersion: model.SchemaVersion, SupportState: model.SupportDisabled}
	workloads, err := workloadprojection.Encode(raw, docker, workloadprojection.Identity{SourceID: testBinding.ServerID, Version: "0.2.0-canary.1"}, consent)
	if err != nil {
		t.Fatal("synthetic workloads refused")
	}
	body, err := combinedprojection.Encode(host, workloads, testBinding.ServerID)
	if err != nil {
		t.Fatal("synthetic combined body refused")
	}
	return body
}

func newRichHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.s.body = richBody(t, testNow, workloadprojection.Consent{Processes: true})
	h.l.record.Profile = richProfile
	var err error
	h.m, err = NewForProfile(testBinding, richProfile, h.s, h.c, h.tr, h.l)
	if err != nil {
		t.Fatal("rich constructor refused")
	}
	return h
}

func TestProfileConfigurationIsClosedAndNumericIsDefault(t *testing.T) {
	h := newHarness(t)
	for _, profile := range []ProfileBinding{
		{Kind: ProfileKind(255)}, {Kind: RichProcessesProfile}, {PolicyDigest: richProfile.PolicyDigest},
	} {
		if _, err := NewForProfile(testBinding, profile, h.s, h.c, h.tr, h.l); err != ErrConfig {
			t.Fatal("invalid profile configuration accepted")
		}
		if ValidateRecordForProfile(Record{Binding: testBinding, Profile: profile}, testBinding, profile) != ErrRecovery {
			t.Fatal("invalid expected profile accepted")
		}
	}
	if m, err := NewForProfile(testBinding, ProfileBinding{}, h.s, h.c, h.tr, h.l); err != nil {
		t.Fatal("explicit default numeric refused")
	} else if got, err := m.Step(context.Background()); got != Acknowledged || err != nil {
		t.Fatal("default numeric behavior changed")
	}
	for _, record := range []Record{
		{Binding: testBinding, Profile: richProfile},
		{Binding: testBinding, Profile: richProfile, Stopped: Rejected},
	} {
		if ValidateRecord(record, testBinding) != ErrRecovery || ValidateRecordForProfile(record, testBinding, richProfile) != nil {
			t.Fatal("empty or terminal profile binding ignored")
		}
		wrong := richProfile
		wrong.PolicyDigest[0]++
		if ValidateRecordForProfile(record, testBinding, wrong) != ErrRecovery {
			t.Fatal("wrong expected policy digest accepted")
		}
	}
}

func TestRichAdmissionRetryRestartAndAckPreserveExactRequest(t *testing.T) {
	h := newRichHarness(t)
	if len(h.s.body) <= remoteprojection.MaxBytes || len(h.s.body) > combinedprojection.MaxBytes {
		t.Fatal("fixture does not exercise rich-only body capacity")
	}
	h.tr.response = Response{Outcome: Retry}
	h.tr.hook = func(request Request) {
		if h.l.record.Profile != richProfile || h.l.record.Pending == nil || !bytes.Equal(h.l.record.Pending.Body, request.Body) {
			t.Fatal("send preceded correctly bound durable admission")
		}
		request.Body[0] = 'x'
	}
	step(t, h, Retry, nil)
	original := clone(h.l.record)
	h.s.body = richBody(t, testNow.Add(time.Second), workloadprojection.Consent{Processes: true})
	step(t, h, Retry, nil)
	if !reflect.DeepEqual(original, h.l.record) {
		t.Fatal("retry changed bound pending state")
	}
	var err error
	h.m, err = NewForProfile(testBinding, richProfile, h.s, h.c, h.tr, h.l)
	if err != nil {
		t.Fatal(err)
	}
	h.tr.response = Response{Acknowledged, testBinding.ServerID, 1}
	step(t, h, Acknowledged, nil)
	for _, request := range h.tr.requests {
		if request.Sequence != 1 || !bytes.Equal(request.Body, original.Pending.Body) {
			t.Fatal("retry/restart changed request identity or bytes")
		}
	}
	if h.s.reads != 1 || h.l.record.Profile != richProfile || h.l.record.Pending != nil || h.l.record.LastAck != original.Pending.Digest || h.l.record.Watermark != 1 {
		t.Fatal("ack lost profile/watermark or replaced pending from source")
	}
}

func TestMachineRejectsWrongProfileEvenForEmptyOrTerminalState(t *testing.T) {
	for _, stopped := range []Outcome{"", Rejected} {
		h := newHarness(t)
		h.l.record.Profile, h.l.record.Stopped = richProfile, stopped
		step(t, h, "", ErrRecovery)
		if h.s.reads != 0 || h.l.commits != 0 || len(h.tr.requests) != 0 {
			t.Fatal("default numeric machine adopted rich empty/terminal state")
		}
		h = newRichHarness(t)
		h.l.record.Profile, h.l.record.Stopped = ProfileBinding{}, stopped
		step(t, h, "", ErrRecovery)
		if h.s.reads != 0 || h.l.commits != 0 || len(h.tr.requests) != 0 {
			t.Fatal("rich machine adopted numeric empty/terminal state")
		}
	}
}

func TestRichSourceGroupSchemaAndPrivacyRefusalBeforeCommit(t *testing.T) {
	valid := richBody(t, testNow, workloadprojection.Consent{Processes: true})
	secret := "hl" + "c_" + strings.Repeat("a", 43)
	for name, body := range map[string][]byte{
		"numeric":            bodyAt(t, testNow),
		"disabled processes": richBody(t, testNow, workloadprojection.Consent{}),
		"docker enabled":     richBody(t, testNow, workloadprojection.Consent{Processes: true, Containers: true}),
		"oversized":          bytes.Repeat([]byte("x"), combinedprojection.MaxBytes+1),
		"malformed":          []byte("private-rejected-body"),
		"wrong source":       bytes.ReplaceAll(valid, []byte(testBinding.ServerID), []byte("srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")),
		"sensitive process":  bytes.Replace(valid, []byte("process-199-"+strings.Repeat("n", 100)), []byte(secret), 1),
	} {
		t.Run(name, func(t *testing.T) {
			h := newRichHarness(t)
			h.s.body = body
			step(t, h, "", ErrSnapshot)
			if h.l.commits != 0 || len(h.tr.requests) != 0 {
				t.Fatal("invalid/sensitive source persisted or transmitted")
			}
		})
	}
	h := newHarness(t)
	h.s.body = valid
	step(t, h, "", ErrSnapshot)
	if h.l.commits != 0 || len(h.tr.requests) != 0 {
		t.Fatal("default numeric machine accepted rich source")
	}
}

func TestRichPendingAndProfileMismatchLatchBeforeAnyTransport(t *testing.T) {
	for name, mutate := range map[string]func(*Record){
		"numeric binding": func(r *Record) { r.Profile = ProfileBinding{} },
		"policy binding":  func(r *Record) { r.Profile.PolicyDigest[0]++ },
		"unknown kind":    func(r *Record) { r.Profile.Kind = ProfileKind(255) },
		"wrong group": func(r *Record) {
			r.Pending.Body = richBody(t, testNow, workloadprojection.Consent{Processes: true, Containers: true})
		},
		"numeric body": func(r *Record) { r.Pending.Body = bodyAt(t, testNow) },
		"secret body": func(r *Record) {
			r.Pending.Body = bytes.Replace(r.Pending.Body, []byte("process-199-"+strings.Repeat("n", 100)), []byte("hl"+"e_"+strings.Repeat("b", 43)), 1)
		},
		"digest":          func(r *Record) { r.Pending.Digest[0]++ },
		"collection time": func(r *Record) { r.Pending.CollectedAt = testNow.Add(time.Second) },
	} {
		t.Run(name, func(t *testing.T) {
			h := newRichHarness(t)
			body := bytes.Clone(h.s.body)
			h.l.record.Watermark = 1
			h.l.record.Pending = &Pending{Sequence: 1, Body: body, Digest: sha256.Sum256(body), CollectedAt: testNow}
			mutate(&h.l.record)
			if name != "digest" {
				h.l.record.Pending.Digest = sha256.Sum256(h.l.record.Pending.Body)
			}
			original := clone(h.l.record)
			// A malformed expired pending must not be retired and replaced to bypass admission.
			h.c.at = testNow.Add(MaxAge + time.Second)
			step(t, h, "", ErrRecovery)
			step(t, h, "", ErrRecovery)
			if h.l.loads != 1 || h.s.reads != 0 || h.l.commits != 0 || len(h.tr.requests) != 0 || !reflect.DeepEqual(original, h.l.record) {
				t.Fatal("invalid pending repaired, reloaded or sent")
			}
		})
	}
}

func TestRichCachedRetryAndCommitRevalidateImmutableBinding(t *testing.T) {
	for _, change := range []func(*Record){
		func(r *Record) { r.Profile.PolicyDigest[0]++ },
		func(r *Record) { r.Profile = ProfileBinding{} },
		func(r *Record) { r.Pending.Body[0] = 'x' },
	} {
		h := newRichHarness(t)
		h.tr.response = Response{Outcome: Retry}
		step(t, h, Retry, nil)
		change(h.m.record)
		step(t, h, "", ErrRecovery)
		if len(h.tr.requests) != 1 || h.l.commits != 1 {
			t.Fatal("cached invalid retry reached transport or ledger")
		}
	}
	h := newRichHarness(t)
	h.tr.response = Response{Outcome: Retry}
	step(t, h, Retry, nil)
	next := clone(*h.m.record)
	next.Profile.PolicyDigest[0]++
	if err := h.m.commit(context.Background(), next); err != ErrRecovery || h.l.commits != 1 {
		t.Fatal("profile mutation reached ledger CAS")
	}
}

func TestRichExpiryMaximumSequenceAndUncertainCommit(t *testing.T) {
	h := newRichHarness(t)
	h.tr.response = Response{Outcome: Retry}
	step(t, h, Retry, nil)
	h.c.at = testNow.Add(MaxAge + time.Nanosecond)
	h.s.body = richBody(t, h.c.at, workloadprojection.Consent{Processes: true})
	h.tr.response = Response{Acknowledged, testBinding.ServerID, 2}
	step(t, h, Acknowledged, nil)
	if h.l.record.Watermark != 2 || h.l.record.Profile != richProfile || h.tr.requests[1].Sequence != 2 {
		t.Fatal("rich expiry reused sequence or changed policy")
	}
	h = newRichHarness(t)
	h.l.record.Watermark = math.MaxInt64
	h.l.record.Pending = &Pending{Sequence: math.MaxInt64, Body: bytes.Clone(h.s.body), Digest: sha256.Sum256(h.s.body), CollectedAt: testNow}
	h.tr.response = Response{Acknowledged, testBinding.ServerID, math.MaxInt64}
	step(t, h, Acknowledged, nil)
	if h.l.record.Watermark != math.MaxInt64 || h.l.record.Profile != richProfile {
		t.Fatal("maximum signed sequence/profile changed")
	}
	for _, failedCommit := range []int{1, 2} {
		h = newRichHarness(t)
		h.l.failAt = failedCommit
		h.l.mutateOnFailure = true
		step(t, h, "", ErrRecovery)
		requests := len(h.tr.requests)
		step(t, h, "", ErrRecovery)
		if len(h.tr.requests) != requests || h.l.commits != failedCommit {
			t.Fatal("uncertain rich commit resumed")
		}
	}
}
