package history

import (
	"errors"
	"sort"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

const logRetention = 7 * 24 * time.Hour

var errLogCoverage = errors.New("invalid log coverage evidence")

// A blank reason is positive evidence. UNKNOWN is absence, never a stored row.
// Segments use nanosecond-precise half-open intervals; projection alone reduces
// them to conservative whole seconds for the public contract.
type logCoverageSegment struct {
	Start, End time.Time
	Reason     logobs.ReasonCode
}

func validLogTime(t time.Time) bool {
	return !t.IsZero() && t.Location() == time.UTC && t.Year() >= 1 && t.Year() <= 9999
}

func validLogSegment(s logCoverageSegment) bool {
	if !validLogTime(s.Start) || !validLogTime(s.End) || !s.Start.Before(s.End) {
		return false
	}
	if s.Reason == "" {
		return true
	}
	_, ok := logobs.HistoricalReasonRank(s.Reason)
	return ok && s.Reason != logobs.ReasonNotYetObserved
}

func deriveLogCoverage(cp logobs.Checkpoint, b logobs.Batch) ([]logCoverageSegment, error) {
	if err := b.Validate(); err != nil {
		return nil, errLogCoverage
	}
	if err := (logobs.ReadRequest{Source: b.Source, Checkpoint: cp, QueryStartedAt: b.QueryStartedAt}).Validate(); err != nil {
		return nil, errLogCoverage
	}
	if cp.Revision != b.ExpectedRevision {
		return nil, logobs.ErrRevisionConflict
	}
	if cp.ResetPending && b.Kind == logobs.BatchNormal {
		return nil, errLogCoverage
	}
	if !cp.ResetPending && len(cp.Opaque) == 0 && b.Kind != logobs.BatchNormal {
		return nil, errLogCoverage
	}
	// Native adapters attribute invalid future timestamps to q as known discards.
	// Recheck here so a bad adapter cannot create unbounded future-retention rows.
	for _, event := range b.Events {
		if event.ObservedAt.After(b.QueryStartedAt.Add(2 * time.Second)) {
			return nil, errLogCoverage
		}
	}
	for _, discard := range b.Discards {
		if discard.At.After(b.QueryStartedAt.Add(2 * time.Second)) {
			return nil, errLogCoverage
		}
	}
	q := b.QueryStartedAt
	start := q.Add(-5 * time.Minute)
	if cp.PreviousAttemptAt != nil {
		start = *cp.PreviousAttemptAt
	}
	floor := q.Add(-logRetention)
	if start.Before(floor) {
		start = floor
	}
	var out []logCoverageSegment
	if b.Kind == logobs.BatchNormal && b.CaughtUp {
		if cp.PreviousAttemptAt != nil && start.Before(q.Add(-time.Minute)) {
			out = append(out, logCoverageSegment{start, q.Add(-time.Minute), logobs.ReasonMissedCollection})
			start = q.Add(-time.Minute)
		}
		out = append(out, logCoverageSegment{start, q, ""})
	} else {
		if b.ReasonCode == nil {
			return nil, errLogCoverage
		}
		reason := *b.ReasonCode
		switch reason {
		case logobs.ReasonNoVisibleJournal, logobs.ReasonLogHelperUnavailable, logobs.ReasonLogHelperMismatch, logobs.ReasonPlatformUnsupported:
			reason = logobs.ReasonReaderFailed
		}
		out = append(out, logCoverageSegment{start, q, reason})
	}
	for _, s := range out {
		if !validLogSegment(s) {
			return nil, errLogCoverage
		}
	}
	return out, nil
}

// mergeLogCoverage preserves every explicit gap, choosing the closed highest
// precedence reason on overlap. It clips before projection and never aliases
// caller-owned slices. Its input is a single source's sorted disjoint evidence.
func mergeLogCoverage(old, additions []logCoverageSegment, floor, end time.Time) ([]logCoverageSegment, error) {
	if !validLogTime(floor) || !validLogTime(end) || !floor.Before(end) || end.Sub(floor) > logRetention || len(additions) > logobs.MaxCoverageSegmentsPerCommit {
		return nil, errLogCoverage
	}
	var current []logCoverageSegment
	for i, s := range old {
		if !validLogSegment(s) || i > 0 && old[i-1].End.After(s.Start) {
			return nil, errLogCoverage
		}
		if s.Start.Before(floor) {
			s.Start = floor
		}
		if s.End.After(end) {
			s.End = end
		}
		if s.Start.Before(s.End) {
			current = appendLogSegment(current, s)
		}
	}
	for _, addition := range additions {
		if !validLogSegment(addition) {
			return nil, errLogCoverage
		}
		if addition.Start.Before(floor) {
			addition.Start = floor
		}
		if addition.End.After(end) {
			addition.End = end
		}
		if !addition.Start.Before(addition.End) {
			continue
		}
		bounds := make([]time.Time, 0, 2*len(current)+2)
		for _, s := range current {
			bounds = append(bounds, s.Start, s.End)
		}
		bounds = append(bounds, addition.Start, addition.End)
		sort.Slice(bounds, func(i, j int) bool { return bounds[i].Before(bounds[j]) })
		var merged []logCoverageSegment
		position := 0
		for i := 0; i+1 < len(bounds); i++ {
			left, right := bounds[i], bounds[i+1]
			if !left.Before(right) {
				continue
			}
			for position < len(current) && !current[position].End.After(left) {
				position++
			}
			var chosen *logCoverageSegment
			if position < len(current) && !left.Before(current[position].Start) {
				s := current[position]
				chosen = &s
			}
			if !left.Before(addition.Start) && left.Before(addition.End) {
				if chosen == nil || strongerLogReason(addition.Reason, chosen.Reason) {
					s := addition
					chosen = &s
				}
			}
			if chosen != nil {
				merged = appendLogSegment(merged, logCoverageSegment{left, right, chosen.Reason})
			}
		}
		current = merged
	}
	return current, nil
}

func strongerLogReason(candidate, existing logobs.ReasonCode) bool {
	if candidate == "" {
		return false
	}
	if existing == "" {
		return true
	}
	a, _ := logobs.HistoricalReasonRank(candidate)
	b, _ := logobs.HistoricalReasonRank(existing)
	return a < b
}

func appendLogSegment(out []logCoverageSegment, s logCoverageSegment) []logCoverageSegment {
	if len(out) > 0 && out[len(out)-1].End.Equal(s.Start) && out[len(out)-1].Reason == s.Reason {
		out[len(out)-1].End = s.End
		return out
	}
	return append(out, s)
}

// projectLogCoverage counts only complete UTC-aligned one-second cells proven
// covered. A fractional gap can never disappear through duration rounding.
// Counts are attached separately: captured records cannot upgrade coverage.
func projectLogCoverage(segments []logCoverageSegment, start, end time.Time) (logobs.SummaryBucket, error) {
	b := logobs.SummaryBucket{At: start}
	if !validLogTime(start) || !validLogTime(end) || start.Nanosecond() != 0 || end.Nanosecond() != 0 || !start.Before(end) || end.Sub(start) > time.Hour {
		return b, errLogCoverage
	}
	reason := logobs.ReasonNotYetObserved
	var covered time.Duration
	coalesced, err := mergeLogCoverage(segments, nil, start, end)
	if err != nil {
		return logobs.SummaryBucket{}, err
	}
	for _, s := range coalesced {
		left, right := s.Start, s.End
		if left.Before(start) {
			left = start
		}
		if right.After(end) {
			right = end
		}
		if s.Reason != "" {
			if strongerLogReason(s.Reason, reason) {
				reason = s.Reason
			}
			continue
		}
		if left.Nanosecond() != 0 {
			left = left.Add(time.Second - time.Duration(left.Nanosecond()))
		}
		right = right.Add(-time.Duration(right.Nanosecond()))
		if left.Before(right) {
			covered += right.Sub(left)
		}
	}
	b.CoveredSeconds = uint32(covered / time.Second)
	switch {
	case covered == end.Sub(start):
		b.CoverageState = logobs.CoverageFull
		b.Counts = &logobs.BucketCounts{}
	case covered > 0:
		b.CoverageState = logobs.CoveragePartial
		b.ReasonCode = &reason
		b.Counts = &logobs.BucketCounts{}
	case reason != logobs.ReasonNotYetObserved:
		b.CoverageState = logobs.CoverageGapState
		b.ReasonCode = &reason
	default:
		b.CoverageState = logobs.CoverageUnknown
		b.ReasonCode = &reason
	}
	return b, nil
}
