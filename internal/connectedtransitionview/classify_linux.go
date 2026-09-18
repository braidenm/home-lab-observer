//go:build linux

// Package connectedtransitionview joins detached transition evidence without
// importing stage filesystem, lease, manager, ledger-read, write, or activation
// authorities.
package connectedtransitionview

import (
	"bytes"
	"errors"

	"github.com/braidenm/home-lab-observer/internal/connectedactive"
	"github.com/braidenm/home-lab-observer/internal/connectedresources"
	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
)

var ErrUnavailable = errors.New("connected_transition_view_unavailable")

// Evidence contains detached bytes and a private ledger witness. Its caller
// independently establishes anchored reads, the installation lease, stopped
// workers, and the physical/logical ledger measurement.
type Evidence struct {
	Preparation []byte
	Files       map[string][]byte
	Record      connectedtransition.Record
	Journal     []byte
	Completion  []byte
	Active      connectedresources.Set
	Ledger      connectedtransition.Ledger
}

// Classify returns a diagnostic phase only: it grants no abort, cleanup,
// replacement, or worker start.
func Classify(e Evidence) (connectedtransition.StagedPhase, error) {
	proposal, ok := e.Files[connectedtransition.StageProposalName]
	canonical, err := connectedtransition.Encode(e.Record)
	if !ok || err != nil || !bytes.Equal(proposal, canonical) {
		return "", ErrUnavailable
	}
	pair, err := connectedresources.Derive(e.Record,
		e.Files[connectedtransition.StageOldCAName], e.Files[connectedtransition.StageNewCAName])
	if err != nil {
		return "", ErrUnavailable
	}
	resources, err := connectedactive.ClassifyBytes(pair, e.Active)
	if err != nil {
		return "", ErrUnavailable
	}
	phase, err := connectedtransition.ClassifyStaged(e.Preparation, e.Files, connectedtransition.StagedObserved{
		Journal: e.Journal, Receipt: e.Completion,
		Resources: resources, Ledger: e.Ledger,
	})
	if err != nil {
		return "", ErrUnavailable
	}
	return phase, nil
}
