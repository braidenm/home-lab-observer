//go:build linux

// Package connectedtransitionview joins read-only transition evidence. It has
// no lease, manager, ledger-read, filesystem-write, or activation authority.
package connectedtransitionview

import (
	"bytes"
	"errors"

	"github.com/braidenm/home-lab-observer/internal/connectedactive"
	"github.com/braidenm/home-lab-observer/internal/connectedresources"
	"github.com/braidenm/home-lab-observer/internal/connectedstagefs"
	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
)

var ErrUnavailable = errors.New("connected_transition_view_unavailable")

// Classify expects independently anchored stage, authority and active reads,
// plus an independently measured private logical/physical ledger witness. A
// phase is diagnostic only: it grants no abort, cleanup, replacement or start.
func Classify(
	stage connectedstagefs.Snapshot,
	authority connectedstagefs.Authority,
	active connectedresources.Set,
	ledger connectedtransition.Ledger,
) (connectedtransition.StagedPhase, error) {
	proposal, ok := stage.Files[connectedtransition.StageProposalName]
	canonical, err := connectedtransition.Encode(stage.Record)
	if !ok || err != nil || !bytes.Equal(proposal, canonical) {
		return "", ErrUnavailable
	}
	pair, err := connectedresources.Derive(stage.Record,
		stage.Files[connectedtransition.StageOldCAName], stage.Files[connectedtransition.StageNewCAName])
	if err != nil {
		return "", ErrUnavailable
	}
	resources, err := connectedactive.ClassifyBytes(pair, active)
	if err != nil {
		return "", ErrUnavailable
	}
	phase, err := connectedtransition.ClassifyStaged(stage.Preparation, stage.Files, connectedtransition.StagedObserved{
		Journal: authority.Journal, Receipt: authority.Completion,
		Resources: resources, Ledger: ledger,
	})
	if err != nil {
		return "", ErrUnavailable
	}
	return phase, nil
}
