package ledgerwitness

import (
	"bytes"
	"encoding/hex"
	"encoding/json"

	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

const ResultVersion = "observer-existing-ledger-result/v1"
const MaxResultBytes = 512

type privateResult struct {
	Version     string `json:"version"`
	ServerID    string `json:"server_id"`
	ConnectorID string `json:"connector_id"`
	Fingerprint string `json:"fingerprint_sha256"`
}

// EncodeResult is a private worker-pipe protocol, never operator diagnostics.
func EncodeResult(binding uploadstate.Binding, fingerprint [32]byte) ([]byte, error) {
	if uploadstate.ValidateRecord(uploadstate.Record{Binding: binding}, binding) != nil {
		return nil, ErrUnsafe
	}
	b, err := json.Marshal(privateResult{ResultVersion, binding.ServerID, binding.ConnectorID, hex.EncodeToString(fingerprint[:])})
	if err != nil || len(b) > MaxResultBytes {
		return nil, ErrUnsafe
	}
	return b, nil
}

// DecodeResult accepts only canonical output for the independently expected binding.
func DecodeResult(data []byte, expected uploadstate.Binding) ([32]byte, error) {
	var zero [32]byte
	var result privateResult
	if len(data) == 0 || len(data) > MaxResultBytes || json.Unmarshal(data, &result) != nil || result.Version != ResultVersion || result.ServerID != expected.ServerID || result.ConnectorID != expected.ConnectorID || len(result.Fingerprint) != 64 {
		return zero, ErrUnsafe
	}
	decoded, err := hex.DecodeString(result.Fingerprint)
	if err != nil || hex.EncodeToString(decoded) != result.Fingerprint {
		return zero, ErrUnsafe
	}
	var fingerprint [32]byte
	copy(fingerprint[:], decoded)
	canonical, err := EncodeResult(expected, fingerprint)
	if err != nil || !bytes.Equal(canonical, data) {
		return zero, ErrUnsafe
	}
	return fingerprint, nil
}
