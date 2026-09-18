package connectedtransition

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/ledgeridentity"
)

func hexByte(b string) string { return strings.Repeat(b, 64) }

func sampleRecord(operation string) Record {
	c := connectedprofile.Config{
		Version: connectedprofile.Version, State: "INSTALLED_READY",
		ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32),
		CollectorUID: 2001, UploaderUID: 2002, UploaderGID: 2003, SharedGID: 2004,
		ArtifactSHA256: hexByte("a"), PolicyGeneration: 1, Addresses: []string{"1.1.1.1"},
	}
	next := c
	next.PolicyGeneration++
	if operation == "refresh" {
		next.Addresses = []string{"8.8.8.8"}
	} else {
		next.ArtifactSHA256 = hexByte("b")
	}
	previousConfig, _ := connectedprofile.Encode(c)
	nextConfig, _ := connectedprofile.Encode(next)
	previousResources := Resources{hexByte("1"), hosts(c.Addresses), hexByte("3"), hexByte("4"), hash(previousConfig)}
	nextResources := Resources{hexByte("1"), hosts(next.Addresses), hexByte("6"), hexByte("7"), hash(nextConfig)}
	previousCode := ""
	if operation == "code-select" {
		previousCode = c.ArtifactSHA256
	} else {
		nextResources.CA = hexByte("a")
	}
	return Record{
		Version: Version, Operation: operation, Previous: c, Next: next,
		PredecessorCompletionSHA: hexByte("c"), PreviousCodeBefore: "", PreviousCodeAfter: previousCode,
		ContractSHA256: hexByte("d"), PreviousResources: previousResources, NextResources: nextResources,
		Ledger: Ledger{LogicalSHA256: hexByte("e"), Physical: FromWitness(ledgeridentity.Witness{
			FilesystemUUID: strings.Repeat("1", 32),
			Directory:      ledgeridentity.Object{Inode: 7, Generation: 9},
			Database:       ledgeridentity.Object{Inode: 8, Generation: 10},
		})},
	}
}

func TestCanonicalRecordAndCompletion(t *testing.T) {
	for _, operation := range []string{"refresh", "code-select"} {
		r := sampleRecord(operation)
		encoded, err := Encode(r)
		if err != nil {
			t.Fatal(operation, err)
		}
		got, err := Decode(encoded)
		if err != nil || got.Validate() != nil {
			t.Fatal(operation, err)
		}
		again, err := Encode(got)
		if err != nil || !bytes.Equal(encoded, again) {
			t.Fatal("non-deterministic roundtrip")
		}
		receipt, err := Completion(encoded)
		sum := sha256.Sum256(encoded)
		want := "observer-connected-transition-complete/v1\n" + hex.EncodeToString(sum[:]) + "\n"
		if err != nil || string(receipt) != want || len(receipt) > 256 {
			t.Fatal("bad receipt")
		}
		for _, changed := range [][]byte{append([]byte(nil), encoded[:len(encoded)-1]...), append(append([]byte(nil), encoded...), '\n'), bytes.Replace(encoded, []byte(`"operation"`), []byte(`"unknown":"x","operation"`), 1), bytes.Replace(encoded, []byte(`"operation"`), []byte(`"operation":"refresh","operation"`), 1)} {
			if _, err := Decode(changed); err == nil {
				t.Fatal("noncanonical record admitted")
			}
			if _, err := Completion(changed); err == nil {
				t.Fatal("noncanonical receipt issued")
			}
		}
	}
}

func TestOperationAndWitnessRefusals(t *testing.T) {
	legacyPredecessor := sampleRecord("refresh")
	legacyPredecessor.PredecessorCompletionSHA = ""
	legacyPredecessor.PreviousCodeBefore = hexByte("f")
	legacyPredecessor.PreviousCodeAfter = legacyPredecessor.PreviousCodeBefore
	if _, err := Encode(legacyPredecessor); err != nil {
		t.Fatal("optional predecessor or retained code history refused", err)
	}
	for name, mutate := range map[string]func(*Record){
		"unknown operation":         func(r *Record) { r.Operation = "restore" },
		"generation gap":            func(r *Record) { r.Next.PolicyGeneration++ },
		"foreign owner":             func(r *Record) { r.Next.ServerID = "srv_" + strings.Repeat("c", 32) },
		"principal drift":           func(r *Record) { r.Next.UploaderUID++ },
		"artifact drift on refresh": func(r *Record) { r.Next.ArtifactSHA256 = hexByte("f") },
		"history drift on refresh":  func(r *Record) { r.PreviousCodeAfter = hexByte("f") },
		"invalid predecessor":       func(r *Record) { r.PredecessorCompletionSHA = "ABC" },
		"invalid contract":          func(r *Record) { r.ContractSHA256 = "" },
		"invalid resource":          func(r *Record) { r.NextResources.InstalledConfig = "" },
		"invalid logical":           func(r *Record) { r.Ledger.LogicalSHA256 = "" },
		"invalid physical":          func(r *Record) { r.Ledger.Physical.Database.Inode = r.Ledger.Physical.Directory.Inode },
	} {
		r := sampleRecord("refresh")
		mutate(&r)
		if _, err := Encode(r); err == nil {
			t.Fatal(name)
		}
	}
	for name, mutate := range map[string]func(*Record){
		"same artifact": func(r *Record) { r.Next.ArtifactSHA256 = r.Previous.ArtifactSHA256 },
		"wrong history": func(r *Record) { r.PreviousCodeAfter = hexByte("f") },
		"address drift": func(r *Record) { r.Next.Addresses = []string{"8.8.8.8"} },
		"CA drift":      func(r *Record) { r.NextResources.CA = hexByte("f") },
		"hosts drift":   func(r *Record) { r.NextResources.Hosts = hexByte("f") },
	} {
		r := sampleRecord("code-select")
		mutate(&r)
		if _, err := Encode(r); err == nil {
			t.Fatal(name)
		}
	}
	r := sampleRecord("code-select")
	r.PreviousCodeBefore = hexByte("f")
	if _, err := Encode(r); err != nil {
		t.Fatal("completed history refused", err)
	}
	r.Previous.PolicyGeneration = ^uint64(0)
	r.Next.PolicyGeneration = 0
	if _, err := Encode(r); err == nil {
		t.Fatal("generation overflow accepted")
	}
}

func TestBoundAndNoInputMutation(t *testing.T) {
	r := sampleRecord("refresh")
	if _, err := Decode(bytes.Repeat([]byte(" "), MaxRecordBytes+1)); err == nil {
		t.Fatal("oversize accepted")
	}
	before := append([]string(nil), r.Next.Addresses...)
	if _, err := Encode(r); err != nil || !bytes.Equal([]byte(strings.Join(before, ",")), []byte(strings.Join(r.Next.Addresses, ","))) {
		t.Fatal("record mutated")
	}
}
