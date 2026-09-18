package connectedtransition

import "bytes"

// StagedPhase is private recovery evidence, never authority to abort, clean
// staging files, or start workers. The caller must independently verify the
// exact owned files, stopped workers, ledger and durable synchronization.
type StagedPhase string

const (
	StagedPreparationPending StagedPhase = "PREPARATION_PENDING"
	StagedForwardRecovery     StagedPhase = "FORWARD_RECOVERY_REQUIRED"
	StagedCleanupPending      StagedPhase = "COMPLETE_CLEANUP_PENDING"
)

type StagedObserved struct {
	// Nil means the corresponding fixed name is absent. A present empty file
	// is invalid, even though bytes.Equal considers it equal to nil.
	Journal   []byte
	Receipt   []byte
	Resources Resources
	Ledger    Ledger
}

// ClassifyStaged recognizes only the exact complete stage and prior/current
// authoritative bytes. In particular, an old receipt left after publishing a
// new journal is not a completion of the new transition.
func ClassifyStaged(preparation []byte, files map[string][]byte, observed StagedObserved) (StagedPhase, error) {
	record, err := ValidateStage(preparation, files)
	if err != nil || observed.Ledger != record.Ledger {
		return "", ErrInvalid
	}
	proposal := files[StageProposalName]
	predecessor, hasPredecessor := files[StagePredecessorName]
	predecessorReceipt := files[StagePredecessorCompletionName]

	if hasPredecessor && observed.Journal != nil && bytes.Equal(observed.Journal, predecessor) {
		if observed.Receipt == nil || !bytes.Equal(observed.Receipt, predecessorReceipt) ||
			observed.Resources != record.PreviousResources {
			return "", ErrInvalid
		}
		return StagedPreparationPending, nil
	}
	if !hasPredecessor && observed.Journal == nil {
		if observed.Receipt != nil || observed.Resources != record.PreviousResources {
			return "", ErrInvalid
		}
		return StagedPreparationPending, nil
	}
	if observed.Journal == nil || !bytes.Equal(observed.Journal, proposal) {
		return "", ErrInvalid
	}
	receipt := observed.Receipt
	if hasPredecessor && receipt == nil {
		return "", ErrInvalid
	}
	if hasPredecessor && receipt != nil && bytes.Equal(receipt, predecessorReceipt) {
		receipt = nil // Retained old receipt is not completion of this proposal.
	}
	result, err := Classify(record, Observed{Resources: observed.Resources, Ledger: observed.Ledger, Receipt: receipt})
	if err != nil {
		return "", ErrInvalid
	}
	if result == Complete {
		return StagedCleanupPending, nil
	}
	return StagedForwardRecovery, nil
}
