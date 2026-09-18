package connectedtransition

import (
	"bytes"
	"testing"
)

func TestPreparationCanonicalIntent(t *testing.T) {
	for _, operation := range []string{"refresh", "code-select"} {
		record := sampleRecord(operation)
		preparation, err := Prepare(record)
		if err != nil {
			t.Fatal(operation, err)
		}
		proposed, _ := Encode(record)
		if preparation.TransitionSHA256 != hash(proposed) {
			t.Fatal("proposed record not bound")
		}
		encoded, err := EncodePreparation(preparation)
		if err != nil {
			t.Fatal(operation, err)
		}
		decoded, err := DecodePreparation(encoded)
		if err != nil || decoded.TransitionSHA256 != preparation.TransitionSHA256 {
			t.Fatal(operation, err)
		}
		again, _ := EncodePreparation(decoded)
		if !bytes.Equal(encoded, again) {
			t.Fatal("non-deterministic preparation")
		}
		if MatchesProposal(decoded, record) != nil {
			t.Fatal("exact proposal refused")
		}
		changedRecord := record
		changedRecord.NextResources.UploaderUnit = hexByte("f")
		if MatchesProposal(decoded, changedRecord) == nil {
			t.Fatal("changed proposal admitted")
		}
		for _, changed := range [][]byte{
			append([]byte(nil), encoded[:len(encoded)-1]...),
			append(append([]byte(nil), encoded...), '\n'),
			bytes.Replace(encoded, []byte(`"version"`), []byte(`"unknown":1,"version"`), 1),
			bytes.Replace(encoded, []byte(`"version"`), []byte(`"version":"observer-connected-transition-preparing/v1","version"`), 1),
			bytes.Repeat([]byte("x"), MaxPreparationBytes+1),
		} {
			if _, err := DecodePreparation(changed); err == nil {
				t.Fatal("noncanonical or oversized preparation admitted")
			}
		}
	}
}

func TestPreparationRejectsInvalidIntent(t *testing.T) {
	record := sampleRecord("refresh")
	record.Next.PolicyGeneration++
	if _, err := Prepare(record); err == nil {
		t.Fatal("invalid proposed transition admitted")
	}
	valid, err := Prepare(sampleRecord("refresh"))
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Preparation){
		"version":        func(p *Preparation) { p.Version = "observer-connected-transition-preparing/v2" },
		"transition":     func(p *Preparation) { p.TransitionSHA256 = "not-a-hash" },
		"predecessor":    func(p *Preparation) { p.PredecessorCompletionSHA = "ABC" },
		"config":         func(p *Preparation) { p.Previous.ServerID = "foreign" },
		"config hash":    func(p *Preparation) { p.PreviousResources.InstalledConfig = hexByte("f") },
		"hosts hash":     func(p *Preparation) { p.PreviousResources.Hosts = hexByte("f") },
		"resource":       func(p *Preparation) { p.PreviousResources.CA = "" },
		"logical ledger": func(p *Preparation) { p.Ledger.LogicalSHA256 = "" },
		"physical":       func(p *Preparation) { p.Ledger.Physical.Database.Inode = p.Ledger.Physical.Directory.Inode },
	} {
		changed := valid
		mutate(&changed)
		if _, err := EncodePreparation(changed); err == nil {
			t.Fatal(name, "admitted")
		}
	}
}
