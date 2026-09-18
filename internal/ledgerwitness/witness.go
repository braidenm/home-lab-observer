// Package ledgerwitness compares existing logical upload state without advancing it.
// A witness is private comparison data, never an authorization or activation receipt.
package ledgerwitness

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash"
	"time"

	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

const Version = "observer-ledger-witness/v1"

var ErrUnsafe = errors.New("ledger_witness_unavailable")

// Fingerprint includes every logical field and never applies pending age expiry.
// The caller must retain exclusive access to the record while this function runs.
func Fingerprint(r uploadstate.Record, binding uploadstate.Binding) ([32]byte, error) {
	if uploadstate.ValidateRecord(r, binding) != nil {
		return [32]byte{}, ErrUnsafe
	}
	h := sha256.New()
	field(h, []byte(Version))
	field(h, []byte(r.Binding.ServerID))
	field(h, []byte(r.Binding.ConnectorID))
	number(h, uint64(r.Watermark))
	flag(h, r.HasAck)
	h.Write(r.LastAck[:])
	field(h, []byte(r.Stopped))
	flag(h, r.Pending != nil)
	if p := r.Pending; p != nil {
		number(h, uint64(p.Sequence))
		field(h, p.Body)
		h.Write(p.Digest[:])
		field(h, []byte(p.CollectedAt.UTC().Format(time.RFC3339Nano)))
	}
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result, nil
}

func field(h hash.Hash, value []byte) {
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(value)))
	h.Write(size[:])
	h.Write(value)
}

func number(h hash.Hash, value uint64) {
	var data [8]byte
	binary.BigEndian.PutUint64(data[:], value)
	h.Write(data[:])
}

func flag(h hash.Hash, value bool) {
	var data [1]byte
	if value {
		data[0] = 1
	}
	h.Write(data[:])
}
