package logobs

import (
	"testing"
	"time"
)

func TestAggregateStatusFrozenMatrix(t *testing.T) {
	at := time.Date(2026, 9, 10, 4, 0, 0, 0, time.UTC)
	ok := Status{SupportState: SupportSupported, CollectionState: CollectionOK, Freshness: FreshnessCurrent, ObservedAt: &at, AttemptedAt: &at}
	backlog := ReasonBacklogDeferred
	partial := Status{SupportState: SupportSupported, CollectionState: CollectionPartial, Freshness: FreshnessUnknown, AttemptedAt: &at, ReasonCode: &backlog}
	permission := ReasonPermissionDenied
	denied := Status{SupportState: SupportPermissionDenied, CollectionState: CollectionNotRun, Freshness: FreshnessUnknown, AttemptedAt: &at, ReasonCode: &permission}
	unsupportedReason := ReasonPlatformUnsupported
	unsupported := Status{SupportState: SupportUnsupported, CollectionState: CollectionNotRun, Freshness: FreshnessUnknown, AttemptedAt: &at, ReasonCode: &unsupportedReason}

	tests := []struct {
		name       string
		statuses   []Status
		support    SupportState
		collection CollectionState
		freshness  Freshness
		reason     *ReasonCode
	}{
		{"disabled", nil, SupportDisabled, CollectionNotRun, FreshnessUnknown, ptrReason(ReasonLogSourcesDisabled)},
		{"singleton ok", []Status{ok}, SupportSupported, CollectionOK, FreshnessCurrent, nil},
		{"singleton partial", []Status{partial}, SupportSupported, CollectionPartial, FreshnessUnknown, ptrReason(ReasonSourcePartial)},
		{"supported and denied", []Status{ok, denied}, SupportSupported, CollectionPartial, FreshnessCurrent, ptrReason(ReasonSourcePartial)},
		{"different unsupported states", []Status{denied, unsupported}, SupportUnavailable, CollectionNotRun, FreshnessUnknown, ptrReason(ReasonSourcePartial)},
		{"same unsupported outcome", []Status{denied, denied}, SupportPermissionDenied, CollectionNotRun, FreshnessUnknown, &permission},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := AggregateStatus(test.statuses)
			if err != nil {
				t.Fatal(err)
			}
			if got.SupportState != test.support || got.CollectionState != test.collection || got.Freshness != test.freshness || !equalReasons(got.ReasonCode, test.reason) {
				t.Fatalf("AggregateStatus() = %+v", got)
			}
			if err := got.Validate(); err != nil {
				t.Fatalf("aggregate is invalid: %v", err)
			}
		})
	}
}

func TestAggregateStatusUsesLatestTimesAndReturnsClones(t *testing.T) {
	old := time.Date(2026, 9, 10, 3, 0, 0, 0, time.UTC)
	newer := old.Add(time.Minute)
	reason := ReasonReaderFailed
	left := Status{
		SupportState: SupportUnavailable, CollectionState: CollectionFailed, Freshness: FreshnessStale,
		ObservedAt: &old, AttemptedAt: &newer, CoverageThrough: &old, ReasonCode: &reason,
	}
	right := left.Clone()
	right.ObservedAt = &newer
	right.AttemptedAt = &newer
	right.CoverageThrough = &newer
	got, err := AggregateStatus([]Status{left, right})
	if err != nil {
		t.Fatal(err)
	}
	if got.ObservedAt == nil || !got.ObservedAt.Equal(newer) || got.CoverageThrough == nil || !got.CoverageThrough.Equal(newer) {
		t.Fatalf("aggregate times = %+v", got)
	}
	*got.ObservedAt = time.Time{}
	if left.ObservedAt.IsZero() || right.ObservedAt.IsZero() {
		t.Fatal("aggregate retained caller time pointer")
	}

	if _, err := AggregateStatus([]Status{{}}); err == nil {
		t.Fatal("invalid input status accepted")
	}
	if _, err := AggregateStatus([]Status{left, right, left}); err == nil {
		t.Fatal("oversized status set accepted")
	}
}
