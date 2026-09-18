package connectedtransition

import (
	"bytes"
	"encoding/json"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/ledgeridentity"
)

const PreparationVersion = "observer-connected-transition-preparing/v1"
const MaxPreparationBytes = 4 << 10

// Preparation is private, bounded intent evidence. It is not an authoritative
// transition journal, a cleanup permit, or worker activation authority.
type Preparation struct {
	Version                  string                  `json:"version"`
	TransitionSHA256         string                  `json:"transition_sha256"`
	Previous                 connectedprofile.Config `json:"previous"`
	PreviousResources        Resources               `json:"previous_resources"`
	PredecessorFormat        PredecessorFormat       `json:"predecessor_format"`
	PredecessorCompletionSHA string                  `json:"predecessor_completion_sha256"`
	Ledger                   Ledger                  `json:"ledger"`
}

// Prepare derives a witness from a proposed, canonical transition record.
// The caller must durably retain and verify the actual record before writes.
func Prepare(record Record) (Preparation, error) {
	encoded, err := Encode(record)
	if err != nil {
		return Preparation{}, ErrInvalid
	}
	return Preparation{
		Version: PreparationVersion, TransitionSHA256: hash(encoded),
		Previous: record.Previous, PreviousResources: record.PreviousResources,
		PredecessorFormat:        record.PredecessorFormat,
		PredecessorCompletionSHA: record.PredecessorCompletionSHA,
		Ledger:                   record.Ledger,
	}, nil
}

func (p Preparation) Validate() error {
	previous, err := connectedprofile.Encode(p.Previous)
	if err != nil || p.Version != PreparationVersion ||
		!digest(p.TransitionSHA256, false) ||
		!validPredecessor(p.PredecessorFormat, p.PredecessorCompletionSHA) ||
		(p.PredecessorFormat == PredecessorNone && p.Previous.PolicyGeneration != 1) ||
		!p.PreviousResources.valid() ||
		p.PreviousResources.InstalledConfig != hash(previous) ||
		p.PreviousResources.Hosts != hosts(p.Previous.Addresses) ||
		!digest(p.Ledger.LogicalSHA256, false) ||
		ledgeridentity.Validate(p.Ledger.Physical.witness()) != nil {
		return ErrInvalid
	}
	return nil
}

func EncodePreparation(p Preparation) ([]byte, error) {
	if p.Validate() != nil {
		return nil, ErrInvalid
	}
	encoded, err := json.Marshal(p)
	if err != nil || len(encoded) == 0 || len(encoded) > MaxPreparationBytes {
		return nil, ErrInvalid
	}
	return encoded, nil
}

func DecodePreparation(data []byte) (Preparation, error) {
	if len(data) == 0 || len(data) > MaxPreparationBytes {
		return Preparation{}, ErrInvalid
	}
	var p Preparation
	if json.Unmarshal(data, &p) != nil {
		return Preparation{}, ErrInvalid
	}
	canonical, err := EncodePreparation(p)
	if err != nil || !bytes.Equal(canonical, data) {
		return Preparation{}, ErrInvalid
	}
	return p, nil
}

// MatchesProposal compares a decoded witness with the exact planned record.
// It does not establish the provenance or durability of either input.
func MatchesProposal(preparation Preparation, record Record) error {
	expected, err := Prepare(record)
	if err != nil {
		return ErrInvalid
	}
	gotBytes, err := EncodePreparation(preparation)
	if err != nil {
		return ErrInvalid
	}
	expectedBytes, err := EncodePreparation(expected)
	if err != nil || !bytes.Equal(gotBytes, expectedBytes) {
		return ErrInvalid
	}
	return nil
}
