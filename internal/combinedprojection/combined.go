// Package combinedprojection defines an unused atomic host/workload body.
// It does not widen upload admission, installed policy, ledger limits or consent.
package combinedprojection

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

const (
	SchemaVersion = "home-lab-connected-observation/v1"
	Profile       = "numeric-host-workloads/v1"
	MaxBytes      = 288 * 1024
)

// ErrValidation is fixed: rejected values and decoder errors never escape.
var ErrValidation = errors.New("combined_projection_invalid_document")

type identity struct {
	Source  string `json:"source_id"`
	Version string `json:"collector_version"`
	At      string `json:"collected_at"`
}

type document struct {
	Schema    string          `json:"schema_version"`
	Profile   string          `json:"projection_profile"`
	Source    string          `json:"source_id"`
	Version   string          `json:"collector_version"`
	At        string          `json:"collected_at"`
	Host      json.RawMessage `json:"host_metrics"`
	Workloads json.RawMessage `json:"workload_inventory"`
}

// Encode binds two canonical object components to one enrolled identity/cycle.
// Component validators run before composition. Policy claims inside a component
// are not persisted consent, authentication, freshness or management permission.
func Encode(host, workloads []byte, expectedSource string) ([]byte, error) {
	if len(host) == 0 || len(host) > remoteprojection.MaxBytes || len(workloads) == 0 || len(workloads) > workloadprojection.MaxBytes {
		return nil, ErrValidation
	}
	if !utf8.Valid(host) || !utf8.Valid(workloads) || remoteprojection.ValidateNative(host, expectedSource) != nil || workloadprojection.Validate(workloads, expectedSource) != nil {
		return nil, ErrValidation
	}
	var h, w identity
	if json.Unmarshal(host, &h) != nil || json.Unmarshal(workloads, &w) != nil || h != w {
		return nil, ErrValidation
	}
	d := document{Schema: SchemaVersion, Profile: Profile, Source: h.Source, Version: h.Version, At: h.At,
		Host: json.RawMessage(host), Workloads: json.RawMessage(workloads)}
	b, err := json.Marshal(d)
	if err != nil || len(b) > MaxBytes {
		return nil, ErrValidation
	}
	return b, nil
}

// Validate accepts exactly canonical producer bytes for the expected identity.
// Sequence and retry digest are external metadata over these exact bytes.
func Validate(b []byte, expectedSource string) error {
	_, err := CollectionTime(b, expectedSource)
	return err
}

// CollectionTime returns the validated root cycle only. It is not a replacement
// for individual workload observation timestamps or a receipt-freshness check.
func CollectionTime(b []byte, expectedSource string) (time.Time, error) {
	if len(b) == 0 || len(b) > MaxBytes || !utf8.Valid(b) {
		return time.Time{}, ErrValidation
	}
	var d document
	if json.Unmarshal(b, &d) != nil || d.Schema != SchemaVersion || d.Profile != Profile {
		return time.Time{}, ErrValidation
	}
	canonical, err := Encode(d.Host, d.Workloads, expectedSource)
	if err != nil || !bytes.Equal(canonical, b) {
		return time.Time{}, ErrValidation
	}
	at, err := time.Parse(time.RFC3339Nano, d.At)
	if err != nil {
		return time.Time{}, ErrValidation
	}
	return at, nil
}
