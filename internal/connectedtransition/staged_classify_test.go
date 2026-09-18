package connectedtransition

import (
	"bytes"
	"testing"
)

func TestStagedClassificationWithoutNewFormatPredecessor(t *testing.T) {
	record, preparation, files := stageFixture(t, "refresh")
	proposal := files[StageProposalName]
	completion, err := Completion(proposal)
	if err != nil {
		t.Fatal(err)
	}
	before := StagedObserved{Resources: record.PreviousResources, Ledger: record.Ledger}
	if phase, err := ClassifyStaged(preparation, files, before); err != nil || phase != StagedPreparationPending {
		t.Fatal("complete preparation refused", phase, err)
	}
	mixed := record.PreviousResources
	mixed.CA = record.NextResources.CA
	for name, resources := range map[string]Resources{
		"previous": record.PreviousResources,
		"mixed":    mixed,
		"next":     record.NextResources,
	} {
		observed := StagedObserved{Journal: proposal, Resources: resources, Ledger: record.Ledger}
		if phase, err := ClassifyStaged(preparation, files, observed); err != nil || phase != StagedForwardRecovery {
			t.Fatal(name, "forward recovery refused", phase, err)
		}
	}
	closed := StagedObserved{Journal: proposal, Receipt: completion, Resources: record.NextResources, Ledger: record.Ledger}
	if phase, err := ClassifyStaged(preparation, files, closed); err != nil || phase != StagedCleanupPending {
		t.Fatal("exact completed stage refused", phase, err)
	}
	for name, mutate := range map[string]func(*StagedObserved){
		"receipt before journal": func(o *StagedObserved) { o.Journal = nil },
		"empty journal":         func(o *StagedObserved) { o.Journal = []byte{} },
		"wrong journal":         func(o *StagedObserved) { o.Journal = []byte("foreign") },
		"wrong receipt":         func(o *StagedObserved) { o.Receipt = []byte("foreign") },
		"previous resources":    func(o *StagedObserved) { o.Resources = record.PreviousResources },
		"ledger replacement": func(o *StagedObserved) {
			o.Ledger.Physical.Database.Generation++
		},
	} {
		changed := closed
		mutate(&changed)
		if _, err := ClassifyStaged(preparation, files, changed); err == nil {
			t.Fatal(name, "admitted")
		}
	}
	stale := before
	stale.Resources = mixed
	if _, err := ClassifyStaged(preparation, files, stale); err == nil {
		t.Fatal("resource replacement before publication admitted")
	}
	changedFiles := cloneStage(files)
	changedFiles[StageNewCAName][0] ^= 1
	if _, err := ClassifyStaged(preparation, changedFiles, closed); err == nil {
		t.Fatal("substituted stage admitted")
	}
}

func TestStagedClassificationRetainsOldReceiptAsOld(t *testing.T) {
	record, preparation, files := chainedStageFixture(t)
	proposal := files[StageProposalName]
	predecessor := files[StagePredecessorName]
	oldReceipt := files[StagePredecessorCompletionName]
	newReceipt, err := Completion(proposal)
	if err != nil {
		t.Fatal(err)
	}
	before := StagedObserved{Journal: predecessor, Receipt: oldReceipt, Resources: record.PreviousResources, Ledger: record.Ledger}
	if phase, err := ClassifyStaged(preparation, files, before); err != nil || phase != StagedPreparationPending {
		t.Fatal("predecessor state refused", phase, err)
	}
	mixed := record.PreviousResources
	mixed.CA = record.NextResources.CA
	published := StagedObserved{Journal: proposal, Receipt: oldReceipt, Resources: mixed, Ledger: record.Ledger}
	if phase, err := ClassifyStaged(preparation, files, published); err != nil || phase != StagedForwardRecovery {
		t.Fatal("old receipt misclassified as new completion", phase, err)
	}
	published.Resources = record.NextResources
	if phase, err := ClassifyStaged(preparation, files, published); err != nil || phase != StagedForwardRecovery {
		t.Fatal("old receipt closed the new proposal", phase, err)
	}
	published.Receipt = newReceipt
	if phase, err := ClassifyStaged(preparation, files, published); err != nil || phase != StagedCleanupPending {
		t.Fatal("new completion refused", phase, err)
	}
	for name, mutate := range map[string]func(*StagedObserved){
		"missing old receipt": func(o *StagedObserved) { o.Receipt = nil },
		"empty receipt":     func(o *StagedObserved) { o.Receipt = []byte{} },
		"foreign receipt":   func(o *StagedObserved) { o.Receipt = []byte("foreign") },
		"changed journal":   func(o *StagedObserved) { o.Journal = append(bytes.Clone(o.Journal), '\n') },
		"mixed completion":  func(o *StagedObserved) { o.Resources = mixed },
	} {
		changed := published
		mutate(&changed)
		if _, err := ClassifyStaged(preparation, files, changed); err == nil {
			t.Fatal(name, "admitted")
		}
	}
	before.Receipt = nil
	if _, err := ClassifyStaged(preparation, files, before); err == nil {
		t.Fatal("missing predecessor receipt before publication admitted")
	}
	before.Receipt = oldReceipt
	before.Resources = mixed
	if _, err := ClassifyStaged(preparation, files, before); err == nil {
		t.Fatal("changed resources before publication admitted")
	}
}
