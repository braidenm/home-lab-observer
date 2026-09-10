package logprotocol

import (
	"encoding/base64"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

type buildWire struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

func buildToWire(build Build) buildWire {
	return buildWire{build.Version, build.Commit, build.OS, build.Arch}
}
func (wire buildWire) build() Build { return Build{wire.Version, wire.Commit, wire.OS, wire.Arch} }

type checkpointWire struct {
	Revision          uint64  `json:"revision"`
	ResetPending      bool    `json:"reset_pending"`
	Opaque            *string `json:"opaque_base64"`
	PreviousAttemptAt *string `json:"previous_attempt_at"`
	CoverageThrough   *string `json:"coverage_through"`
}
type requestWire struct {
	Protocol       string         `json:"protocol"`
	Build          buildWire      `json:"build"`
	Source         logobs.Source  `json:"source"`
	QueryStartedAt string         `json:"query_started_at"`
	Checkpoint     checkpointWire `json:"checkpoint"`
}
type responseWire struct {
	Protocol string    `json:"protocol"`
	Build    buildWire `json:"build"`
	Batch    batchWire `json:"batch"`
}
type eventWire struct {
	ObservedAt string          `json:"observed_at"`
	Source     logobs.Source   `json:"source"`
	Severity   logobs.Severity `json:"severity"`
	EventCode  string          `json:"event_code"`
}
type discardWire struct {
	At    string `json:"at"`
	Count uint32 `json:"count"`
}
type batchWire struct {
	Kind             logobs.BatchKind       `json:"kind"`
	Source           logobs.Source          `json:"source"`
	ExpectedRevision uint64                 `json:"expected_revision"`
	QueryStartedAt   string                 `json:"query_started_at"`
	StartedAt        string                 `json:"started_at"`
	FinishedAt       string                 `json:"finished_at"`
	SupportState     logobs.SupportState    `json:"support_state"`
	CollectionState  logobs.CollectionState `json:"collection_state"`
	ReasonCode       *logobs.ReasonCode     `json:"reason_code"`
	Events           []eventWire            `json:"events"`
	Discards         []discardWire          `json:"discards"`
	ExaminedCount    uint32                 `json:"examined_count"`
	ProbeCount       uint32                 `json:"probe_count"`
	DiscardedCount   uint32                 `json:"discarded_count"`
	Deferred         bool                   `json:"deferred"`
	CaughtUp         bool                   `json:"caught_up"`
	NextOpaque       *string                `json:"next_opaque_base64"`
}

func batchToWire(batch logobs.Batch) batchWire {
	wire := batchWire{
		Kind: batch.Kind, Source: batch.Source, ExpectedRevision: batch.ExpectedRevision,
		QueryStartedAt: formatTime(batch.QueryStartedAt), StartedAt: formatTime(batch.StartedAt), FinishedAt: formatTime(batch.FinishedAt),
		SupportState: batch.SupportState, CollectionState: batch.CollectionState, ReasonCode: batch.ReasonCode,
		Events: make([]eventWire, len(batch.Events)), Discards: make([]discardWire, len(batch.Discards)),
		ExaminedCount: batch.ExaminedCount, ProbeCount: batch.ProbeCount, DiscardedCount: batch.DiscardedCount,
		Deferred: batch.Deferred, CaughtUp: batch.CaughtUp, NextOpaque: encodeCursor(batch.NextOpaque),
	}
	for i, event := range batch.Events {
		wire.Events[i] = eventWire{formatTime(event.ObservedAt), event.Source, event.Severity, event.EventCode}
	}
	for i, discard := range batch.Discards {
		wire.Discards[i] = discardWire{formatTime(discard.At), discard.Count}
	}
	return wire
}
func (wire batchWire) batch() (logobs.Batch, error) {
	var zero logobs.Batch
	query, err := parseTime(wire.QueryStartedAt)
	if err != nil {
		return zero, err
	}
	started, err := parseTime(wire.StartedAt)
	if err != nil {
		return zero, err
	}
	finished, err := parseTime(wire.FinishedAt)
	if err != nil {
		return zero, err
	}
	opaque, err := decodeCursor(wire.NextOpaque)
	if err != nil {
		return zero, err
	}
	batch := logobs.Batch{Kind: wire.Kind, Source: wire.Source, ExpectedRevision: wire.ExpectedRevision,
		QueryStartedAt: query, StartedAt: started, FinishedAt: finished,
		SupportState: wire.SupportState, CollectionState: wire.CollectionState, ReasonCode: wire.ReasonCode,
		Events: make([]logobs.Event, len(wire.Events)), Discards: make([]logobs.DiscardCount, len(wire.Discards)),
		ExaminedCount: wire.ExaminedCount, ProbeCount: wire.ProbeCount, DiscardedCount: wire.DiscardedCount,
		Deferred: wire.Deferred, CaughtUp: wire.CaughtUp, NextOpaque: opaque}
	for i, event := range wire.Events {
		observed, err := parseTime(event.ObservedAt)
		if err != nil {
			return zero, err
		}
		batch.Events[i] = logobs.Event{ObservedAt: observed, Source: event.Source, Severity: event.Severity, EventCode: event.EventCode}
	}
	for i, discard := range wire.Discards {
		at, err := parseTime(discard.At)
		if err != nil {
			return zero, err
		}
		batch.Discards[i] = logobs.DiscardCount{At: at, Count: discard.Count}
	}
	return batch, nil
}

func encodeCursor(value []byte) *string {
	if len(value) == 0 {
		return nil
	}
	encoded := base64.StdEncoding.EncodeToString(value)
	return &encoded
}
func decodeCursor(value *string) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	if len(*value) == 0 || len(*value) > base64.StdEncoding.EncodedLen(logobs.MaxCheckpointBytes) {
		return nil, ErrInvalid
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(*value)
	if err != nil || len(decoded) == 0 || len(decoded) > logobs.MaxCheckpointBytes || base64.StdEncoding.EncodeToString(decoded) != *value {
		return nil, ErrInvalid
	}
	return decoded, nil
}
