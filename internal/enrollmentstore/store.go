// Package enrollmentstore persists one-shot enrollment, without exchange or activation.
package enrollmentstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

var (
	ErrUnsupported = errors.New("enrollment_store_unsupported")
	ErrRecovery    = errors.New("enrollment_store_recovery_required")
	ErrUnsafe      = errors.New("enrollment_store_unsafe_storage")
	ErrBusy        = errors.New("enrollment_store_busy")
)

const (
	lockName       = ".enrollment-lock"
	stageName      = ".record.tmp"
	attemptName    = "attempt.json"
	credentialName = "credential.json"
	readyName      = "ready.json"
	ledgerName     = "ledger"
	maxRecord      = 512
)

var secretPattern = regexp.MustCompile(`^hlc_[A-Za-z0-9_-]{43}$`)

type record struct {
	Version     string `json:"version"`
	State       string `json:"state"`
	ServerID    string `json:"server_id"`
	ConnectorID string `json:"connector_id"`
	Secret      string `json:"secret,omitempty"`
}

func validBinding(binding uploadstate.Binding) bool {
	return uploadstate.ValidateRecord(uploadstate.Record{Binding: binding}, binding) == nil
}

func makeRecord(binding uploadstate.Binding, state string, secret []byte) record {
	return record{"observer-enrollment/v1", state, binding.ServerID, binding.ConnectorID, string(secret)}
}

func decode(data []byte, binding uploadstate.Binding, state string) (record, error) {
	var r record
	if len(data) == 0 || len(data) > maxRecord || !validBinding(binding) || json.Unmarshal(data, &r) != nil {
		return record{}, ErrRecovery
	}
	if r.Version != "observer-enrollment/v1" || r.State != state || r.ServerID != binding.ServerID || r.ConnectorID != binding.ConnectorID {
		return record{}, ErrRecovery
	}
	if (state == "CREDENTIAL" && (len(r.Secret) != 47 || !secretPattern.MatchString(r.Secret))) || (state != "CREDENTIAL" && r.Secret != "") {
		return record{}, ErrRecovery
	}
	canonical, err := json.Marshal(r)
	defer clear(canonical)
	if err != nil || !bytes.Equal(data, canonical) {
		return record{}, ErrRecovery
	}
	return r, nil
}
