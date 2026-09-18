//go:build linux

package connectedstagefs

import (
	"bytes"
	"os"

	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
)

// InspectFirstPredecessorAt joins an anchored complete stage with the two
// installed legacy refresh names. It is only for the first new-format
// transition (none or legacy predecessor), and does not authorize publication,
// cleanup, replacement or worker startup. The caller holds the installation
// lease and independently proves stopped workers and the private ledger.
func InspectFirstPredecessorAt(config *os.File) (Snapshot, error) {
	stage, err := InspectAt(config)
	if err != nil {
		return Snapshot{}, err
	}
	if stage.Record.PredecessorFormat == connectedtransition.PredecessorTransition {
		return Snapshot{}, ErrUnsafe
	}
	legacy, err := ReadLegacyAt(config)
	if err != nil {
		return Snapshot{}, err
	}
	switch stage.Record.PredecessorFormat {
	case connectedtransition.PredecessorNone:
		if legacy.Journal != nil || legacy.Completion != nil {
			return Snapshot{}, ErrRecovery
		}
	case connectedtransition.PredecessorLegacy:
		if legacy.Journal == nil || legacy.Completion == nil ||
			!bytes.Equal(legacy.Journal, stage.Files[connectedtransition.StagePredecessorName]) ||
			!bytes.Equal(legacy.Completion, stage.Files[connectedtransition.StagePredecessorCompletionName]) {
			return Snapshot{}, ErrRecovery
		}
	default:
		return Snapshot{}, ErrRecovery
	}
	return stage, nil
}
