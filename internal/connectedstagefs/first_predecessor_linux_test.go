//go:build linux

package connectedstagefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
)

func TestOwnedTransitionStageFirstPredecessorFixture(t *testing.T) {
	requireOwnedTransitionStageFixture(t)
	configPath, stagePath := syntheticStageFixture(t)
	config, err := os.Open(configPath)
	if err != nil {
		t.Fatal(err)
	}
	defer config.Close()
	stage, err := InspectFirstPredecessorAt(config)
	if err != nil || stage.Record.PredecessorFormat != connectedtransition.PredecessorNone {
		t.Fatal("initial stage with absent legacy names refused", err)
	}
	legacyJournalPath := filepath.Join(configPath, LegacyJournalName)
	legacyReceiptPath := filepath.Join(configPath, LegacyCompletionName)
	if err := os.WriteFile(legacyJournalPath, []byte("foreign"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := InspectFirstPredecessorAt(config); err != ErrRecovery || got.Record.Version != "" {
		t.Fatal("initial transition adopted existing legacy evidence", err)
	}
	if err := os.Remove(legacyJournalPath); err != nil {
		t.Fatal(err)
	}

	legacyReceipt, legacyReceiptPath := writeSyntheticLegacyPredecessor(t, configPath, stagePath, stage.Record)
	stage, err = InspectFirstPredecessorAt(config)
	if err != nil || stage.Record.PredecessorFormat != connectedtransition.PredecessorLegacy {
		t.Fatal("anchored exact legacy predecessor refused", err)
	}
	if err := os.WriteFile(legacyReceiptPath, append(append([]byte(nil), legacyReceipt...), '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := InspectFirstPredecessorAt(config); err != ErrRecovery || got.Record.Version != "" {
		t.Fatal("changed installed legacy receipt accepted", err)
	}
	if err := os.Remove(legacyReceiptPath); err != nil {
		t.Fatal(err)
	}
	if got, err := InspectFirstPredecessorAt(config); err != ErrRecovery || got.Record.Version != "" {
		t.Fatal("missing installed legacy receipt accepted", err)
	}
}

func writeSyntheticLegacyPredecessor(t *testing.T, configPath, stagePath string, record connectedtransition.Record) ([]byte, string) {
	t.Helper()
	record.Previous.PolicyGeneration = 2
	record.Next.PolicyGeneration = 3
	previousBytes, err := connectedprofile.Encode(record.Previous)
	if err != nil {
		t.Fatal(err)
	}
	nextBytes, err := connectedprofile.Encode(record.Next)
	if err != nil {
		t.Fatal(err)
	}
	record.PreviousResources.InstalledConfig = syntheticStageHash(previousBytes)
	record.NextResources.InstalledConfig = syntheticStageHash(nextBytes)
	legacyPrevious := record.Previous
	legacyPrevious.PolicyGeneration = 1
	legacyPrevious.Addresses = []string{"9.9.9.9"}
	legacyRecord := struct {
		Version  string                  `json:"version"`
		Previous connectedprofile.Config `json:"previous"`
		Next     connectedprofile.Config `json:"next"`
	}{"observer-connected-refresh/v1", legacyPrevious, record.Previous}
	legacyJournal, err := json.Marshal(legacyRecord)
	if err != nil {
		t.Fatal(err)
	}
	legacyReceipt := []byte("observer-connected-refresh-complete/v1\n" + syntheticStageHash(legacyJournal) + "\n")
	legacyJournalPath := filepath.Join(configPath, LegacyJournalName)
	legacyReceiptPath := filepath.Join(configPath, LegacyCompletionName)
	record.PredecessorFormat = connectedtransition.PredecessorLegacy
	record.PredecessorCompletionSHA = syntheticStageHash(legacyReceipt)
	proposal, err := connectedtransition.Encode(record)
	if err != nil {
		t.Fatal(err)
	}
	witness, err := connectedtransition.Prepare(record)
	if err != nil {
		t.Fatal(err)
	}
	preparation, err := connectedtransition.EncodePreparation(witness)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		filepath.Join(configPath, PreparationName): preparation,
		filepath.Join(stagePath, connectedtransition.StageProposalName): proposal,
		filepath.Join(stagePath, connectedtransition.StagePredecessorName): legacyJournal,
		filepath.Join(stagePath, connectedtransition.StagePredecessorCompletionName): legacyReceipt,
		legacyJournalPath: legacyJournal,
		legacyReceiptPath: legacyReceipt,
	} {
		if err := os.WriteFile(name, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return legacyReceipt, legacyReceiptPath
}
