package connectedtransition

import (
	"bytes"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

const MaxCABytes = 1 << 20

const (
	StageProposalName              = "proposal.json"
	StageOldCAName                 = "old-ca.pem"
	StageNewCAName                 = "new-ca.pem"
	StagePredecessorName           = "predecessor.json"
	StagePredecessorCompletionName = "predecessor-complete"
)

// ValidateStage checks only the exact, bounded bytes supplied by a caller.
// The caller must separately audit root ownership, no-follow file identity,
// directory membership and durability before using this as recovery evidence.
func ValidateStage(preparation []byte, files map[string][]byte) (Record, error) {
	if len(files) != 3 && len(files) != 5 {
		return Record{}, ErrInvalid
	}
	for name := range files {
		switch name {
		case StageProposalName, StageOldCAName, StageNewCAName,
			StagePredecessorName, StagePredecessorCompletionName:
		default:
			return Record{}, ErrInvalid
		}
	}
	oldCA, oldPresent := files[StageOldCAName]
	newCA, newPresent := files[StageNewCAName]
	proposal, proposalPresent := files[StageProposalName]
	if !oldPresent || !newPresent || !proposalPresent ||
		len(oldCA) == 0 || len(oldCA) > MaxCABytes ||
		len(newCA) == 0 || len(newCA) > MaxCABytes {
		return Record{}, ErrInvalid
	}
	witness, err := DecodePreparation(preparation)
	if err != nil {
		return Record{}, ErrInvalid
	}
	record, err := Decode(proposal)
	if err != nil || MatchesProposal(witness, record) != nil ||
		hash(oldCA) != record.PreviousResources.CA || hash(newCA) != record.NextResources.CA {
		return Record{}, ErrInvalid
	}
	predecessor, hasPredecessor := files[StagePredecessorName]
	completion, hasCompletion := files[StagePredecessorCompletionName]
	if hasPredecessor != hasCompletion || hasPredecessor != (record.PredecessorCompletionSHA != "") {
		return Record{}, ErrInvalid
	}
	if !hasPredecessor {
		return record, nil
	}
	if len(completion) == 0 || len(completion) > 256 || hash(completion) != record.PredecessorCompletionSHA {
		return Record{}, ErrInvalid
	}
	previous, err := Decode(predecessor)
	if err != nil {
		return Record{}, ErrInvalid
	}
	expectedCompletion, err := Completion(predecessor)
	previousConfig, _ := connectedprofile.Encode(previous.Next)
	currentConfig, _ := connectedprofile.Encode(record.Previous)
	if err != nil || !bytes.Equal(completion, expectedCompletion) ||
		!bytes.Equal(previousConfig, currentConfig) ||
		previous.NextResources != record.PreviousResources ||
		previous.ContractSHA256 != record.ContractSHA256 ||
		previous.PreviousCodeAfter != record.PreviousCodeBefore {
		return Record{}, ErrInvalid
	}
	return record, nil
}
