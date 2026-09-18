//go:build linux

package connectedtransitionview

import (
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedresources"
	"github.com/braidenm/home-lab-observer/internal/connectedstagefs"
	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
	"github.com/braidenm/home-lab-observer/internal/ledgeridentity"
)

func transitionViewFixture(t *testing.T) (connectedstagefs.Snapshot, connectedresources.Set, connectedresources.Set, []byte) {
	t.Helper()
	previous := connectedprofile.Config{
		Version: connectedprofile.Version, State: "INSTALLED_READY",
		ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32),
		CollectorUID: 2001, UploaderUID: 2002, UploaderGID: 2003, SharedGID: 2004,
		ArtifactSHA256: strings.Repeat("c", 64), PolicyGeneration: 1, Addresses: []string{"1.1.1.1"},
	}
	next := previous
	next.PolicyGeneration++
	next.Addresses = []string{"8.8.8.8"}
	set := func(c connectedprofile.Config, ca string) connectedresources.Set {
		config, err := connectedprofile.Encode(c)
		if err != nil {
			t.Fatal(err)
		}
		units, err := connectedunits.RenderUnits(c)
		if err != nil {
			t.Fatal(err)
		}
		return connectedresources.Set{
			CA: []byte(ca), Hosts: []byte(c.Addresses[0] + " " + connectedprofile.Hostname + "\n"),
			CollectorUnit: units.Collector, UploaderUnit: units.Uploader,
			InstalledConfig: config,
		}
	}
	oldSet, newSet := set(previous, "old synthetic CA\n"), set(next, "new synthetic CA\n")
	record := connectedtransition.Record{
		Version: connectedtransition.Version, Operation: "refresh", Previous: previous, Next: next,
		ContractSHA256: strings.Repeat("d", 64),
		PreviousResources: connectedresources.Hash(oldSet), NextResources: connectedresources.Hash(newSet),
		Ledger: connectedtransition.Ledger{
			LogicalSHA256: strings.Repeat("e", 64),
			Physical: connectedtransition.FromWitness(ledgeridentity.Witness{
				FilesystemUUID: strings.Repeat("1", 32),
				Directory: ledgeridentity.Object{Inode: 7, Generation: 9},
				Database: ledgeridentity.Object{Inode: 8, Generation: 10},
			}),
		},
	}
	proposal, err := connectedtransition.Encode(record)
	if err != nil {
		t.Fatal(err)
	}
	preparation, err := connectedtransition.Prepare(record)
	if err != nil {
		t.Fatal(err)
	}
	witness, err := connectedtransition.EncodePreparation(preparation)
	if err != nil {
		t.Fatal(err)
	}
	stage := connectedstagefs.Snapshot{Preparation: witness, Record: record, Files: map[string][]byte{
		connectedtransition.StageProposalName: proposal,
		connectedtransition.StageOldCAName: oldSet.CA,
		connectedtransition.StageNewCAName: newSet.CA,
	}}
	if _, err := connectedtransition.ValidateStage(stage.Preparation, stage.Files); err != nil {
		t.Fatal("synthetic complete stage invalid", err)
	}
	completion, err := connectedtransition.Completion(proposal)
	if err != nil {
		t.Fatal(err)
	}
	return stage, oldSet, newSet, completion
}

func TestReadOnlyTransitionPhases(t *testing.T) {
	stage, oldSet, newSet, completion := transitionViewFixture(t)
	proposal := stage.Files[connectedtransition.StageProposalName]
	for _, tc := range []struct {
		name      string
		authority connectedstagefs.Authority
		active    connectedresources.Set
		want      connectedtransition.StagedPhase
	}{
		{"preparation", connectedstagefs.Authority{}, oldSet, connectedtransition.StagedPreparationPending},
		{"published-old", connectedstagefs.Authority{Journal: proposal}, oldSet, connectedtransition.StagedForwardRecovery},
		{"published-mixed", connectedstagefs.Authority{Journal: proposal}, connectedresources.Set{
			CA: newSet.CA, Hosts: oldSet.Hosts, CollectorUnit: newSet.CollectorUnit,
			UploaderUnit: oldSet.UploaderUnit, InstalledConfig: oldSet.InstalledConfig,
		}, connectedtransition.StagedForwardRecovery},
		{"completed", connectedstagefs.Authority{Journal: proposal, Completion: completion}, newSet, connectedtransition.StagedCleanupPending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			phase, err := Classify(Evidence{
				Preparation: stage.Preparation, Files: stage.Files, Record: stage.Record,
				Journal: tc.authority.Journal, Completion: tc.authority.Completion,
				Active: tc.active, Ledger: stage.Record.Ledger,
			})
			if err != nil || phase != tc.want {
				t.Fatal("valid read-only phase refused", err, phase)
			}
		})
	}
}

func TestReadOnlyTransitionRejectsForeignEvidence(t *testing.T) {
	for _, scenario := range []string{"record", "ca", "unit", "active", "ledger", "journal", "receipt"} {
		t.Run(scenario, func(t *testing.T) {
			stage, oldSet, _, _ := transitionViewFixture(t)
			authority := connectedstagefs.Authority{}
			ledger := stage.Record.Ledger
			switch scenario {
			case "record":
				stage.Record.NextResources.CollectorUnit = strings.Repeat("0", 64)
			case "ca":
				stage.Files[connectedtransition.StageOldCAName] = []byte("substituted CA")
			case "unit":
				stage.Record.PreviousResources.UploaderUnit = strings.Repeat("0", 64)
			case "active":
				oldSet.Hosts = []byte("foreign hosts")
			case "ledger":
				ledger.LogicalSHA256 = strings.Repeat("0", 64)
			case "journal":
				authority.Journal = []byte("foreign")
			case "receipt":
				authority.Completion = []byte("premature")
			}
			phase, err := Classify(Evidence{
				Preparation: stage.Preparation, Files: stage.Files, Record: stage.Record,
				Journal: authority.Journal, Completion: authority.Completion,
				Active: oldSet, Ledger: ledger,
			})
			if err != ErrUnavailable || phase != "" {
				t.Fatal("foreign transition evidence classified")
			}
		})
	}
}
