// Package connectedstatus holds bounded operational state, never upload authority.
package connectedstatus

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"
)

var ErrUnsafe = errors.New("connected_status_unavailable")

// FreshnessWindow matches the hosted observation status, not C1's longer
// admission/duplicate-delivery window. An acknowledged snapshot can be stale.
const FreshnessWindow = 45 * time.Second

type Record struct {
	Version          string    `json:"version"`
	InvocationID     string    `json:"invocation_id,omitempty"`
	State            string    `json:"state"`
	UpdatedAt        time.Time `json:"updated_at"`
	AcknowledgedAt   time.Time `json:"acknowledged_at"`
	CollectedAt      time.Time `json:"collected_at"`
	Steps            uint64    `json:"steps"`
	Attempts         uint64    `json:"attempts"`
	Acknowledgements uint64    `json:"acknowledgements"`
}

func Decode(data []byte) (Record, error) {
	var r Record
	if len(data) > 1024 || json.Unmarshal(data, &r) != nil {
		return Record{}, ErrUnsafe
	}
	canonical, err := Encode(r)
	if err != nil || !bytes.Equal(canonical, data) {
		return Record{}, ErrUnsafe
	}
	return r, nil
}

func Encode(r Record) ([]byte, error) {
	if r.InvocationID != "" {
		if len(r.InvocationID) != 32 {
			return nil, ErrUnsafe
		}
		for _, ch := range r.InvocationID {
			if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
				return nil, ErrUnsafe
			}
		}
	}
	if r.CollectedAt.Year() < 1 || r.CollectedAt.Year() > 9999 {
		return nil, ErrUnsafe
	}
	if r.Version != "observer-connected-status/v1" || r.UpdatedAt.IsZero() || r.UpdatedAt.Year() < 1 || r.UpdatedAt.Year() > 9999 || r.AcknowledgedAt.Year() < 1 || r.AcknowledgedAt.Year() > 9999 {
		return nil, ErrUnsafe
	}
	switch r.State {
	case "COLLECTING", "WAITING_FIRST_UPLOAD", "PENDING", "RETRYING", "RATE_LIMITED", "CREDENTIAL_REJECTED", "RECOVERY_REQUIRED", "ACKNOWLEDGED_FRESH", "ACKNOWLEDGED_STALE", "SOURCE_UNAVAILABLE", "INVALID_SNAPSHOT", "STOPPED":
	default:
		return nil, ErrUnsafe
	}
	b, err := json.Marshal(r)
	if err != nil || len(b) > 1024 {
		return nil, ErrUnsafe
	}
	return b, nil
}
