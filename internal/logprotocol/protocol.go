// Package logprotocol encodes the bounded private native-helper contract. It has
// no process, filesystem, network, or native-library role. Its wire payloads are
// private: unlike public log JSON, they contain continuation checkpoints.
package logprotocol

import (
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

const (
	Protocol         = "observer-log-helper/v1"
	MaxRequestBytes  = 32 << 10
	MaxResponseBytes = logobs.MaxSourceBytes
)

var (
	ErrInvalid          = errors.New("INVALID_LOG_HELPER_PROTOCOL")
	ErrIdentityMismatch = errors.New("LOG_HELPER_IDENTITY_MISMATCH")
	versionPattern      = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*$`)
	commitPattern       = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// Build is the trusted, exact parent/helper identity. It never selects a path or command.
type Build struct {
	Version string
	Commit  string
	OS      string
	Arch    string
}

func (b Build) valid() bool {
	return len(b.Version) <= 64 && versionPattern.MatchString(b.Version) && commitPattern.MatchString(b.Commit) &&
		(b.OS == "linux" || b.OS == "windows") && (b.Arch == "amd64" || b.Arch == "arm64")
}

func validRequest(build Build, request logobs.ReadRequest) bool {
	return build.valid() && request.Validate() == nil &&
		(request.Source == logobs.SourceSystem || build.OS == "windows")
}

func validResponse(build Build, request logobs.ReadRequest, batch logobs.Batch) error {
	if !validRequest(build, request) || batch.Validate() != nil {
		return ErrInvalid
	}
	if batch.Source != request.Source || batch.ExpectedRevision != request.Checkpoint.Revision || !batch.QueryStartedAt.Equal(request.QueryStartedAt) {
		return ErrIdentityMismatch
	}
	if request.Checkpoint.ResetPending && batch.Kind == logobs.BatchNormal {
		return ErrInvalid
	}
	if !request.Checkpoint.ResetPending && len(request.Checkpoint.Opaque) == 0 && batch.Kind != logobs.BatchNormal {
		return ErrInvalid
	}
	return nil
}

// EncodeRequest produces a private payload for an already selected fixed helper.
func EncodeRequest(build Build, request logobs.ReadRequest) ([]byte, error) {
	if !validRequest(build, request) {
		return nil, ErrInvalid
	}
	return encodeBounded(requestWire{
		Protocol: Protocol, Build: buildToWire(build), Source: request.Source,
		QueryStartedAt: formatTime(request.QueryStartedAt),
		Checkpoint: checkpointWire{Revision: request.Checkpoint.Revision, ResetPending: request.Checkpoint.ResetPending,
			Opaque: encodeCursor(request.Checkpoint.Opaque), PreviousAttemptAt: formatOptionalTime(request.Checkpoint.PreviousAttemptAt),
			CoverageThrough: formatOptionalTime(request.Checkpoint.CoverageThrough)},
	}, MaxRequestBytes)
}

// DecodeRequest accepts only a complete packet matching the helper's own trusted build.
func DecodeRequest(payload []byte, expected Build) (logobs.ReadRequest, error) {
	var zero logobs.ReadRequest
	if !expected.valid() || !validPacket(payload, MaxRequestBytes) || !requestShape(payload) {
		return zero, ErrInvalid
	}
	var wire requestWire
	if json.Unmarshal(payload, &wire) != nil {
		return zero, ErrInvalid
	}
	if wire.Protocol != Protocol || wire.Build.build() != expected {
		return zero, ErrIdentityMismatch
	}
	query, err := parseTime(wire.QueryStartedAt)
	if err != nil {
		return zero, ErrInvalid
	}
	opaque, err := decodeCursor(wire.Checkpoint.Opaque)
	if err != nil {
		return zero, ErrInvalid
	}
	previous, err := parseOptionalTime(wire.Checkpoint.PreviousAttemptAt)
	if err != nil {
		return zero, ErrInvalid
	}
	coverage, err := parseOptionalTime(wire.Checkpoint.CoverageThrough)
	if err != nil {
		return zero, ErrInvalid
	}
	request := logobs.ReadRequest{Source: wire.Source, QueryStartedAt: query,
		Checkpoint: logobs.Checkpoint{Revision: wire.Checkpoint.Revision, ResetPending: wire.Checkpoint.ResetPending,
			Opaque: opaque, PreviousAttemptAt: previous, CoverageThrough: coverage}}
	if !validRequest(expected, request) {
		return zero, ErrInvalid
	}
	return request, nil
}

// EncodeResponse validates the normalized result and original request correlation.
func EncodeResponse(build Build, request logobs.ReadRequest, batch logobs.Batch) ([]byte, error) {
	if err := validResponse(build, request, batch); err != nil {
		return nil, err
	}
	wire := batchToWire(batch)
	return encodeBounded(responseWire{Protocol: Protocol, Build: buildToWire(build), Batch: wire}, MaxResponseBytes)
}

// DecodeResponse returns no partial result if any packet or correlation check fails.
func DecodeResponse(payload []byte, expected Build, request logobs.ReadRequest) (logobs.Batch, error) {
	var zero logobs.Batch
	if !validRequest(expected, request) || !validPacket(payload, MaxResponseBytes) || !responseShape(payload) {
		return zero, ErrInvalid
	}
	var wire responseWire
	if json.Unmarshal(payload, &wire) != nil {
		return zero, ErrInvalid
	}
	if wire.Protocol != Protocol || wire.Build.build() != expected {
		return zero, ErrIdentityMismatch
	}
	batch, err := wire.Batch.batch()
	if err != nil {
		return zero, ErrInvalid
	}
	if err = validResponse(expected, request, batch); err != nil {
		return zero, err
	}
	return batch, nil
}

func encodeBounded(value any, maximum int) ([]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) > maximum {
		return nil, ErrInvalid
	}
	return payload, nil
}

func formatTime(value time.Time) string { return value.Format(time.RFC3339Nano) }
func formatOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := formatTime(*value)
	return &formatted
}
func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.IsZero() || parsed.Year() < 1 || parsed.Year() > 9999 || parsed.Location() != time.UTC || formatTime(parsed) != value {
		return time.Time{}, ErrInvalid
	}
	return parsed, nil
}
func parseOptionalTime(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := parseTime(*value)
	if err != nil {
		return nil, ErrInvalid
	}
	return &parsed, nil
}
