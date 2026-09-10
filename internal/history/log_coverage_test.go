package history

import (
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

var logTestNow = time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)

func logSuccess(q time.Time) logobs.Batch {
	return logobs.Batch{Kind: logobs.BatchNormal, Source: logobs.SourceSystem,
		QueryStartedAt: q, StartedAt: q, FinishedAt: q,
		SupportState: logobs.SupportSupported, CollectionState: logobs.CollectionOK, CaughtUp: true}
}

func TestLogCoverageAttemptWindows(t *testing.T) {
	q := logTestNow
	for _, tc := range []struct {
		name  string
		prior *time.Time
		want  []logCoverageSegment
	}{
		{"first", nil, []logCoverageSegment{{q.Add(-5 * time.Minute), q, ""}}},
		{"on cadence", timePtr(q.Add(-time.Minute)), []logCoverageSegment{{q.Add(-time.Minute), q, ""}}},
		{"early", timePtr(q.Add(-30 * time.Second)), []logCoverageSegment{{q.Add(-30 * time.Second), q, ""}}},
		{"missed polls", timePtr(q.Add(-10 * time.Minute)), []logCoverageSegment{
			{q.Add(-10 * time.Minute), q.Add(-time.Minute), logobs.ReasonMissedCollection}, {q.Add(-time.Minute), q, ""}}},
		{"long shutdown", timePtr(q.Add(-30 * 24 * time.Hour)), []logCoverageSegment{
			{q.Add(-7 * 24 * time.Hour), q.Add(-time.Minute), logobs.ReasonMissedCollection}, {q.Add(-time.Minute), q, ""}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cp := logobs.Checkpoint{}
			b := logSuccess(q)
			if tc.prior != nil {
				cp.Revision = 1
				cp.PreviousAttemptAt = tc.prior
				b.ExpectedRevision = 1
			}
			got, err := deriveLogCoverage(cp, b)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, %v; want %#v", got, err, tc.want)
			}
		})
	}
}

func TestLogCoverageFailureAndResetNeverProveCoverage(t *testing.T) {
	for _, reason := range []logobs.ReasonCode{logobs.ReasonReaderFailed, logobs.ReasonDeadlineExceeded,
		logobs.ReasonNoVisibleJournal, logobs.ReasonLogHelperUnavailable, logobs.ReasonLogHelperMismatch, logobs.ReasonPlatformUnsupported} {
		b := logSuccess(logTestNow)
		b.CaughtUp = false
		b.ReasonCode = &reason
		b.SupportState = logobs.SupportUnavailable
		b.CollectionState = logobs.CollectionNotRun
		if reason == logobs.ReasonDeadlineExceeded {
			b.SupportState = logobs.SupportSupported
			b.CollectionState = logobs.CollectionFailed
		}
		if reason == logobs.ReasonPlatformUnsupported {
			b.SupportState = logobs.SupportUnsupported
		}
		got, err := deriveLogCoverage(logobs.Checkpoint{}, b)
		wantReason := reason
		if reason != logobs.ReasonDeadlineExceeded {
			wantReason = logobs.ReasonReaderFailed
		}
		if err != nil || len(got) != 1 || got[0].Reason != wantReason {
			t.Fatalf("%s: %#v %v", reason, got, err)
		}
	}
	q := logTestNow
	cp := logobs.Checkpoint{Revision: 3, Opaque: []byte("synthetic-cursor"), PreviousAttemptAt: timePtr(q.Add(-10 * time.Minute)), CoverageThrough: timePtr(q.Add(-time.Hour))}
	b := logSuccess(q)
	b.ExpectedRevision = 3
	b.Kind = logobs.BatchResetEstablished
	b.CaughtUp = false
	b.CollectionState = logobs.CollectionPartial
	reason := logobs.ReasonCheckpointReset
	b.ReasonCode = &reason
	b.ProbeCount = 1
	b.ExaminedCount = 1
	b.NextOpaque = []byte("synthetic-tail")
	got, err := deriveLogCoverage(cp, b)
	want := []logCoverageSegment{{q.Add(-10 * time.Minute), q, reason}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("reset: %#v %v", got, err)
	}
}

func TestLogCoverageRejectsBadTransitions(t *testing.T) {
	q := logTestNow
	for _, tc := range []struct {
		name string
		cp   logobs.Checkpoint
		edit func(*logobs.Batch)
	}{
		{"revision", logobs.Checkpoint{}, func(b *logobs.Batch) { b.ExpectedRevision = 1 }},
		{"same time", logobs.Checkpoint{Revision: 1, PreviousAttemptAt: &q}, func(b *logobs.Batch) { b.ExpectedRevision = 1 }},
		{"rollback", logobs.Checkpoint{Revision: 1, PreviousAttemptAt: timePtr(q.Add(time.Second))}, func(b *logobs.Batch) { b.ExpectedRevision = 1 }},
		{"pending normal", logobs.Checkpoint{Revision: 1, ResetPending: true, PreviousAttemptAt: timePtr(q.Add(-time.Minute))}, func(b *logobs.Batch) { b.ExpectedRevision = 1 }},
		{"unrepresentable query time", logobs.Checkpoint{}, func(b *logobs.Batch) {
			b.QueryStartedAt = time.Date(0, 1, 1, 0, 1, 0, 0, time.UTC)
			b.StartedAt = b.QueryStartedAt
			b.FinishedAt = b.QueryStartedAt
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := logSuccess(q)
			tc.edit(&b)
			if _, err := deriveLogCoverage(tc.cp, b); err == nil {
				t.Fatal("accepted invalid transition")
			}
		})
	}
}

func TestLogCoverageStickyPrecedenceAndCoalescing(t *testing.T) {
	q := logTestNow
	at := func(n int) time.Time { return q.Add(time.Duration(n) * time.Second) }
	old := []logCoverageSegment{{at(0), at(10), ""}, {at(10), at(20), logobs.ReasonReaderFailed}, {at(20), at(30), ""}}
	before := append([]logCoverageSegment(nil), old...)
	got, err := mergeLogCoverage(old, []logCoverageSegment{{at(5), at(25), logobs.ReasonDeadlineExceeded}, {at(0), at(40), ""}}, at(0), at(40))
	want := []logCoverageSegment{{at(0), at(5), ""}, {at(5), at(25), logobs.ReasonDeadlineExceeded}, {at(25), at(40), ""}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v %v", got, err)
	}
	if !reflect.DeepEqual(old, before) {
		t.Fatal("mutated caller input")
	}
	got, err = mergeLogCoverage(got, nil, at(12), at(35))
	if err != nil || len(got) != 2 || !got[0].Start.Equal(at(12)) || !got[1].End.Equal(at(35)) {
		t.Fatalf("retention %#v %v", got, err)
	}
}

func TestLogCoverageMergeMatchesSyntheticCellOracle(t *testing.T) {
	r := rand.New(rand.NewSource(51))
	var segments []logCoverageSegment
	const cells = 120
	model := make([]*logobs.ReasonCode, cells)
	reasons := []logobs.ReasonCode{"", logobs.ReasonCheckpointReset, logobs.ReasonPermissionDenied, logobs.ReasonDeadlineExceeded,
		logobs.ReasonInvalidResponse, logobs.ReasonResponseTooLarge, logobs.ReasonReaderFailed, logobs.ReasonBacklogDeferred, logobs.ReasonMissedCollection}
	at := func(i int) time.Time { return logTestNow.Add(time.Duration(i) * time.Second) }
	for attempt := 0; attempt < 400; attempt++ {
		start := r.Intn(cells)
		end := start + 1 + r.Intn(cells-start)
		reason := reasons[r.Intn(len(reasons))]
		var err error
		segments, err = mergeLogCoverage(segments, []logCoverageSegment{{at(start), at(end), reason}}, at(0), at(cells))
		if err != nil {
			t.Fatal(err)
		}
		for i := start; i < end; i++ {
			if model[i] == nil {
				v := reason
				model[i] = &v
			} else {
				old := *model[i]
				rankNew, _ := logobs.HistoricalReasonRank(reason)
				rankOld, _ := logobs.HistoricalReasonRank(old)
				if reason != "" && (old == "" || rankNew < rankOld) {
					v := reason
					model[i] = &v
				}
			}
		}
		for i, want := range model {
			var got *logobs.ReasonCode
			for _, s := range segments {
				if !at(i).Before(s.Start) && at(i).Before(s.End) {
					v := s.Reason
					got = &v
					break
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("attempt %d cell %d mismatch", attempt, i)
			}
		}
		for i := 1; i < len(segments); i++ {
			if segments[i-1].End.After(segments[i].Start) || segments[i-1].End.Equal(segments[i].Start) && segments[i-1].Reason == segments[i].Reason {
				t.Fatal("not disjoint/coalesced")
			}
		}
	}
}

func TestLogCoverageProjectionConservativeWholeSeconds(t *testing.T) {
	q := logTestNow
	for _, tc := range []struct {
		name     string
		segments []logCoverageSegment
		seconds  uint32
		state    logobs.CoverageState
		reason   logobs.ReasonCode
	}{
		{"unknown", nil, 0, logobs.CoverageUnknown, logobs.ReasonNotYetObserved},
		{"full", []logCoverageSegment{{q, q.Add(time.Minute), ""}}, 60, logobs.CoverageFull, ""},
		{"fraction", []logCoverageSegment{{q.Add(time.Nanosecond), q.Add(time.Minute), ""}}, 59, logobs.CoveragePartial, logobs.ReasonNotYetObserved},
		{"subsecond proof", []logCoverageSegment{{q, q.Add(time.Second - time.Nanosecond), ""}}, 0, logobs.CoverageUnknown, logobs.ReasonNotYetObserved},
		{"tiny sticky gap", []logCoverageSegment{{q, q.Add(time.Nanosecond), logobs.ReasonReaderFailed}, {q.Add(time.Nanosecond), q.Add(time.Minute), ""}}, 59, logobs.CoveragePartial, logobs.ReasonReaderFailed},
		{"gap", []logCoverageSegment{{q, q.Add(time.Minute), logobs.ReasonCheckpointReset}}, 0, logobs.CoverageGapState, logobs.ReasonCheckpointReset},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := projectLogCoverage(tc.segments, q, q.Add(time.Minute))
			reason := logobs.ReasonCode("")
			if got.ReasonCode != nil {
				reason = *got.ReasonCode
			}
			if err != nil || got.CoveredSeconds != tc.seconds || got.CoverageState != tc.state || reason != tc.reason {
				t.Fatalf("%#v %v", got, err)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time { return &t }

func TestLogCoverageFutureAttributionBoundary(t *testing.T) {
	for _, delta := range []time.Duration{-30 * 24 * time.Hour, 2 * time.Second, 2*time.Second + time.Nanosecond} {
		for _, discarded := range []bool{false, true} {
			b := logSuccess(logTestNow)
			b.ExaminedCount = 1
			b.NextOpaque = []byte("synthetic-cursor")
			at := logTestNow.Add(delta)
			if discarded {
				b.CollectionState = logobs.CollectionPartial
				reason := logobs.ReasonInvalidResponse
				b.ReasonCode = &reason
				b.DiscardedCount = 1
				b.Discards = []logobs.DiscardCount{{At: at, Count: 1}}
			} else {
				b.Events = []logobs.Event{{ObservedAt: at, Source: b.Source, Severity: logobs.SeverityInfo, EventCode: "WIN_1"}}
			}
			_, err := deriveLogCoverage(logobs.Checkpoint{}, b)
			if (err != nil) != (delta > 2*time.Second) {
				t.Fatalf("delta=%s discard=%v err=%v", delta, discarded, err)
			}
		}
	}
}

func TestLogCoverageEveryReasonPairUsesClosedPrecedence(t *testing.T) {
	reasons := []logobs.ReasonCode{logobs.ReasonCheckpointReset, logobs.ReasonPermissionDenied,
		logobs.ReasonDeadlineExceeded, logobs.ReasonInvalidResponse, logobs.ReasonResponseTooLarge,
		logobs.ReasonReaderFailed, logobs.ReasonBacklogDeferred, logobs.ReasonMissedCollection}
	q := logTestNow
	end := q.Add(time.Minute)
	for i, left := range reasons {
		for j, right := range reasons {
			got, err := mergeLogCoverage([]logCoverageSegment{{q, end, left}}, []logCoverageSegment{{q, end, right}}, q, end)
			want := left
			if j < i {
				want = right
			}
			if err != nil || len(got) != 1 || got[0].Reason != want {
				t.Fatalf("%s / %s: %#v %v", left, right, got, err)
			}
		}
	}
}

func TestLogCoverageRejectsMalformedStoredEvidence(t *testing.T) {
	q := logTestNow
	end := q.Add(time.Minute)
	for _, segments := range [][]logCoverageSegment{
		{{q, q, ""}}, {{end, q, ""}}, {{q, end, logobs.ReasonNotYetObserved}},
		{{q, end, logobs.ReasonLogStorageUnavailable}}, {{q, end, logobs.ReasonSourcePartial}},
		{{q, end, "ARBITRARY"}}, {{q.In(time.FixedZone("UTC-lookalike", 0)), end, ""}},
		{{q, end, ""}, {q, end, logobs.ReasonReaderFailed}},
	} {
		if _, err := mergeLogCoverage(segments, nil, q, end); err == nil {
			t.Fatalf("accepted %#v", segments)
		}
	}
	if _, err := mergeLogCoverage(nil, nil, q, q.Add(logRetention+time.Nanosecond)); err == nil {
		t.Fatal("accepted unbounded horizon")
	}
	if _, err := mergeLogCoverage(nil, make([]logCoverageSegment, 3), q, end); err == nil {
		t.Fatal("accepted unbounded additions")
	}
}

func TestLogCoverageProjectionCoalescesAdjacentProofBeforeRounding(t *testing.T) {
	q := logTestNow
	end := q.Add(time.Minute)
	middle := q.Add(time.Second / 2)
	got, err := projectLogCoverage([]logCoverageSegment{{q, middle, ""}, {middle, end, ""}}, q, end)
	if err != nil || got.CoverageState != logobs.CoverageFull || got.CoveredSeconds != 60 {
		t.Fatalf("%#v %v", got, err)
	}
}

func TestLogCoverageInitializedEmptyCannotClaimReset(t *testing.T) {
	q := logTestNow
	cp := logobs.Checkpoint{Revision: 1, PreviousAttemptAt: timePtr(q.Add(-time.Minute))}
	for _, kind := range []logobs.BatchKind{logobs.BatchResetPending, logobs.BatchResetEstablished} {
		b := logSuccess(q)
		b.Kind = kind
		b.ExpectedRevision = 1
		b.CaughtUp = false
		b.CollectionState = logobs.CollectionPartial
		reason := logobs.ReasonCheckpointReset
		b.ReasonCode = &reason
		if kind == logobs.BatchResetEstablished {
			b.NextOpaque = []byte("synthetic-tail")
			b.ProbeCount = 1
			b.ExaminedCount = 1
		}
		if err := b.Validate(); err != nil {
			t.Fatal(err)
		}
		if _, err := deriveLogCoverage(cp, b); err == nil {
			t.Fatalf("accepted %s without prior cursor", kind)
		}
	}
}

func TestLogCoverageContinuousSuccessHasConstantRowCount(t *testing.T) {
	q := logTestNow
	var old []logCoverageSegment
	cp := logobs.Checkpoint{}
	for attempt := 0; attempt < 11000; attempt++ {
		b := logSuccess(q)
		b.ExpectedRevision = cp.Revision
		addition, err := deriveLogCoverage(cp, b)
		if err != nil {
			t.Fatal(err)
		}
		old, err = mergeLogCoverage(old, addition, q.Add(-logRetention), q)
		if err != nil {
			t.Fatal(err)
		}
		if len(old) != 1 {
			t.Fatalf("attempt %d has %d rows", attempt, len(old))
		}
		if old[0].Start.Before(q.Add(-logRetention)) {
			t.Fatal("escaped retention")
		}
		cp.Revision++
		cp.PreviousAttemptAt = timePtr(q)
		cp.CoverageThrough = timePtr(q)
		q = q.Add(time.Minute)
	}
}
