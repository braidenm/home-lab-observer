package logobs

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type collectorTicker struct {
	c       chan time.Time
	stopped atomic.Bool
}

func (t *collectorTicker) C() <-chan time.Time { return t.c }
func (t *collectorTicker) Stop()               { t.stopped.Store(true) }

type collectorClock struct{ at time.Time }

func (c collectorClock) Now() time.Time { return c.at }

type steppingCollectorClock struct {
	at    time.Time
	calls atomic.Int32
}

func (c *steppingCollectorClock) Now() time.Time {
	call := c.calls.Add(1) - 1
	return c.at.Add(time.Duration(call) * time.Minute)
}

type collectorStore struct {
	load   func(context.Context, Source) (Checkpoint, error)
	commit func(context.Context, Batch) error
	query  func(context.Context, []Source, SummaryQuery) (Summary, error)
}

func (s *collectorStore) LoadCheckpoint(ctx context.Context, source Source) (Checkpoint, error) {
	if s.load == nil {
		return Checkpoint{}, nil
	}
	return s.load(ctx, source)
}
func (s *collectorStore) CommitBatch(ctx context.Context, batch Batch) error {
	if s.commit == nil {
		return nil
	}
	return s.commit(ctx, batch)
}
func (s *collectorStore) QuerySummary(ctx context.Context, sources []Source, query SummaryQuery) (Summary, error) {
	if s.query == nil {
		return Summary{}, errors.New("unexpected summary query")
	}
	return s.query(ctx, sources, query)
}

type collectorReader func(context.Context, ReadRequest) (Batch, error)

func (r collectorReader) Read(ctx context.Context, request ReadRequest) (Batch, error) {
	return r(ctx, request)
}

func TestCollectorConfigAndDisabledMode(t *testing.T) {
	store := &collectorStore{}
	for name, config := range map[string]Config{
		"missing store":    {},
		"missing reader":   {Sources: []Source{SourceSystem}, Store: store},
		"duplicate":        {Sources: []Source{SourceSystem, SourceSystem}, Reader: collectorReader(nil), Store: store},
		"reverse order":    {Sources: []Source{SourceApplication, SourceSystem}, Reader: collectorReader(nil), Store: store},
		"unknown source":   {Sources: []Source{"security"}, Reader: collectorReader(nil), Store: store},
		"too many sources": {Sources: []Source{SourceSystem, SourceApplication, SourceSystem}, Reader: collectorReader(nil), Store: store},
	} {
		if _, err := New(config); err == nil {
			t.Fatalf("%s config accepted", name)
		}
	}

	var storeCalls atomic.Int32
	store.load = func(context.Context, Source) (Checkpoint, error) { storeCalls.Add(1); return Checkpoint{}, nil }
	store.commit = func(context.Context, Batch) error { storeCalls.Add(1); return nil }
	store.query = func(context.Context, []Source, SummaryQuery) (Summary, error) {
		storeCalls.Add(1)
		return Summary{}, nil
	}
	ticker := &collectorTicker{c: make(chan time.Time, 1)}
	intervals := make(chan time.Duration, 1)
	collector, err := New(Config{Store: store, NewTicker: func(interval time.Duration) Ticker {
		intervals <- interval
		return ticker
	}})
	if err != nil {
		t.Fatal(err)
	}
	preStartCtx, preStartCancel := context.WithTimeout(context.Background(), time.Second)
	defer preStartCancel()
	if err := collector.Stop(preStartCtx); err != nil {
		t.Fatalf("Stop before Start: %v", err)
	}
	if err := collector.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := collector.Start(context.Background()); err == nil {
		t.Fatal("second Start succeeded")
	}
	ticker.c <- testTime
	time.Sleep(10 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := collector.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := collector.Stop(ctx); err != nil {
		t.Fatalf("repeated Stop: %v", err)
	}
	if storeCalls.Load() != 0 || !ticker.stopped.Load() {
		t.Fatalf("disabled collector made %d store calls, ticker stopped=%v", storeCalls.Load(), ticker.stopped.Load())
	}
	if interval := <-intervals; interval != time.Minute {
		t.Fatalf("ticker interval = %v", interval)
	}
	current := collector.Current()
	if current.Status.SupportState != SupportDisabled || current.Status.ReasonCode == nil || *current.Status.ReasonCode != ReasonLogSourcesDisabled {
		t.Fatalf("disabled current = %+v", current)
	}
	query := SummaryQuery{WindowStart: testTime.Add(-time.Hour), WindowEnd: testTime, BucketInterval: time.Minute, BucketCount: 60}
	summary, err := collector.Summary(context.Background(), query)
	if err != nil || len(summary.Sources) != 0 || summary.BucketCount != query.BucketCount {
		t.Fatalf("disabled summary = %+v, err=%v", summary, err)
	}
	if storeCalls.Load() != 0 {
		t.Fatalf("disabled summary made %d store calls", storeCalls.Load())
	}
}

func TestCollectorImmediateCadenceIsSingleFlight(t *testing.T) {
	ticker := &collectorTicker{c: make(chan time.Time, 1)}
	entered := make(chan struct{}, 2)
	release := make(chan struct{}, 2)
	var active, peak, reads atomic.Int32
	reader := collectorReader(func(_ context.Context, request ReadRequest) (Batch, error) {
		reads.Add(1)
		current := active.Add(1)
		for {
			observed := peak.Load()
			if current <= observed || peak.CompareAndSwap(observed, current) {
				break
			}
		}
		entered <- struct{}{}
		<-release
		active.Add(-1)
		return successfulBatch(request, nil), nil
	})
	collector, err := New(Config{
		Sources: []Source{SourceSystem}, Reader: reader, Store: &collectorStore{}, Clock: collectorClock{testTime},
		NewTicker: func(time.Duration) Ticker { return ticker },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := collector.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-entered
	ticker.c <- testTime.Add(time.Minute)
	release <- struct{}{}
	<-entered
	if peak.Load() != 1 {
		t.Fatalf("peak reader concurrency = %d", peak.Load())
	}
	release <- struct{}{}
	waitCollector(t, func() bool { return reads.Load() == 2 })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := collector.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCollectorSharesOneFourSecondLaneAcrossSources(t *testing.T) {
	var mu sync.Mutex
	deadlines := make([]time.Time, 0, 2)
	reader := collectorReader(func(ctx context.Context, request ReadRequest) (Batch, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			return Batch{}, errors.New("missing collector deadline")
		}
		mu.Lock()
		deadlines = append(deadlines, deadline)
		mu.Unlock()
		reason := ReasonPlatformUnsupported
		return Batch{
			Kind: BatchNormal, Source: request.Source, ExpectedRevision: request.Checkpoint.Revision,
			QueryStartedAt: request.QueryStartedAt, StartedAt: request.QueryStartedAt, FinishedAt: request.QueryStartedAt,
			SupportState: SupportUnsupported, CollectionState: CollectionNotRun, ReasonCode: &reason,
		}, nil
	})
	started := time.Now()
	collector, err := New(Config{
		Sources: []Source{SourceSystem, SourceApplication}, Reader: reader, Store: &collectorStore{}, Clock: collectorClock{testTime},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = collector.Start(context.Background())
	waitCollector(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(deadlines) == 2
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = collector.Stop(ctx)
	mu.Lock()
	defer mu.Unlock()
	if !deadlines[0].Equal(deadlines[1]) {
		t.Fatalf("sources received different lane deadlines: %v and %v", deadlines[0], deadlines[1])
	}
	duration := deadlines[0].Sub(started)
	if duration < 3*time.Second || duration > 5*time.Second {
		t.Fatalf("collector lane deadline = %v", duration)
	}
}

func TestCollectorLoadsCheckpointsBeforeStartingNativeLane(t *testing.T) {
	var loads atomic.Int32
	loadFinished := make(chan time.Time, 1)
	store := &collectorStore{load: func(context.Context, Source) (Checkpoint, error) {
		if loads.Add(1) == 2 {
			time.Sleep(600 * time.Millisecond)
			loadFinished <- time.Now()
		}
		return Checkpoint{}, nil
	}}
	reader := collectorReader(func(ctx context.Context, request ReadRequest) (Batch, error) {
		if request.Source == SourceSystem {
			finished := <-loadFinished
			deadline, ok := ctx.Deadline()
			if !ok || deadline.Sub(finished) < 3700*time.Millisecond {
				return Batch{}, errors.New("native lane deadline was consumed by checkpoint loading")
			}
		}
		reason := ReasonPlatformUnsupported
		return Batch{
			Kind: BatchNormal, Source: request.Source, ExpectedRevision: request.Checkpoint.Revision,
			QueryStartedAt: request.QueryStartedAt, StartedAt: request.QueryStartedAt, FinishedAt: request.QueryStartedAt,
			SupportState: SupportUnsupported, CollectionState: CollectionNotRun, ReasonCode: &reason,
		}, nil
	})
	collector, err := New(Config{
		Sources: []Source{SourceSystem, SourceApplication}, Reader: reader, Store: store, Clock: collectorClock{testTime},
	})
	if err != nil {
		t.Fatal(err)
	}
	collector.collectOnce(context.Background())
	current := collector.Current()
	if current.Status.ReasonCode == nil || *current.Status.ReasonCode != ReasonPlatformUnsupported {
		t.Fatalf("preloaded native lane failed: %+v", current.Status)
	}
}

func TestCollectorBoundsEveryStorePhase(t *testing.T) {
	deadlineErrors := make(chan error, 3)
	assertBounded := func(ctx context.Context) {
		deadline, ok := ctx.Deadline()
		if !ok {
			deadlineErrors <- errors.New("store call has no deadline")
			return
		}
		remaining := time.Until(deadline)
		if remaining <= 0 || remaining > storageOperationTimeout+time.Second {
			deadlineErrors <- errors.New("store deadline is outside the fixed bound")
		}
	}
	var loads, commits, queries atomic.Int32
	base := validFullSummary()
	store := &collectorStore{
		load: func(ctx context.Context, _ Source) (Checkpoint, error) {
			assertBounded(ctx)
			loads.Add(1)
			return Checkpoint{}, nil
		},
		commit: func(ctx context.Context, _ Batch) error {
			assertBounded(ctx)
			commits.Add(1)
			return nil
		},
		query: func(ctx context.Context, _ []Source, _ SummaryQuery) (Summary, error) {
			assertBounded(ctx)
			queries.Add(1)
			return base.Clone(), nil
		},
	}
	reader := collectorReader(func(_ context.Context, request ReadRequest) (Batch, error) { return successfulBatch(request, nil), nil })
	collector, _ := New(Config{Sources: []Source{SourceSystem}, Reader: reader, Store: store, Clock: collectorClock{testTime}})
	_ = collector.Start(context.Background())
	waitCollector(t, func() bool { return commits.Load() == 1 })
	query := SummaryQuery{WindowStart: testTime.Add(-time.Hour), WindowEnd: testTime, BucketInterval: time.Minute, BucketCount: 60}
	if _, err := collector.Summary(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = collector.Stop(ctx)
	if loads.Load() != 1 || commits.Load() != 1 || queries.Load() != 1 {
		t.Fatalf("store calls loads=%d commits=%d queries=%d", loads.Load(), commits.Load(), queries.Load())
	}
	select {
	case err := <-deadlineErrors:
		t.Fatal(err)
	default:
	}
}

func TestCollectorStopCancelsAndJoinsWithoutCommitOrClose(t *testing.T) {
	entered := make(chan struct{})
	exited := make(chan struct{})
	var commits atomic.Int32
	store := &collectorStore{commit: func(context.Context, Batch) error { commits.Add(1); return nil }}
	reader := collectorReader(func(ctx context.Context, _ ReadRequest) (Batch, error) {
		close(entered)
		<-ctx.Done()
		close(exited)
		return Batch{}, ctx.Err()
	})
	collector, err := New(Config{Sources: []Source{SourceSystem}, Reader: reader, Store: store, Clock: collectorClock{testTime}})
	if err != nil {
		t.Fatal(err)
	}
	_ = collector.Start(context.Background())
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := collector.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	default:
		t.Fatal("Stop returned before reader exited")
	}
	if commits.Load() != 0 {
		t.Fatalf("shutdown cancellation committed %d batches", commits.Load())
	}
}

func TestCollectorReaderErrorCommitsOnlyFixedFailure(t *testing.T) {
	previous := testTime.Add(-time.Minute)
	checkpoint := Checkpoint{Revision: 4, ResetPending: true, PreviousAttemptAt: &previous}
	committed := make(chan Batch, 1)
	store := &collectorStore{
		load: func(context.Context, Source) (Checkpoint, error) { return checkpoint.Clone(), nil },
		commit: func(_ context.Context, batch Batch) error {
			committed <- batch.Clone()
			return nil
		},
	}
	reader := collectorReader(func(context.Context, ReadRequest) (Batch, error) {
		return Batch{}, errors.New("synthetic private native failure")
	})
	collector, err := New(Config{Sources: []Source{SourceSystem}, Reader: reader, Store: store, Clock: collectorClock{testTime}})
	if err != nil {
		t.Fatal(err)
	}
	_ = collector.Start(context.Background())
	var batch Batch
	select {
	case batch = <-committed:
	case <-time.After(time.Second):
		t.Fatal("fixed reader failure was not committed")
	}
	if batch.Kind != BatchResetPending || batch.ExpectedRevision != 4 || batch.ReasonCode == nil || *batch.ReasonCode != ReasonReaderFailed ||
		len(batch.Events) != 0 || len(batch.Discards) != 0 || len(batch.NextOpaque) != 0 || batch.ExaminedCount != 0 {
		t.Fatalf("committed reader failure = %+v", batch)
	}
	if err := batch.Validate(); err != nil {
		t.Fatalf("fixed reader failure is invalid: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = collector.Stop(ctx)
}

func TestCollectorRejectsSuccessReturnedAfterReaderDeadline(t *testing.T) {
	reader := collectorReader(func(_ context.Context, request ReadRequest) (Batch, error) {
		return successfulBatch(request, nil), nil
	})
	collector, err := New(Config{Sources: []Source{SourceSystem}, Reader: reader, Store: &collectorStore{}, Clock: collectorClock{testTime}})
	if err != nil {
		t.Fatal(err)
	}
	expired, cancel := context.WithCancel(context.Background())
	cancel()
	batch, ok := collector.readSource(context.Background(), expired, SourceSystem, Checkpoint{})
	if !ok {
		t.Fatal("expired reader attempt was not reduced to a fixed batch")
	}
	if batch.CollectionState != CollectionFailed || batch.ReasonCode == nil || *batch.ReasonCode != ReasonReaderFailed || batch.CaughtUp {
		t.Fatalf("late success escaped as trustworthy: %+v", batch)
	}
}

func TestBatchMustMatchRequestResetState(t *testing.T) {
	previous := testTime.Add(-time.Minute)
	initial := ReadRequest{Source: SourceSystem, QueryStartedAt: testTime}
	resetReason := ReasonCheckpointReset
	reset := Batch{
		Kind: BatchResetEstablished, Source: SourceSystem, QueryStartedAt: testTime, StartedAt: testTime, FinishedAt: testTime,
		SupportState: SupportSupported, CollectionState: CollectionPartial, ReasonCode: &resetReason,
		ExaminedCount: 1, ProbeCount: 1, NextOpaque: []byte("tail"),
	}
	if err := reset.Validate(); err != nil {
		t.Fatal(err)
	}
	if validBatchForRequest(reset, initial) {
		t.Fatal("cursorless non-pending request accepted a reset transition")
	}

	pending := ReadRequest{
		Source: SourceSystem, QueryStartedAt: testTime,
		Checkpoint: Checkpoint{Revision: 1, ResetPending: true, PreviousAttemptAt: &previous},
	}
	normal := successfulBatch(pending, nil)
	if validBatchForRequest(normal, pending) {
		t.Fatal("reset-pending request accepted a normal batch")
	}

	stale := pending
	stale.Checkpoint.ResetPending = false
	stale.Checkpoint.Opaque = []byte("stale")
	reset.ExpectedRevision = 1
	if !validBatchForRequest(reset, stale) {
		t.Fatal("ordinary cursor request rejected a same-attempt reset transition")
	}
}

func TestCollectorAmbiguousCommitUsesSameBatchWithoutSecondRead(t *testing.T) {
	for _, appliedBeforeError := range []bool{false, true} {
		t.Run(map[bool]string{false: "retry", true: "already applied"}[appliedBeforeError], func(t *testing.T) {
			var mu sync.Mutex
			checkpoint := Checkpoint{}
			var commits []Batch
			var reads int
			store := &collectorStore{}
			store.load = func(context.Context, Source) (Checkpoint, error) {
				mu.Lock()
				defer mu.Unlock()
				return checkpoint.Clone(), nil
			}
			store.commit = func(_ context.Context, batch Batch) error {
				mu.Lock()
				defer mu.Unlock()
				commits = append(commits, batch.Clone())
				if len(commits) == 1 {
					if appliedBeforeError {
						checkpoint = Checkpoint{Revision: 1, PreviousAttemptAt: cloneTime(&batch.QueryStartedAt), Opaque: cloneBytes(batch.NextOpaque)}
					}
					batch.NextOpaque[0] = 'X'
					return errors.New("synthetic ambiguous write")
				}
				checkpoint = Checkpoint{Revision: 1, PreviousAttemptAt: cloneTime(&batch.QueryStartedAt), Opaque: cloneBytes(batch.NextOpaque)}
				return nil
			}
			reader := collectorReader(func(_ context.Context, request ReadRequest) (Batch, error) {
				reads++
				event := Event{ObservedAt: request.QueryStartedAt, Source: request.Source, Severity: SeverityInfo, EventCode: "SYSTEMD_PRIORITY_6"}
				return successfulBatch(request, []Event{event}), nil
			})
			collector, err := New(Config{Sources: []Source{SourceSystem}, Reader: reader, Store: store, Clock: collectorClock{testTime}})
			if err != nil {
				t.Fatal(err)
			}
			_ = collector.Start(context.Background())
			waitCollector(t, func() bool { return collector.Current().TotalCount == 1 })
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = collector.Stop(ctx)
			mu.Lock()
			defer mu.Unlock()
			wantCommits := 2
			if appliedBeforeError {
				wantCommits = 1
			}
			if reads != 1 || len(commits) != wantCommits {
				t.Fatalf("reads=%d commits=%d", reads, len(commits))
			}
			if len(commits) == 2 && !reflect.DeepEqual(commits[0], commits[1]) {
				t.Fatal("ambiguous retry changed the immutable batch")
			}
		})
	}
}

func TestCollectorDefiniteRevisionConflictNeverConfirmsOrCachesRejectedBatch(t *testing.T) {
	for _, ambiguousFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "initial conflict", true: "retry conflict"}[ambiguousFirst], func(t *testing.T) {
			var loads, commits atomic.Int32
			store := &collectorStore{
				load: func(context.Context, Source) (Checkpoint, error) {
					loads.Add(1)
					return Checkpoint{}, nil
				},
				commit: func(context.Context, Batch) error {
					attempt := commits.Add(1)
					if ambiguousFirst && attempt == 1 {
						return errors.New("synthetic ambiguous write")
					}
					return ErrRevisionConflict
				},
			}
			reader := collectorReader(func(_ context.Context, request ReadRequest) (Batch, error) {
				event := Event{ObservedAt: request.QueryStartedAt, Source: request.Source, Severity: SeverityInfo, EventCode: "SYSTEMD_PRIORITY_6"}
				return successfulBatch(request, []Event{event}), nil
			})
			collector, err := New(Config{Sources: []Source{SourceSystem}, Reader: reader, Store: store, Clock: collectorClock{testTime}})
			if err != nil {
				t.Fatal(err)
			}
			collector.collectOnce(context.Background())
			current := collector.Current()
			wantLoads, wantCommits := int32(1), int32(1)
			if ambiguousFirst {
				wantLoads, wantCommits = 2, 2
			}
			if loads.Load() != wantLoads || commits.Load() != wantCommits {
				t.Fatalf("definite conflict triggered further ambiguity resolution: loads=%d commits=%d", loads.Load(), commits.Load())
			}
			if current.TotalCount != 0 || len(current.Events) != 0 || current.Status.ReasonCode == nil || *current.Status.ReasonCode != ReasonLogStorageUnavailable {
				t.Fatalf("definitely rejected batch entered cache: %+v", current)
			}
		})
	}
}

func TestCollectorBoundsAndDeepClonesCurrentRing(t *testing.T) {
	events := make([]Event, 205)
	for i := range events {
		events[i] = Event{
			ObservedAt: testTime.Add(-time.Duration(i) * time.Second), Source: SourceSystem,
			Severity: SeverityInfo, EventCode: "SYSTEMD_PRIORITY_6",
		}
	}
	reader := collectorReader(func(_ context.Context, request ReadRequest) (Batch, error) {
		batch := successfulBatch(request, events)
		return batch, nil
	})
	collector, _ := New(Config{Sources: []Source{SourceSystem}, Reader: reader, Store: &collectorStore{}, Clock: collectorClock{testTime}})
	_ = collector.Start(context.Background())
	waitCollector(t, func() bool { return collector.Current().TotalCount == MaxRecentEvents })
	first := collector.Current()
	if len(first.Events) != MaxRecentEvents || first.Events[0].ObservedAt.Before(first.Events[len(first.Events)-1].ObservedAt) {
		t.Fatalf("bounded ring is not newest first: %d", len(first.Events))
	}
	collector.mu.RLock()
	retainedCapacity := cap(collector.events)
	collector.mu.RUnlock()
	if retainedCapacity != MaxRecentEvents {
		t.Fatalf("bounded ring retained oversized backing storage: cap=%d", retainedCapacity)
	}
	first.Events[0].EventCode = "WIN_1"
	*first.Status.ObservedAt = time.Time{}
	second := collector.Current()
	if second.Events[0].EventCode == "WIN_1" || second.Status.ObservedAt == nil || second.Status.ObservedAt.IsZero() {
		t.Fatal("Current returned mutable cache state")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = collector.Stop(ctx)
}

func TestCollectorStorageFailureKeepsRingAndOverlaysSummary(t *testing.T) {
	ticker := &collectorTicker{c: make(chan time.Time, 1)}
	var mu sync.Mutex
	checkpoint := Checkpoint{}
	commitNumber := 0
	store := &collectorStore{}
	store.load = func(context.Context, Source) (Checkpoint, error) {
		mu.Lock()
		defer mu.Unlock()
		return checkpoint.Clone(), nil
	}
	store.commit = func(_ context.Context, batch Batch) error {
		mu.Lock()
		defer mu.Unlock()
		commitNumber++
		if commitNumber == 1 {
			checkpoint = Checkpoint{Revision: 1, PreviousAttemptAt: cloneTime(&batch.QueryStartedAt), CoverageThrough: cloneTime(&batch.QueryStartedAt), Opaque: cloneBytes(batch.NextOpaque)}
			return nil
		}
		return errors.New("synthetic storage failure")
	}
	baseSummary := validFullSummary()
	store.query = func(_ context.Context, sources []Source, query SummaryQuery) (Summary, error) {
		if !reflect.DeepEqual(sources, []Source{SourceSystem}) {
			return Summary{}, errors.New("wrong source authority")
		}
		result := baseSummary.Clone()
		result.WindowStart, result.WindowEnd = query.WindowStart, query.WindowEnd
		result.BucketInterval, result.BucketCount = query.BucketInterval, query.BucketCount
		for i := range result.Sources[0].Buckets {
			result.Sources[0].Buckets[i].At = query.WindowStart.Add(time.Duration(i) * query.BucketInterval)
		}
		return result, nil
	}
	readNumber := 0
	reader := collectorReader(func(_ context.Context, request ReadRequest) (Batch, error) {
		readNumber++
		if readNumber == 1 {
			event := Event{ObservedAt: request.QueryStartedAt, Source: request.Source, Severity: SeverityError, EventCode: "SYSTEMD_PRIORITY_3"}
			return successfulBatch(request, []Event{event}), nil
		}
		reason := ReasonPlatformUnsupported
		return Batch{
			Kind: BatchNormal, Source: request.Source, ExpectedRevision: request.Checkpoint.Revision,
			QueryStartedAt: request.QueryStartedAt, StartedAt: request.QueryStartedAt, FinishedAt: request.QueryStartedAt,
			SupportState: SupportUnsupported, CollectionState: CollectionNotRun, ReasonCode: &reason,
		}, nil
	})
	collector, _ := New(Config{
		Sources: []Source{SourceSystem}, Reader: reader, Store: store, Clock: &steppingCollectorClock{at: testTime},
		NewTicker: func(time.Duration) Ticker { return ticker },
	})
	_ = collector.Start(context.Background())
	waitCollector(t, func() bool { return collector.Current().TotalCount == 1 })
	ticker.c <- testTime.Add(time.Minute)
	waitCollector(t, func() bool {
		current := collector.Current()
		return current.Status.ReasonCode != nil && *current.Status.ReasonCode == ReasonLogStorageUnavailable
	})
	current := collector.Current()
	if current.TotalCount != 1 || current.Status.SupportState != SupportUnavailable || current.Status.CollectionState != CollectionFailed || current.Status.Freshness != FreshnessStale {
		t.Fatalf("current after failed unsupported commit = %+v", current)
	}
	query := SummaryQuery{WindowStart: testTime.Add(-time.Hour), WindowEnd: testTime, BucketInterval: time.Minute, BucketCount: 60}
	summary, err := collector.Summary(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	status := summary.Sources[0].Status
	if status.SupportState != SupportUnavailable || status.CollectionState != CollectionFailed || status.ReasonCode == nil || *status.ReasonCode != ReasonLogStorageUnavailable || status.ObservedAt == nil {
		t.Fatalf("summary overlay = %+v", status)
	}
	summary.Sources[0].Buckets[0].At = time.Time{}
	again, err := collector.Summary(context.Background(), query)
	if err != nil || again.Sources[0].Buckets[0].At.IsZero() {
		t.Fatal("Summary returned mutable store/cache state")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = collector.Stop(ctx)
}

func TestStorageOverlayRetainsDurableCoverageAfterSessionPartial(t *testing.T) {
	observed := testTime
	coverage := testTime.Add(-time.Minute)
	storedReason := ReasonReaderFailed
	stored := Status{
		SupportState: SupportUnavailable, CollectionState: CollectionFailed, Freshness: FreshnessStale,
		ObservedAt: &observed, AttemptedAt: &observed, CoverageThrough: &coverage, ReasonCode: &storedReason,
	}
	sessionObserved := testTime.Add(time.Minute)
	failureAttempt := sessionObserved.Add(time.Minute)
	overlayReason := ReasonLogStorageUnavailable
	overlay := Status{
		SupportState: SupportSupported, CollectionState: CollectionFailed, Freshness: FreshnessStale,
		ObservedAt: &sessionObserved, AttemptedAt: &failureAttempt, ReasonCode: &overlayReason,
	}
	merged := mergeStorageOverlay(stored, overlay)
	if merged.CoverageThrough == nil || !merged.CoverageThrough.Equal(coverage) {
		t.Fatalf("durable coverage was lost: %+v", merged)
	}
	if err := merged.Validate(); err != nil {
		t.Fatalf("merged overlay is invalid: %v", err)
	}
}

func TestSummaryIgnoresOlderOverlayAndRequiresRequestedGrid(t *testing.T) {
	base := validFullSummary()
	newerAttempt := testTime.Add(time.Minute)
	base.Sources[0].Status.ObservedAt = &newerAttempt
	base.Sources[0].Status.AttemptedAt = &newerAttempt
	base.Sources[0].Status.CoverageThrough = &newerAttempt
	store := &collectorStore{query: func(context.Context, []Source, SummaryQuery) (Summary, error) { return base.Clone(), nil }}
	unusedReader := collectorReader(func(context.Context, ReadRequest) (Batch, error) {
		return Batch{}, errors.New("unexpected reader call")
	})
	collector, _ := New(Config{Sources: []Source{SourceSystem}, Reader: unusedReader, Store: store})
	oldReason := ReasonLogStorageUnavailable
	collector.overlays[SourceSystem] = Status{
		SupportState: SupportUnavailable, CollectionState: CollectionFailed, Freshness: FreshnessUnknown,
		AttemptedAt: &testTime, ReasonCode: &oldReason,
	}
	query := SummaryQuery{WindowStart: testTime.Add(-time.Hour), WindowEnd: testTime, BucketInterval: time.Minute, BucketCount: 60}
	summary, err := collector.Summary(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Sources[0].Status.CollectionState != CollectionOK || !summary.Sources[0].Status.AttemptedAt.Equal(newerAttempt) {
		t.Fatalf("older overlay replaced newer durable status: %+v", summary.Sources[0].Status)
	}

	wrongGrid := SummaryQuery{WindowStart: testTime.Add(-6 * time.Hour), WindowEnd: testTime, BucketInterval: 5 * time.Minute, BucketCount: 72}
	if _, err := collector.Summary(context.Background(), wrongGrid); err == nil {
		t.Fatal("valid but unrequested store grid accepted")
	}
}

func TestStatusAfterBatchRetainsUsefulQuality(t *testing.T) {
	previousAt := testTime.Add(-time.Minute)
	previous := Status{
		SupportState: SupportSupported, CollectionState: CollectionOK, Freshness: FreshnessCurrent,
		ObservedAt: &previousAt, AttemptedAt: &previousAt, CoverageThrough: &previousAt,
	}
	reason := ReasonBacklogDeferred
	event := Event{ObservedAt: testTime, Source: SourceSystem, Severity: SeverityWarn, EventCode: "SYSTEMD_PRIORITY_4"}
	partial := Batch{
		Kind: BatchNormal, Source: SourceSystem, QueryStartedAt: testTime, StartedAt: testTime, FinishedAt: testTime,
		SupportState: SupportSupported, CollectionState: CollectionPartial, ReasonCode: &reason,
		Events: []Event{event}, ExaminedCount: 2, Deferred: true, NextOpaque: []byte("cursor"),
	}
	status := StatusAfterBatch(partial, previous)
	if status.Freshness != FreshnessCurrent || status.ObservedAt == nil || !status.ObservedAt.Equal(testTime) || status.CoverageThrough == nil || !status.CoverageThrough.Equal(previousAt) {
		t.Fatalf("partial status = %+v", status)
	}
	failureReason := ReasonReaderFailed
	failureAt := testTime.Add(time.Minute)
	failure := Batch{
		Kind: BatchNormal, Source: SourceSystem, QueryStartedAt: failureAt, StartedAt: failureAt, FinishedAt: failureAt,
		SupportState: SupportUnavailable, CollectionState: CollectionFailed, ReasonCode: &failureReason,
	}
	failed := StatusAfterBatch(failure, status)
	if failed.Freshness != FreshnessStale || failed.ObservedAt == nil || !failed.ObservedAt.Equal(testTime) || failed.CoverageThrough == nil || !failed.CoverageThrough.Equal(previousAt) {
		t.Fatalf("failed status = %+v", failed)
	}
}

func successfulBatch(request ReadRequest, events []Event) Batch {
	return Batch{
		Kind: BatchNormal, Source: request.Source, ExpectedRevision: request.Checkpoint.Revision,
		QueryStartedAt: request.QueryStartedAt, StartedAt: request.QueryStartedAt, FinishedAt: request.QueryStartedAt,
		SupportState: SupportSupported, CollectionState: CollectionOK,
		Events: events, ExaminedCount: uint32(len(events)), CaughtUp: true, NextOpaque: []byte("synthetic-cursor"),
	}
}

func waitCollector(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("collector condition was not reached")
}
