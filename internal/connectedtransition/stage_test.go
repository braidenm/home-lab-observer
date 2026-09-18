package connectedtransition

import (
	"bytes"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func stageFixture(t *testing.T, operation string) (Record, []byte, map[string][]byte) {
	t.Helper()
	record := sampleRecord(operation)
	record.PredecessorCompletionSHA = ""
	oldCA := []byte("synthetic old CA\n")
	newCA := []byte("synthetic new CA\n")
	if operation == "code-select" {
		newCA = bytes.Clone(oldCA)
	}
	record.PreviousResources.CA = hash(oldCA)
	record.NextResources.CA = hash(newCA)
	proposal, err := Encode(record)
	if err != nil {
		t.Fatal(err)
	}
	witness, err := Prepare(record)
	if err != nil {
		t.Fatal(err)
	}
	preparation, err := EncodePreparation(witness)
	if err != nil {
		t.Fatal(err)
	}
	return record, preparation, map[string][]byte{
		StageProposalName: proposal,
		StageOldCAName:    oldCA,
		StageNewCAName:    newCA,
	}
}

func cloneStage(files map[string][]byte) map[string][]byte {
	clone := make(map[string][]byte, len(files))
	for name, data := range files {
		clone[name] = bytes.Clone(data)
	}
	return clone
}

func TestStageAdmissionRequiresExactFixedEvidence(t *testing.T) {
	for _, operation := range []string{"refresh", "code-select"} {
		_, preparation, files := stageFixture(t, operation)
		if _, err := ValidateStage(preparation, files); err != nil {
			t.Fatal(operation, err)
		}
		for name, mutate := range map[string]func(map[string][]byte){
			"missing proposal":    func(f map[string][]byte) { delete(f, StageProposalName) },
			"unknown entry":      func(f map[string][]byte) { f["extra"] = []byte("x") },
			"changed proposal":   func(f map[string][]byte) { f[StageProposalName] = append(f[StageProposalName], '\n') },
			"changed old CA":     func(f map[string][]byte) { f[StageOldCAName][0] ^= 1 },
			"changed new CA":     func(f map[string][]byte) { f[StageNewCAName][0] ^= 1 },
			"oversized old CA":   func(f map[string][]byte) { f[StageOldCAName] = bytes.Repeat([]byte("x"), MaxCABytes+1) },
			"empty new CA":       func(f map[string][]byte) { f[StageNewCAName] = nil },
			"orphan predecessor": func(f map[string][]byte) { f[StagePredecessorName] = []byte("x"); f[StagePredecessorCompletionName] = []byte("y") },
		} {
			changed := cloneStage(files)
			mutate(changed)
			if _, err := ValidateStage(preparation, changed); err == nil {
				t.Fatal(operation, name, "admitted")
			}
		}
		if _, err := ValidateStage(append(bytes.Clone(preparation), '\n'), files); err == nil {
			t.Fatal(operation, "noncanonical preparation admitted")
		}
	}
}

func chainedStageFixture(t *testing.T) (Record, []byte, map[string][]byte) {
	t.Helper()
	predecessor, _, earlier := stageFixture(t, "refresh")
	predecessorBytes := earlier[StageProposalName]
	predecessorCompletion, err := Completion(predecessorBytes)
	if err != nil {
		t.Fatal(err)
	}
	record := predecessor
	record.Previous = predecessor.Next
	record.Next = predecessor.Next
	record.Next.PolicyGeneration++
	record.Next.Addresses = []string{"9.9.9.9"}
	record.PreviousResources = predecessor.NextResources
	record.NextResources = predecessor.NextResources
	record.NextResources.CA = hash([]byte("third synthetic CA\n"))
	record.NextResources.Hosts = hosts(record.Next.Addresses)
	nextConfig, _ := connectedprofile.Encode(record.Next)
	record.NextResources.InstalledConfig = hash(nextConfig)
	record.PredecessorCompletionSHA = hash(predecessorCompletion)
	record.PreviousCodeBefore = predecessor.PreviousCodeAfter
	record.PreviousCodeAfter = predecessor.PreviousCodeAfter
	proposal, err := Encode(record)
	if err != nil {
		t.Fatal(err)
	}
	witness, err := Prepare(record)
	if err != nil {
		t.Fatal(err)
	}
	preparation, err := EncodePreparation(witness)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		StageProposalName:              proposal,
		StageOldCAName:                 earlier[StageNewCAName],
		StageNewCAName:                 []byte("third synthetic CA\n"),
		StagePredecessorName:           predecessorBytes,
		StagePredecessorCompletionName: predecessorCompletion,
	}
	return record, preparation, files
}

func TestStageAdmissionBindsExactPredecessor(t *testing.T) {
	record, preparation, files := chainedStageFixture(t)
	if _, err := ValidateStage(preparation, files); err != nil {
		t.Fatal("exact predecessor refused", err)
	}
	for name, mutate := range map[string]func(map[string][]byte){
		"missing journal": func(f map[string][]byte) { delete(f, StagePredecessorName) },
		"missing receipt": func(f map[string][]byte) { delete(f, StagePredecessorCompletionName) },
		"changed journal": func(f map[string][]byte) { f[StagePredecessorName][0] ^= 1 },
		"changed receipt": func(f map[string][]byte) { f[StagePredecessorCompletionName][0] ^= 1 },
	} {
		changed := cloneStage(files)
		mutate(changed)
		if _, err := ValidateStage(preparation, changed); err == nil {
			t.Fatal(name, "admitted")
		}
	}
	for name, mutate := range map[string]func(*Record){
		"resource chain": func(r *Record) { r.PreviousResources.CollectorUnit = hexByte("f") },
		"contract chain": func(r *Record) { r.ContractSHA256 = hexByte("f") },
		"code history": func(r *Record) {
			r.PreviousCodeBefore = hexByte("f")
			r.PreviousCodeAfter = hexByte("f")
		},
	} {
		changedRecord := record
		mutate(&changedRecord)
		changedProposal, err := Encode(changedRecord)
		if err != nil {
			t.Fatal(name, err)
		}
		changedWitness, err := Prepare(changedRecord)
		if err != nil {
			t.Fatal(name, err)
		}
		changedPreparation, err := EncodePreparation(changedWitness)
		if err != nil {
			t.Fatal(name, err)
		}
		changedFiles := cloneStage(files)
		changedFiles[StageProposalName] = changedProposal
		if _, err := ValidateStage(changedPreparation, changedFiles); err == nil {
			t.Fatal(name, "non-contiguous predecessor admitted")
		}
	}
	// Even an internally valid prior transition and its exact receipt cannot
	// be substituted if it does not lead to this proposal's previous state.
	unrelated, _, otherFiles := stageFixture(t, "code-select")
	unrelatedBytes := otherFiles[StageProposalName]
	unrelatedCompletion, err := Completion(unrelatedBytes)
	if err != nil {
		t.Fatal(err)
	}
	wrong := record
	wrong.PredecessorCompletionSHA = hash(unrelatedCompletion)
	wrongProposal, err := Encode(wrong)
	if err != nil {
		t.Fatal(err)
	}
	wrongWitness, err := Prepare(wrong)
	if err != nil {
		t.Fatal(err)
	}
	wrongPreparation, err := EncodePreparation(wrongWitness)
	if err != nil {
		t.Fatal(err)
	}
	wrongFiles := cloneStage(files)
	wrongFiles[StageProposalName] = wrongProposal
	wrongFiles[StagePredecessorName] = unrelatedBytes
	wrongFiles[StagePredecessorCompletionName] = unrelatedCompletion
	if _, err := ValidateStage(wrongPreparation, wrongFiles); err == nil {
		t.Fatal("non-contiguous but internally valid predecessor admitted", unrelated.Operation)
	}
}
