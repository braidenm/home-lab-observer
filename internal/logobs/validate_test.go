package logobs

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var testTime = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

func TestEventValidateClosedVocabulary(t *testing.T) {
	valid := []string{
		"WIN_0",
		"WIN_4294967295",
		"WIN_0123456789abcdef0123456789abcdef_42",
		"SYSTEMD_0123456789abcdef0123456789abcdef",
		"SYSTEMD_PRIORITY_7",
	}
	for _, code := range valid {
		event := Event{ObservedAt: testTime, Source: SourceSystem, Severity: SeverityWarn, EventCode: code}
		if err := event.Validate(); err != nil {
			t.Fatalf("valid code %q rejected: %v", code, err)
		}
	}

	invalid := []string{
		"WIN_00",
		"WIN_4294967296",
		"WIN_0123456789ABCDEF0123456789abcdef_42",
		"WIN_provider_42",
		"SYSTEMD_0123456789abcdef0123456789abcde",
		"SYSTEMD_PRIORITY_8",
		strings.Join([]string{"github", "pat", ""}, "_") + strings.Repeat("A", 30),
	}
	for _, code := range invalid {
		event := Event{ObservedAt: testTime, Source: SourceSystem, Severity: SeverityWarn, EventCode: code}
		if err := event.Validate(); err == nil {
			t.Fatalf("invalid code %q accepted", code)
		}
	}

	badTime := Event{ObservedAt: testTime.In(time.FixedZone("zero", 0)), Source: SourceSystem, Severity: SeverityWarn, EventCode: "WIN_1"}
	if err := badTime.Validate(); err == nil {
		t.Fatal("non-UTC event time accepted")
	}
	badTime.ObservedAt = testTime
	badTime.Source = "security"
	if err := badTime.Validate(); err == nil {
		t.Fatal("unknown source accepted")
	}
	badTime.Source = SourceSystem
	badTime.Severity = "NOTICE"
	if err := badTime.Validate(); err == nil {
		t.Fatal("unknown severity accepted")
	}
}

func TestEnumAndReasonValidation(t *testing.T) {
	for _, validate := range []func() error{
		func() error { return SourceSystem.Validate() },
		func() error { return SeverityCritical.Validate() },
		func() error { return SupportPermissionDenied.Validate() },
		func() error { return CollectionPartial.Validate() },
		func() error { return FreshnessStale.Validate() },
		func() error { return BatchResetPending.Validate() },
		func() error { return CoverageGapState.Validate() },
		func() error { return ReasonNoVisibleJournal.Validate() },
	} {
		if err := validate(); err != nil {
			t.Fatalf("valid enum rejected: %v", err)
		}
	}
	for _, reason := range []ReasonCode{"", "lowercase", "HAS-DASH", "NEW_CODE", ReasonCode(strings.Repeat("A", 65))} {
		if err := reason.Validate(); err == nil {
			t.Fatalf("invalid reason %q accepted", reason)
		}
	}

	historical := []ReasonCode{
		ReasonCheckpointReset, ReasonPermissionDenied, ReasonDeadlineExceeded,
		ReasonInvalidResponse, ReasonResponseTooLarge, ReasonReaderFailed,
		ReasonBacklogDeferred, ReasonMissedCollection, ReasonNotYetObserved,
	}
	for want, reason := range historical {
		rank, ok := HistoricalReasonRank(reason)
		if !ok || int(rank) != want {
			t.Fatalf("rank(%q) = %d, %v; want %d, true", reason, rank, ok, want)
		}
	}
	for _, reason := range []ReasonCode{ReasonLogStorageUnavailable, ReasonNoVisibleJournal, ReasonLogHelperUnavailable, ReasonPlatformUnsupported} {
		if _, ok := HistoricalReasonRank(reason); ok {
			t.Fatalf("status-only reason %q accepted as historical", reason)
		}
	}
}

func TestCheckpointAndReadRequestValidation(t *testing.T) {
	previous := testTime.Add(-time.Minute)
	coverage := previous.Add(-time.Minute)
	valid := []Checkpoint{
		{},
		{Revision: 1, PreviousAttemptAt: &previous},
		{Revision: 2, ResetPending: true, PreviousAttemptAt: &previous, CoverageThrough: &coverage},
		{Revision: 3, Opaque: []byte("private"), PreviousAttemptAt: &previous, CoverageThrough: &coverage},
	}
	for _, checkpoint := range valid {
		if err := checkpoint.Validate(); err != nil {
			t.Fatalf("valid checkpoint rejected: %v", err)
		}
	}

	tooLarge := Checkpoint{Revision: 1, Opaque: make([]byte, MaxCheckpointBytes+1)}
	if err := tooLarge.Validate(); err == nil {
		t.Fatal("oversized checkpoint accepted")
	}
	for name, checkpoint := range map[string]Checkpoint{
		"pending opaque":        {Revision: 1, ResetPending: true, Opaque: []byte("private")},
		"pending initial":       {ResetPending: true},
		"revision without time": {Revision: 1},
		"initial durable":       {PreviousAttemptAt: &previous},
		"coverage without prev": {Revision: 1, CoverageThrough: &coverage},
		"coverage after prev":   {Revision: 1, PreviousAttemptAt: &coverage, CoverageThrough: &previous},
	} {
		if err := checkpoint.Validate(); err == nil {
			t.Fatalf("%s checkpoint accepted", name)
		}
	}

	request := ReadRequest{Source: SourceSystem, Checkpoint: valid[3], QueryStartedAt: testTime}
	if err := request.Validate(); err != nil {
		t.Fatalf("valid read request rejected: %v", err)
	}
	request.QueryStartedAt = previous
	if err := request.Validate(); err == nil {
		t.Fatal("non-advancing read request accepted")
	}
}

func TestStatusValidationRejectsFalseSuccessAndInvalidStatePairs(t *testing.T) {
	if err := successfulStatus().Validate(); err != nil {
		t.Fatalf("valid successful status rejected: %v", err)
	}
	storageReason := ReasonLogStorageUnavailable
	attempted := testTime
	storageFailure := Status{
		SupportState: SupportUnavailable, CollectionState: CollectionFailed, Freshness: FreshnessUnknown,
		AttemptedAt: &attempted, ReasonCode: &storageReason,
	}
	if err := storageFailure.Validate(); err != nil {
		t.Fatalf("summary storage overlay rejected: %v", err)
	}
	permissionReason := ReasonPermissionDenied
	permission := Status{
		SupportState: SupportPermissionDenied, CollectionState: CollectionNotRun, Freshness: FreshnessUnknown,
		AttemptedAt: &attempted, ReasonCode: &permissionReason,
	}
	if err := permission.Validate(); err != nil {
		t.Fatalf("permission status rejected: %v", err)
	}

	for name, status := range map[string]Status{
		"unknown successful": {
			SupportState: SupportSupported, CollectionState: CollectionOK, Freshness: FreshnessUnknown,
		},
		"disabled failed": {
			SupportState: SupportDisabled, CollectionState: CollectionFailed, Freshness: FreshnessUnknown, ReasonCode: &storageReason,
		},
		"permission current": {
			SupportState: SupportPermissionDenied, CollectionState: CollectionNotRun, Freshness: FreshnessCurrent,
			ObservedAt: &attempted, AttemptedAt: &attempted, ReasonCode: &permissionReason,
		},
		"failed current": {
			SupportState: SupportUnavailable, CollectionState: CollectionFailed, Freshness: FreshnessCurrent,
			ObservedAt: &attempted, AttemptedAt: &attempted, ReasonCode: &storageReason,
		},
	} {
		if err := status.Validate(); err == nil {
			t.Fatalf("%s status accepted", name)
		}
	}
}

func TestBatchValidationBoundsAndResetState(t *testing.T) {
	reasonBacklog := ReasonBacklogDeferred
	reasonReader := ReasonReaderFailed
	reasonInvalid := ReasonInvalidResponse
	reasonReset := ReasonCheckpointReset
	event := Event{ObservedAt: testTime.Add(-time.Second), Source: SourceSystem, Severity: SeverityError, EventCode: "SYSTEMD_PRIORITY_3"}
	discard := DiscardCount{At: testTime.Add(-2 * time.Second), Count: 1}

	valid := []Batch{
		{
			Kind: BatchNormal, Source: SourceSystem, ExpectedRevision: 2,
			QueryStartedAt: testTime, StartedAt: testTime, FinishedAt: testTime.Add(time.Second),
			SupportState: SupportSupported, CollectionState: CollectionOK,
			Events: []Event{event}, ExaminedCount: 1, CaughtUp: true, NextOpaque: []byte("cursor"),
		},
		{
			Kind: BatchNormal, Source: SourceSystem, QueryStartedAt: testTime, StartedAt: testTime, FinishedAt: testTime,
			SupportState: SupportSupported, CollectionState: CollectionPartial, ReasonCode: &reasonBacklog,
			Events: []Event{event}, Discards: []DiscardCount{discard},
			ExaminedCount: 3, DiscardedCount: 1, Deferred: true, NextOpaque: []byte("cursor"),
		},
		{
			Kind: BatchNormal, Source: SourceSystem, QueryStartedAt: testTime, StartedAt: testTime, FinishedAt: testTime,
			SupportState: SupportUnavailable, CollectionState: CollectionFailed, ReasonCode: &reasonReader,
		},
		{
			Kind: BatchResetPending, Source: SourceSystem, QueryStartedAt: testTime, StartedAt: testTime, FinishedAt: testTime,
			SupportState: SupportUnavailable, CollectionState: CollectionFailed, ReasonCode: &reasonReader, ExaminedCount: 1, ProbeCount: 1,
		},
		{
			Kind: BatchResetEstablished, Source: SourceSystem, QueryStartedAt: testTime, StartedAt: testTime, FinishedAt: testTime,
			SupportState: SupportSupported, CollectionState: CollectionPartial, ReasonCode: &reasonReset,
			ExaminedCount: 1, ProbeCount: 1, NextOpaque: []byte("new-cursor"),
		},
		{
			Kind: BatchNormal, Source: SourceSystem, QueryStartedAt: testTime, StartedAt: testTime, FinishedAt: testTime,
			SupportState: SupportSupported, CollectionState: CollectionPartial, ReasonCode: &reasonInvalid,
			Events: []Event{event}, Discards: []DiscardCount{discard},
			ExaminedCount: 2, DiscardedCount: 1, CaughtUp: true, NextOpaque: []byte("cursor"),
		},
	}
	for i, batch := range valid {
		if err := batch.Validate(); err != nil {
			t.Fatalf("valid batch %d rejected: %v", i, err)
		}
	}

	tests := []struct {
		name   string
		mutate func(*Batch)
	}{
		{"unknown kind", func(b *Batch) { b.Kind = "TAIL" }},
		{"source mismatch", func(b *Batch) { b.Events[0].Source = SourceApplication }},
		{"discard mismatch", func(b *Batch) { b.DiscardedCount = 2 }},
		{"examined mismatch", func(b *Batch) { b.ExaminedCount = 0 }},
		{"deferred caught up", func(b *Batch) { b.Deferred = true; b.ExaminedCount = 3 }},
		{"oversized cursor", func(b *Batch) { b.NextOpaque = make([]byte, MaxCheckpointBytes+1) }},
		{"missing reason", func(b *Batch) { b.CollectionState = CollectionPartial; b.ReasonCode = nil }},
		{"inverted time", func(b *Batch) { b.StartedAt = b.QueryStartedAt.Add(-time.Nanosecond) }},
	}
	for _, test := range tests {
		batch := valid[0].Clone()
		test.mutate(&batch)
		if err := batch.Validate(); err == nil {
			t.Fatalf("%s accepted", test.name)
		}
	}

	badReset := valid[4].Clone()
	badReset.NextOpaque = nil
	if err := badReset.Validate(); err == nil {
		t.Fatal("established reset without checkpoint accepted")
	}
	badReset = valid[3].Clone()
	badReset.Events = []Event{event}
	if err := badReset.Validate(); err == nil {
		t.Fatal("pending reset with event accepted")
	}
	badReset = valid[3].Clone()
	badReset.SupportState = SupportSupported
	badReset.CollectionState = CollectionOK
	badReset.ReasonCode = nil
	if err := badReset.Validate(); err == nil {
		t.Fatal("healthy pending reset accepted")
	}
	badReset = valid[4].Clone()
	badReset.SupportState = SupportUnavailable
	badReset.CollectionState = CollectionFailed
	if err := badReset.Validate(); err == nil {
		t.Fatal("failed established reset accepted")
	}
	badReset = valid[4].Clone()
	badReset.ProbeCount = 0
	if err := badReset.Validate(); err == nil {
		t.Fatal("established reset without a probe accepted")
	}
	badReset = valid[4].Clone()
	badReset.ProbeCount = 2
	badReset.ExaminedCount = 2
	if err := badReset.Validate(); err != nil {
		t.Fatalf("two-probe established reset rejected: %v", err)
	}
	badReset = valid[3].Clone()
	badReset.ProbeCount = 2
	badReset.ExaminedCount = 2
	if err := badReset.Validate(); err != nil {
		t.Fatalf("two-probe pending reset rejected: %v", err)
	}
	badReset.ExaminedCount = 1
	if err := badReset.Validate(); err == nil {
		t.Fatal("reset probe/examined mismatch accepted")
	}
	badNormal := valid[0].Clone()
	badNormal.NextOpaque = nil
	if err := badNormal.Validate(); err == nil {
		t.Fatal("observed normal batch without checkpoint accepted")
	}
	badNormal = valid[0].Clone()
	badNormal.CaughtUp = false
	if err := badNormal.Validate(); err == nil {
		t.Fatal("OK batch without caught-up proof accepted")
	}
	boundedDiscards := Batch{
		Kind: BatchNormal, Source: SourceSystem, QueryStartedAt: testTime, StartedAt: testTime, FinishedAt: testTime,
		SupportState: SupportSupported, CollectionState: CollectionPartial, ReasonCode: &reasonInvalid,
		Discards: []DiscardCount{{At: testTime, Count: MaxAcceptedEvents}}, ExaminedCount: MaxAcceptedEvents,
		DiscardedCount: MaxAcceptedEvents, CaughtUp: true, NextOpaque: []byte("cursor"),
	}
	if err := boundedDiscards.Validate(); err != nil {
		t.Fatalf("512 discarded rows rejected: %v", err)
	}
	boundedDiscards.Discards[0].Count++
	boundedDiscards.DiscardedCount++
	boundedDiscards.ExaminedCount++
	if err := boundedDiscards.Validate(); err == nil {
		t.Fatal("513 discarded rows accepted")
	}
	for name, mutate := range map[string]func(*Batch){
		"disabled failed": func(b *Batch) {
			b.SupportState = SupportDisabled
		},
		"permission failed": func(b *Batch) {
			b.SupportState = SupportPermissionDenied
		},
		"unsupported failed": func(b *Batch) {
			b.SupportState = SupportUnsupported
		},
		"supported not run": func(b *Batch) {
			b.SupportState = SupportSupported
			b.CollectionState = CollectionNotRun
		},
	} {
		stateBatch := valid[2].Clone()
		mutate(&stateBatch)
		if err := stateBatch.Validate(); err == nil {
			t.Fatalf("%s batch accepted", name)
		}
	}
	for _, disallowed := range []ReasonCode{ReasonMissedCollection, ReasonNotYetObserved, ReasonLogStorageUnavailable, ReasonLogSourcesDisabled, ReasonSourcePartial} {
		stateBatch := valid[2].Clone()
		stateBatch.ReasonCode = &disallowed
		if err := stateBatch.Validate(); err == nil {
			t.Fatalf("non-reader reason %q accepted in native batch", disallowed)
		}
	}
	for _, truncation := range []ReasonCode{ReasonDeadlineExceeded, ReasonResponseTooLarge, ReasonBacklogDeferred} {
		stateBatch := valid[5].Clone()
		stateBatch.ReasonCode = &truncation
		if err := stateBatch.Validate(); err == nil {
			t.Fatalf("truncated reason %q accepted with caught-up proof", truncation)
		}
	}
	stateBatch := valid[5].Clone()
	stateBatch.CollectionState = CollectionOK
	stateBatch.ReasonCode = nil
	if err := stateBatch.Validate(); err == nil {
		t.Fatal("OK batch with discarded row accepted")
	}
	tooManyDiscardGroups := valid[2].Clone()
	tooManyDiscardGroups.Discards = make([]DiscardCount, MaxAcceptedEvents+1)
	if err := tooManyDiscardGroups.Validate(); err == nil {
		t.Fatal("oversized discard group list accepted")
	}

	payload, err := json.Marshal(valid[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "cursor") || strings.Contains(string(payload), "NextOpaque") || strings.Contains(string(payload), "ProbeCount") {
		t.Fatalf("private next checkpoint serialized: %s", payload)
	}
}

func TestBatchProbeAccountingAndVisitBudget(t *testing.T) {
	event := Event{ObservedAt: testTime.Add(-time.Second), Source: SourceSystem, Severity: SeverityInfo, EventCode: "SYSTEMD_PRIORITY_6"}

	initialEmptyTailProof := Batch{
		Kind: BatchNormal, Source: SourceSystem,
		QueryStartedAt: testTime, StartedAt: testTime, FinishedAt: testTime,
		SupportState: SupportSupported, CollectionState: CollectionOK,
		ExaminedCount: 1, ProbeCount: 1, CaughtUp: true, NextOpaque: []byte("tail-cursor"),
	}
	if err := initialEmptyTailProof.Validate(); err != nil {
		t.Fatalf("empty-window tail proof rejected: %v", err)
	}
	missingProbeCursor := initialEmptyTailProof.Clone()
	missingProbeCursor.NextOpaque = nil
	if err := missingProbeCursor.Validate(); err == nil {
		t.Fatal("normal probe without a checkpoint cursor accepted")
	}

	continuation := initialEmptyTailProof.Clone()
	continuation.Events = make([]Event, 511)
	for i := range continuation.Events {
		continuation.Events[i] = event
	}
	continuation.ExaminedCount = 512
	if err := continuation.Validate(); err != nil {
		t.Fatalf("511 rows plus continuation probe rejected: %v", err)
	}
	independentEOF := continuation.Clone()
	independentEOF.Events = append(independentEOF.Events, event)
	independentEOF.ExaminedCount = MaxExaminedEvents
	if err := independentEOF.Validate(); err != nil {
		t.Fatalf("512 rows plus probe with independent EOF proof rejected: %v", err)
	}

	deferredReason := ReasonBacklogDeferred
	continuation.CollectionState = CollectionPartial
	continuation.ReasonCode = &deferredReason
	continuation.CaughtUp = false
	continuation.Deferred = true
	continuation.ExaminedCount = MaxExaminedEvents
	if err := continuation.Validate(); err != nil {
		t.Fatalf("511 rows plus probe and reserved sentinel rejected: %v", err)
	}

	overBudget := continuation.Clone()
	overBudget.Events = append(overBudget.Events, event)
	overBudget.ExaminedCount++
	if err := overBudget.Validate(); err == nil {
		t.Fatal("512 rows plus probe and sentinel accepted")
	}

	twoNormalProbes := initialEmptyTailProof.Clone()
	twoNormalProbes.ProbeCount = 2
	twoNormalProbes.ExaminedCount = 2
	if err := twoNormalProbes.Validate(); err == nil {
		t.Fatal("normal batch with two probes accepted")
	}
}

func TestSummaryValidationFixedGridCountsAndCoverage(t *testing.T) {
	unknown := validUnknownSummary(SourceSystem)
	if err := unknown.Validate(); err != nil {
		t.Fatalf("valid unknown summary rejected: %v", err)
	}
	application := validUnknownSummary(SourceApplication)
	if err := application.Validate(); err != nil {
		t.Fatalf("application-only summary rejected: %v", err)
	}

	full := validFullSummary()
	if err := full.Validate(); err != nil {
		t.Fatalf("valid full summary rejected: %v", err)
	}

	bad := full.Clone()
	bad.Sources[0].Buckets[0].Counts.Severity.Info = 1
	if err := bad.Validate(); err == nil {
		t.Fatal("severity/captured mismatch accepted")
	}
	bad = full.Clone()
	bad.Sources[0].Buckets[0].Counts.Captured = MaxSafeInteger + 1
	if err := bad.Validate(); err == nil {
		t.Fatal("unsafe integer accepted")
	}
	bad = full.Clone()
	bad.Sources[0].Buckets[0].Counts = nil
	if err := bad.Validate(); err == nil {
		t.Fatal("full bucket without known zero accepted")
	}
	bad = unknown.Clone()
	bad.Sources[0].Buckets[0].ReasonCode = nil
	if err := bad.Validate(); err == nil {
		t.Fatal("unknown bucket without reason accepted")
	}
	bad = unknown.Clone()
	bad.Sources[0].Buckets[0].CoverageState = CoverageGapState
	if err := bad.Validate(); err == nil {
		t.Fatal("gap bucket with not-yet-observed reason accepted")
	}
	gapReason := ReasonReaderFailed
	bad = unknown.Clone()
	bad.Sources[0].Buckets[0].CoverageState = CoverageGapState
	bad.Sources[0].Buckets[0].ReasonCode = &gapReason
	bad.Sources[0].Buckets[0].Counts = &BucketCounts{}
	bad.Sources[0].CoverageState = CoverageGapState
	bad.Sources[0].Counts = &Counts{}
	if err := bad.Validate(); err == nil {
		t.Fatal("gap bucket with invented zero counts accepted")
	}
	positiveGap := unknown.Clone()
	positiveGap.Sources[0].Buckets[0].CoverageState = CoverageGapState
	positiveGap.Sources[0].Buckets[0].ReasonCode = &gapReason
	positiveGap.Sources[0].Buckets[0].Counts = &BucketCounts{Counts: Counts{Captured: 1}, Severity: SeverityCounts{Error: 1}}
	positiveGap.Sources[0].CoverageState = CoverageGapState
	positiveGap.Sources[0].Counts = &Counts{Captured: 1}
	if err := positiveGap.Validate(); err != nil {
		t.Fatalf("positive known counts in gap rejected: %v", err)
	}
	partial := full.Clone()
	partial.Sources[0].Buckets[0].CoverageState = CoveragePartial
	partial.Sources[0].Buckets[0].CoveredSeconds = 30
	partial.Sources[0].Buckets[0].ReasonCode = ptrReason(ReasonNotYetObserved)
	partial.Sources[0].CoverageState = CoveragePartial
	partial.Sources[0].CoveredSeconds -= 30
	if err := partial.Validate(); err != nil {
		t.Fatalf("partial covered/unknown bucket rejected: %v", err)
	}
	bad = full.Clone()
	bad.Sources[0].CoveredSeconds--
	if err := bad.Validate(); err == nil {
		t.Fatal("wrong source coverage total accepted")
	}
	bad = full.Clone()
	bad.Sources[0].Buckets[1].At = bad.Sources[0].Buckets[1].At.Add(time.Second)
	if err := bad.Validate(); err == nil {
		t.Fatal("off-grid bucket accepted")
	}

	wrongOrder := validUnknownSummary(SourceApplication)
	wrongOrder.Sources = append(wrongOrder.Sources, validUnknownSummary(SourceSystem).Sources[0])
	if err := wrongOrder.Validate(); err == nil {
		t.Fatal("reversed source order accepted")
	}

	overflow := validFullSummary()
	for i := range overflow.Sources[0].Buckets {
		overflow.Sources[0].Buckets[i].Counts = &BucketCounts{}
	}
	overflow.Sources[0].Buckets[0].Counts.Captured = MaxSafeInteger
	overflow.Sources[0].Buckets[0].Counts.Severity.Error = MaxSafeInteger
	overflow.Sources[0].Counts = &Counts{Captured: MaxSafeInteger}
	applicationSource := overflow.Sources[0]
	applicationSource.Source = SourceApplication
	applicationSource.Status = applicationSource.Status.Clone()
	applicationSource.Buckets = make([]SummaryBucket, len(overflow.Sources[0].Buckets))
	for i, bucket := range overflow.Sources[0].Buckets {
		applicationSource.Buckets[i] = bucket
		applicationSource.Buckets[i].Counts = cloneBucketCounts(bucket.Counts)
	}
	applicationSource.Counts = cloneCounts(overflow.Sources[0].Counts)
	overflow.Sources = append(overflow.Sources, applicationSource)
	if err := overflow.Validate(); err == nil {
		t.Fatal("cross-source safe-integer overflow accepted")
	}
}

func TestSourceSummaryStatusReasonMatrix(t *testing.T) {
	reset := ReasonCheckpointReset
	storage := ReasonLogStorageUnavailable
	reader := ReasonReaderFailed

	valid := []Status{
		{
			SupportState: SupportSupported, CollectionState: CollectionPartial, Freshness: FreshnessUnknown,
			AttemptedAt: &testTime, ReasonCode: &reset,
		},
		{
			SupportState: SupportSupported, CollectionState: CollectionFailed, Freshness: FreshnessStale,
			ObservedAt: &testTime, AttemptedAt: &testTime, ReasonCode: &storage,
		},
		{
			SupportState: SupportUnavailable, CollectionState: CollectionFailed, Freshness: FreshnessStale,
			ObservedAt: &testTime, AttemptedAt: &testTime, ReasonCode: &storage,
		},
		{
			SupportState: SupportUnavailable, CollectionState: CollectionNotRun, Freshness: FreshnessUnknown,
			AttemptedAt: &testTime, ReasonCode: &reader,
		},
	}
	for i, status := range valid {
		summary := validFullSummary()
		summary.Sources[0].Status = status
		if err := summary.Validate(); err != nil {
			t.Fatalf("valid source status %d rejected: %v", i, err)
		}
	}

	for name, status := range map[string]Status{
		"aggregate-only partial reason": {
			SupportState: SupportSupported, CollectionState: CollectionPartial, Freshness: FreshnessUnknown,
			AttemptedAt: &testTime, ReasonCode: ptrReason(ReasonSourcePartial),
		},
		"disabled configured source": {
			SupportState: SupportDisabled, CollectionState: CollectionNotRun, Freshness: FreshnessUnknown,
			ReasonCode: ptrReason(ReasonLogSourcesDisabled),
		},
		"unavailable permission reason": {
			SupportState: SupportUnavailable, CollectionState: CollectionNotRun, Freshness: FreshnessUnknown,
			AttemptedAt: &testTime, ReasonCode: ptrReason(ReasonPermissionDenied),
		},
		"supported failed reset reason": {
			SupportState: SupportSupported, CollectionState: CollectionFailed, Freshness: FreshnessUnknown,
			AttemptedAt: &testTime, ReasonCode: &reset,
		},
	} {
		summary := validFullSummary()
		summary.Sources[0].Status = status
		if err := summary.Validate(); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}

func TestSummaryQueryRejectsNonContractGrid(t *testing.T) {
	valid := SummaryQuery{WindowStart: testTime.Add(-time.Hour), WindowEnd: testTime, BucketInterval: time.Minute, BucketCount: 60}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid query rejected: %v", err)
	}
	for name, mutate := range map[string]func(*SummaryQuery){
		"wrong count":    func(q *SummaryQuery) { q.BucketCount = 59 },
		"wrong interval": func(q *SummaryQuery) { q.BucketInterval = 2 * time.Minute },
		"wrong span":     func(q *SummaryQuery) { q.WindowStart = q.WindowStart.Add(time.Minute) },
		"not aligned": func(q *SummaryQuery) {
			q.WindowStart = q.WindowStart.Add(time.Second)
			q.WindowEnd = q.WindowEnd.Add(time.Second)
		},
		"nanoseconds": func(q *SummaryQuery) {
			q.WindowStart = q.WindowStart.Add(time.Nanosecond)
			q.WindowEnd = q.WindowEnd.Add(time.Nanosecond)
		},
		"non utc": func(q *SummaryQuery) { q.WindowEnd = q.WindowEnd.In(time.FixedZone("zero", 0)) },
	} {
		query := valid
		mutate(&query)
		if err := query.Validate(); err == nil {
			t.Fatalf("%s query accepted", name)
		}
	}
}

func TestTimeValidationRejectsNonRFC3339Year(t *testing.T) {
	event := Event{
		ObservedAt: time.Date(10_000, 1, 1, 0, 0, 0, 0, time.UTC),
		Source:     SourceSystem, Severity: SeverityInfo, EventCode: "WIN_1",
	}
	if err := event.Validate(); err == nil {
		t.Fatal("year outside RFC3339 range accepted")
	}
}

func TestSnapshotValidation(t *testing.T) {
	status := successfulStatus()
	events := []Event{
		{ObservedAt: testTime, Source: SourceSystem, Severity: SeverityInfo, EventCode: "WIN_1"},
		{ObservedAt: testTime.Add(-time.Second), Source: SourceApplication, Severity: SeverityWarn, EventCode: "WIN_2"},
	}
	snapshot := Snapshot{Status: status, TotalCount: len(events), Events: events}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("valid snapshot rejected: %v", err)
	}
	snapshot.Events[0], snapshot.Events[1] = snapshot.Events[1], snapshot.Events[0]
	if err := snapshot.Validate(); err == nil {
		t.Fatal("oldest-first snapshot accepted")
	}
	snapshot.TotalCount = MaxRecentEvents + 1
	if err := snapshot.Validate(); err == nil {
		t.Fatal("oversized snapshot accepted")
	}
}

func validUnknownSummary(source Source) Summary {
	start := testTime.Add(-time.Hour)
	reason := ReasonNotYetObserved
	statusReason := ReasonNoVisibleJournal
	buckets := make([]SummaryBucket, 60)
	for i := range buckets {
		buckets[i] = SummaryBucket{
			At: start.Add(time.Duration(i) * time.Minute), CoverageState: CoverageUnknown, ReasonCode: &reason,
		}
	}
	return Summary{
		WindowStart: start, WindowEnd: testTime, BucketInterval: time.Minute, BucketCount: 60,
		Sources: []SourceSummary{{
			Source:        source,
			Status:        Status{SupportState: SupportUnavailable, CollectionState: CollectionNotRun, Freshness: FreshnessUnknown, ReasonCode: &statusReason},
			CoverageState: CoverageUnknown, Buckets: buckets,
		}},
	}
}

func validFullSummary() Summary {
	start := testTime.Add(-time.Hour)
	zero := &BucketCounts{}
	buckets := make([]SummaryBucket, 60)
	for i := range buckets {
		buckets[i] = SummaryBucket{
			At: start.Add(time.Duration(i) * time.Minute), CoverageState: CoverageFull, CoveredSeconds: 60,
			Counts: cloneBucketCounts(zero),
		}
	}
	counts := &Counts{}
	return Summary{
		WindowStart: start, WindowEnd: testTime, BucketInterval: time.Minute, BucketCount: 60,
		Sources: []SourceSummary{{
			Source: SourceSystem, Status: successfulStatus(), CoverageState: CoverageFull,
			CoveredSeconds: 3600, Counts: counts, Buckets: buckets,
		}},
	}
}

func successfulStatus() Status {
	observed := testTime
	attempted := testTime
	coverage := testTime
	return Status{
		SupportState: SupportSupported, CollectionState: CollectionOK, Freshness: FreshnessCurrent,
		ObservedAt: &observed, AttemptedAt: &attempted, CoverageThrough: &coverage,
	}
}

func ptrReason(reason ReasonCode) *ReasonCode { return &reason }
