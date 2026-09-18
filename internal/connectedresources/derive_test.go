package connectedresources

import (
	"bytes"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
	"github.com/braidenm/home-lab-observer/internal/ledgeridentity"
)

func sample(t *testing.T, operation string) (connectedtransition.Record, []byte, []byte) {
	t.Helper()
	previous := connectedprofile.Config{
		Version: connectedprofile.Version, State: "INSTALLED_READY",
		ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32),
		CollectorUID: 2001, UploaderUID: 2002, UploaderGID: 2003, SharedGID: 2004,
		ArtifactSHA256: strings.Repeat("c", 64), PolicyGeneration: 1, Addresses: []string{"1.1.1.1"},
	}
	next := previous
	next.PolicyGeneration++
	oldCA, newCA := []byte("old synthetic CA\n"), []byte("new synthetic CA\n")
	codeAfter := ""
	if operation == "code-select" {
		next.ArtifactSHA256 = strings.Repeat("d", 64)
		newCA = append([]byte(nil), oldCA...)
		codeAfter = previous.ArtifactSHA256
	} else {
		next.Addresses = []string{"8.8.8.8"}
	}
	oldSet, err := derive(previous, oldCA)
	if err != nil {
		t.Fatal(err)
	}
	newSet, err := derive(next, newCA)
	if err != nil {
		t.Fatal(err)
	}
	record := connectedtransition.Record{
		Version: connectedtransition.Version, Operation: operation,
		Previous: previous, Next: next, PreviousCodeAfter: codeAfter,
		ContractSHA256: strings.Repeat("e", 64),
		PreviousResources: Hash(oldSet), NextResources: Hash(newSet),
		Ledger: connectedtransition.Ledger{
			LogicalSHA256: strings.Repeat("f", 64),
			Physical: connectedtransition.FromWitness(ledgeridentity.Witness{
				FilesystemUUID: strings.Repeat("1", 32),
				Directory: ledgeridentity.Object{Inode: 7, Generation: 9},
				Database: ledgeridentity.Object{Inode: 8, Generation: 10},
			}),
		},
	}
	if err := record.Validate(); err != nil {
		t.Fatal("synthetic record invalid", err)
	}
	return record, oldCA, newCA
}

func TestDeriveExactCodeOwnedResources(t *testing.T) {
	for _, operation := range []string{"refresh", "code-select"} {
		t.Run(operation, func(t *testing.T) {
			record, oldCA, newCA := sample(t, operation)
			pair, err := Derive(record, oldCA, newCA)
			if err != nil || Hash(pair.Previous) != record.PreviousResources || Hash(pair.Next) != record.NextResources {
				t.Fatal("matching fixed resources refused", err)
			}
			oldCA[0] ^= 1
			newCA[0] ^= 1
			if !bytes.Equal(pair.Previous.CA, []byte("old synthetic CA\n")) ||
				(operation == "code-select" && !bytes.Equal(pair.Next.CA, []byte("old synthetic CA\n"))) ||
				(operation == "refresh" && !bytes.Equal(pair.Next.CA, []byte("new synthetic CA\n"))) {
				t.Fatal("stage CA alias escaped derivation")
			}
		})
	}
}

func TestDeriveRefusesUnverifiedResourceClaims(t *testing.T) {
	record, oldCA, newCA := sample(t, "refresh")
	bad := record
	bad.NextResources.UploaderUnit = strings.Repeat("0", 64)
	if _, err := Derive(bad, oldCA, newCA); err != ErrInvalid {
		t.Fatal("foreign uploader unit hash accepted")
	}
	bad = record
	bad.PreviousResources.CollectorUnit = strings.Repeat("0", 64)
	if _, err := Derive(bad, oldCA, newCA); err != ErrInvalid {
		t.Fatal("foreign predecessor unit hash accepted")
	}
	if _, err := Derive(record, []byte("substituted CA"), newCA); err != ErrInvalid {
		t.Fatal("substituted old CA accepted")
	}
	if _, err := Derive(record, oldCA, []byte("substituted CA")); err != ErrInvalid {
		t.Fatal("substituted next CA accepted")
	}
	if _, err := Derive(record, nil, newCA); err != ErrInvalid {
		t.Fatal("missing old CA accepted")
	}
	if _, err := Derive(record, oldCA, bytes.Repeat([]byte("x"), connectedtransition.MaxCABytes+1)); err != ErrInvalid {
		t.Fatal("oversized next CA accepted")
	}
	bad = record
	bad.Next.Addresses = []string{"127.0.0.1"}
	if _, err := Derive(bad, oldCA, newCA); err != ErrInvalid {
		t.Fatal("invalid next configuration accepted")
	}
}
