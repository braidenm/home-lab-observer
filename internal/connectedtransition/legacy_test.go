package connectedtransition

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"
)

func legacyFixture(t *testing.T) ([]byte, []byte, legacyRefresh) {
	t.Helper()
	refresh := sampleRecord("refresh")
	record := legacyRefresh{Version: legacyRefreshVersion, Previous: refresh.Previous, Next: refresh.Next}
	journal, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	receipt := []byte("observer-connected-refresh-complete/v1\n" + hash(journal) + "\n")
	return journal, receipt, record
}

func TestCompletedLegacyRefreshAdmission(t *testing.T) {
	journal, receipt, record := legacyFixture(t)
	predecessor, err := AdmitCompletedLegacy(journal, receipt, record.Next)
	if err != nil || predecessor != hash(receipt) {
		t.Fatal("completed legacy predecessor refused")
	}
	if predecessor == hash(journal) {
		t.Fatal("journal hash substituted for receipt hash")
	}
	for name, input := range map[string][]byte{
		"missing journal": nil,
		"extra newline":   append(append([]byte(nil), journal...), '\n'),
		"unknown key":     bytes.Replace(journal, []byte(`"version"`), []byte(`"unknown":1,"version"`), 1),
		"duplicate key":   bytes.Replace(journal, []byte(`"version"`), []byte(`"version":"observer-connected-refresh/v1","version"`), 1),
		"oversized":       bytes.Repeat([]byte("x"), maxLegacyRefreshBytes+1),
	} {
		if _, err := AdmitCompletedLegacy(input, receipt, record.Next); err == nil {
			t.Fatal(name, "admitted")
		}
	}
	for name, input := range map[string][]byte{
		"missing receipt": nil,
		"empty receipt":   {},
		"changed receipt": append(append([]byte(nil), receipt...), '\n'),
		"oversized":       bytes.Repeat([]byte("x"), maxLegacyCompletionBytes+1),
	} {
		if _, err := AdmitCompletedLegacy(journal, input, record.Next); err == nil {
			t.Fatal(name, "admitted")
		}
	}
	if _, err := AdmitCompletedLegacy(journal, receipt, record.Previous); err == nil {
		t.Fatal("previous configuration accepted as current")
	}
	invalidCurrent := record.Next
	invalidCurrent.ServerID = "foreign"
	if _, err := AdmitCompletedLegacy(journal, receipt, invalidCurrent); err == nil {
		t.Fatal("invalid current configuration accepted")
	}
}

func TestLegacyRefreshRejectsIdentityAndGenerationDrift(t *testing.T) {
	_, _, valid := legacyFixture(t)
	for name, mutate := range map[string]func(*legacyRefresh){
		"version":         func(r *legacyRefresh) { r.Version = "observer-connected-refresh/v2" },
		"principal":       func(r *legacyRefresh) { r.Next.UploaderUID++ },
		"artifact":        func(r *legacyRefresh) { r.Next.ArtifactSHA256 = hexByte("f") },
		"connector":       func(r *legacyRefresh) { r.Next.ConnectorID = "agent_" + hexByte("c")[:32] },
		"generation gap":  func(r *legacyRefresh) { r.Next.PolicyGeneration++ },
		"generation wrap": func(r *legacyRefresh) { r.Previous.PolicyGeneration = math.MaxUint64 },
		"invalid address": func(r *legacyRefresh) { r.Next.Addresses = []string{"127.0.0.1"} },
	} {
		changed := valid
		mutate(&changed)
		journal, err := json.Marshal(changed)
		if err != nil {
			t.Fatal(err)
		}
		receipt := []byte("observer-connected-refresh-complete/v1\n" + hash(journal) + "\n")
		if _, err := AdmitCompletedLegacy(journal, receipt, changed.Next); err == nil {
			t.Fatal(name, "admitted")
		}
	}
}
