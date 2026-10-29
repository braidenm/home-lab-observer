package sharedhandoff

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/containerobs/model"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

func TestDockerCombinedHandoffKeepsProfilesClosed(t *testing.T) {
	p := RichProcessesDockerPolicy{Policy: richTestPolicy.Policy, PolicyDigest: richTestPolicy.PolicyDigest}
	body := richTestBody(t, workloadprojection.Consent{Processes: true, Containers: true})
	record, err := encodeRichProcessesDocker(body, p)
	if err != nil {
		t.Fatal("Docker handoff refused")
	}
	got, err := decodeRichProcessesDocker(record, p)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatal("Docker body changed")
	}
	if _, err := decodeRichProcesses(record, richTestPolicy); err == nil {
		t.Fatal("old profile adopted Docker record")
	}
	old, _ := encodeRichProcesses(richTestBody(t, workloadprojection.Consent{Processes: true}), richTestPolicy)
	if _, err := decodeRichProcessesDocker(old, p); err == nil {
		t.Fatal("Docker profile adopted old record")
	}
	for _, consent := range []workloadprojection.Consent{{}, {Processes: true}, {Containers: true}} {
		if _, err := encodeRichProcessesDocker(richTestBody(t, consent), p); err == nil {
			t.Fatal("group policy widened")
		}
	}
	p.PolicyDigest[0]++
	if _, err := decodeRichProcessesDocker(record, p); err == nil {
		t.Fatal("foreign policy accepted")
	}
}

func frameFixture(t *testing.T) (DockerFramePolicy, workloadprojection.ContainerSection, time.Time) {
	t.Helper()
	at := time.Date(2026, 10, 29, 12, 0, 0, 123456789, time.UTC)
	p := DockerFramePolicy{Policy: richTestPolicy.Policy, PolicyDigest: richTestPolicy.PolicyDigest, DefinitionDigest: sha256.Sum256([]byte("synthetic-definition"))}
	s, err := workloadprojection.ProjectContainers(model.Inventory{SchemaVersion: model.SchemaVersion, ObservedAt: &at, SupportState: model.SupportSupported, CollectionState: model.CollectionOK, Freshness: model.FreshnessCurrent, Items: []model.Container{}})
	if err != nil {
		t.Fatal("frame section refused")
	}
	return p, s, at
}

func largeFrameFixture(t *testing.T) (DockerFramePolicy, workloadprojection.ContainerSection, time.Time) {
	t.Helper()
	p, _, at := frameFixture(t)
	// IDs/digests have fixed maximal lengths. Use the longest UTC timestamp
	// spelling (four-digit year and nine fractional digits) for envelope budget.
	at = time.Date(9999, 10, 29, 12, 0, 0, 999999999, time.UTC)
	raw := model.Inventory{SchemaVersion: model.SchemaVersion, ObservedAt: &at, SupportState: model.SupportSupported, CollectionState: model.CollectionOK, Freshness: model.FreshnessCurrent, ReturnedCount: 500, TotalCount: 500}
	for i := 0; i < 500; i++ {
		raw.Items = append(raw.Items, model.Container{IDAlias: fmt.Sprintf("ctr_%016x", i), Name: strings.Repeat("n", 128), Image: strings.Repeat("i", 240), State: model.StateExited, MetricsState: model.MetricsNotRunning})
	}
	section, err := workloadprojection.ProjectContainers(raw)
	if err != nil {
		t.Fatal("large frame section refused")
	}
	b, err := section.Bytes()
	if err != nil || len(b) < 120*1024 || len(b) > workloadprojection.MaxSectionBytes {
		t.Fatal("large section budget fixture ineffective")
	}
	return p, section, at
}

func TestLargeDockerFrameFitsExactPublishedRecordBudget(t *testing.T) {
	p, section, at := largeFrameFixture(t)
	b, err := encodeDockerFrame(section, at, p)
	if err != nil || len(b) > DockerFrameMaxBytes {
		t.Fatal("large frame exceeds fixed record budget")
	}
	sectionBytes, _ := section.Bytes()
	if len(b)-len(sectionBytes) > 512 {
		t.Fatal("fixed frame metadata exceeds advertised allowance")
	}
	decoded, _, err := decodeDockerFrame(b, p)
	want, _ := section.Bytes()
	got, _ := decoded.Bytes()
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("large section changed")
	}
}

func TestDockerFrameCanonicalBindingAndFixedBudget(t *testing.T) {
	p, s, at := frameFixture(t)
	b, err := encodeDockerFrame(s, at, p)
	if err != nil || len(b) > DockerFrameMaxBytes {
		t.Fatal("frame refused")
	}
	got, produced, err := decodeDockerFrame(b, p)
	if err != nil || !produced.Equal(at) || !freshDockerFrame(got, produced, at) {
		t.Fatal("frame roundtrip refused")
	}
	want, _ := s.Bytes()
	actual, _ := got.Bytes()
	if !bytes.Equal(want, actual) {
		t.Fatal("section bytes changed")
	}
	for _, bad := range [][]byte{append(bytes.Clone(b), ' '), bytes.Replace(b, []byte(`"version":`), []byte(`"extra":true,"version":`), 1), bytes.Replace(b, []byte(`"returned_count":0`), []byte(`"returned_count":1`), 1), bytes.Repeat([]byte("x"), DockerFrameMaxBytes+1)} {
		if _, _, err := decodeDockerFrame(bad, p); err == nil {
			t.Fatal("invalid frame accepted")
		}
	}
	for _, field := range []string{"server", "policy", "definition"} {
		wrong := p
		switch field {
		case "server":
			wrong.ServerID = "srv_ffffffffffffffffffffffffffffffff"
		case "policy":
			wrong.PolicyDigest[0]++
		case "definition":
			wrong.DefinitionDigest[0]++
		}
		if _, _, err := decodeDockerFrame(b, wrong); err == nil {
			t.Fatal("foreign frame accepted")
		}
	}
	if _, err := encodeDockerFrame(s, time.Time{}, p); err == nil {
		t.Fatal("zero publication time accepted")
	}
}

func TestDockerFrameFreshnessIsNotPublicationFreshnessAlone(t *testing.T) {
	p, s, at := frameFixture(t)
	for _, tc := range []struct {
		published, now time.Time
		want           bool
	}{
		{at, at, true}, {at, at.Add(45 * time.Second), true}, {at, at.Add(45*time.Second + time.Nanosecond), false},
		{at, at.Add(-5 * time.Second), true}, {at, at.Add(-5*time.Second - time.Nanosecond), false},
		{at.Add(40 * time.Second), at.Add(46 * time.Second), false},
	} {
		if freshDockerFrame(s, tc.published, tc.now) != tc.want {
			t.Fatal("freshness matrix mismatch")
		}
	}
	if _, err := encodeDockerFrame(s, at.Add(46*time.Second), p); err == nil {
		t.Fatal("old cached rows persisted as fresh")
	}
	u := workloadprojection.UnavailableContainers()
	if !freshDockerFrame(u, at, at) {
		t.Fatal("explicit unavailable section refused")
	}
}
