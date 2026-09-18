package connectedtransition

import (
	"bytes"
	"encoding/json"
	"math"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

const legacyRefreshVersion = "observer-connected-refresh/v1"
const maxLegacyRefreshBytes = 8 << 10
const maxLegacyCompletionBytes = 256

type legacyRefresh struct {
	Version  string                  `json:"version"`
	Previous connectedprofile.Config `json:"previous"`
	Next     connectedprofile.Config `json:"next"`
}

func decodeLegacyRefresh(data []byte) (legacyRefresh, error) {
	var record legacyRefresh
	if len(data) == 0 || len(data) > maxLegacyRefreshBytes ||
		json.Unmarshal(data, &record) != nil ||
		record.Version != legacyRefreshVersion ||
		record.Previous.Validate() != nil || record.Next.Validate() != nil ||
		record.Previous.PolicyGeneration == math.MaxUint64 ||
		record.Next.PolicyGeneration != record.Previous.PolicyGeneration+1 {
		return legacyRefresh{}, ErrInvalid
	}
	want := record.Previous
	want.PolicyGeneration = record.Next.PolicyGeneration
	want.Addresses = record.Next.Addresses
	wantBytes, _ := connectedprofile.Encode(want)
	nextBytes, _ := connectedprofile.Encode(record.Next)
	canonical, err := json.Marshal(record)
	if err != nil || !bytes.Equal(wantBytes, nextBytes) || !bytes.Equal(canonical, data) {
		return legacyRefresh{}, ErrInvalid
	}
	return record, nil
}

// AdmitCompletedLegacy returns only a predecessor receipt digest. The caller
// must separately prove exact root-owned files, stopped workers and the
// current installed resources; this pure check grants no transition authority.
func AdmitCompletedLegacy(journal, receipt []byte, current connectedprofile.Config) (string, error) {
	record, err := decodeLegacyRefresh(journal)
	if err != nil || len(receipt) == 0 || len(receipt) > maxLegacyCompletionBytes ||
		current.Validate() != nil {
		return "", ErrInvalid
	}
	currentBytes, _ := connectedprofile.Encode(current)
	nextBytes, _ := connectedprofile.Encode(record.Next)
	expected := []byte("observer-connected-refresh-complete/v1\n" + hash(journal) + "\n")
	if !bytes.Equal(currentBytes, nextBytes) || !bytes.Equal(receipt, expected) {
		return "", ErrInvalid
	}
	return hash(receipt), nil
}
