package logobs

import (
	"errors"
	"time"
)

func AggregateStatus(statuses []Status) (Status, error) {
	if len(statuses) > MaxSources {
		return Status{}, errors.New("too many statuses to aggregate")
	}
	for _, status := range statuses {
		if err := status.Validate(); err != nil {
			return Status{}, err
		}
	}
	if len(statuses) == 0 {
		reason := ReasonLogSourcesDisabled
		return Status{SupportState: SupportDisabled, CollectionState: CollectionNotRun, Freshness: FreshnessUnknown, ReasonCode: &reason}, nil
	}
	if len(statuses) == 1 {
		result := statuses[0].Clone()
		if result.CollectionState == CollectionPartial {
			reason := ReasonSourcePartial
			result.ReasonCode = &reason
		}
		return result, result.Validate()
	}

	left, right := statuses[0], statuses[1]
	result := Status{
		SupportState: aggregateSupport(left, right),
		Freshness:    aggregateFreshness(left, right),
		ObservedAt:   laterTime(left.ObservedAt, right.ObservedAt),
		AttemptedAt:  laterTime(left.AttemptedAt, right.AttemptedAt),
	}
	result.CollectionState = aggregateCollection(result.SupportState, left, right)
	result.CoverageThrough = laterTime(left.CoverageThrough, right.CoverageThrough)
	if result.CollectionState == CollectionOK {
		result.ReasonCode = nil
	} else if result.CollectionState == CollectionPartial || !sameOutcome(left, right) {
		reason := ReasonSourcePartial
		result.ReasonCode = &reason
	} else {
		result.ReasonCode = cloneReason(left.ReasonCode)
	}
	if result.Freshness == FreshnessUnknown {
		result.ObservedAt = nil
		result.CoverageThrough = nil
	}
	if err := result.Validate(); err != nil {
		return Status{}, err
	}
	return result, nil
}

func aggregateSupport(left, right Status) SupportState {
	if left.SupportState == SupportSupported || right.SupportState == SupportSupported {
		return SupportSupported
	}
	if left.SupportState == right.SupportState {
		return left.SupportState
	}
	return SupportUnavailable
}

func aggregateCollection(support SupportState, left, right Status) CollectionState {
	if support == SupportSupported {
		if left.SupportState != SupportSupported || right.SupportState != SupportSupported || left.CollectionState != right.CollectionState {
			return CollectionPartial
		}
		return left.CollectionState
	}
	if left.CollectionState == CollectionFailed || right.CollectionState == CollectionFailed {
		return CollectionFailed
	}
	return CollectionNotRun
}

func aggregateFreshness(left, right Status) Freshness {
	if left.ObservedAt == nil && right.ObservedAt == nil {
		return FreshnessUnknown
	}
	if (left.ObservedAt != nil && left.Freshness == FreshnessStale) || (right.ObservedAt != nil && right.Freshness == FreshnessStale) {
		return FreshnessStale
	}
	return FreshnessCurrent
}

func sameOutcome(left, right Status) bool {
	return left.SupportState == right.SupportState && left.CollectionState == right.CollectionState && equalReasons(left.ReasonCode, right.ReasonCode)
}

func equalReasons(left, right *ReasonCode) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func laterTime(left, right *time.Time) *time.Time {
	if left == nil {
		return cloneTime(right)
	}
	if right == nil || !right.After(*left) {
		return cloneTime(left)
	}
	return cloneTime(right)
}
