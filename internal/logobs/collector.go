package logobs

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

const (
	collectionInterval      = time.Minute
	collectionTimeout       = 4 * time.Second
	storageOperationTimeout = 2 * time.Second
)

type Clock interface {
	Now() time.Time
}

type Ticker interface {
	C() <-chan time.Time
	Stop()
}

type Config struct {
	Sources   []Source
	Reader    Reader
	Store     Store
	Clock     Clock
	NewTicker func(time.Duration) Ticker
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type realTicker struct{ *time.Ticker }

func (t realTicker) C() <-chan time.Time { return t.Ticker.C }

type Collector struct {
	sources   []Source
	reader    Reader
	store     Store
	clock     Clock
	newTicker func(time.Duration) Ticker

	lifecycleMu sync.Mutex
	started     bool
	cancel      context.CancelFunc
	done        chan struct{}

	mu       sync.RWMutex
	latest   map[Source]Status
	overlays map[Source]Status
	events   []Event
}

func New(config Config) (*Collector, error) {
	if config.Store == nil {
		return nil, errors.New("log store is required")
	}
	if len(config.Sources) > MaxSources {
		return nil, errors.New("too many log sources")
	}
	previousRank := -1
	for _, source := range config.Sources {
		rank := sourceRank(source)
		if rank < 0 || rank <= previousRank {
			return nil, errors.New("log sources must be unique and ordered")
		}
		previousRank = rank
	}
	if len(config.Sources) > 0 && config.Reader == nil {
		return nil, errors.New("log reader is required for enabled sources")
	}
	if config.Clock == nil {
		config.Clock = realClock{}
	}
	if config.NewTicker == nil {
		config.NewTicker = func(interval time.Duration) Ticker { return realTicker{time.NewTicker(interval)} }
	}
	return &Collector{
		sources: append([]Source(nil), config.Sources...), reader: config.Reader, store: config.Store,
		clock: config.Clock, newTicker: config.NewTicker, done: make(chan struct{}),
		latest: make(map[Source]Status), overlays: make(map[Source]Status),
	}, nil
}

func (c *Collector) Start(parent context.Context) error {
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()
	if c.started {
		return errors.New("log collector already started")
	}
	ctx, cancel := context.WithCancel(parent)
	c.cancel = cancel
	c.started = true
	go c.loop(ctx)
	return nil
}

func (c *Collector) loop(ctx context.Context) {
	defer close(c.done)
	ticker := c.newTicker(collectionInterval)
	defer ticker.Stop()
	c.collectOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			c.collectOnce(ctx)
		}
	}
}

func (c *Collector) collectOnce(parent context.Context) {
	if len(c.sources) == 0 {
		return
	}
	type preparedSource struct {
		source     Source
		checkpoint Checkpoint
		ready      bool
	}
	prepared := make([]preparedSource, 0, len(c.sources))
	for _, source := range c.sources {
		if parent.Err() != nil {
			return
		}
		loadCtx, cancelLoad := context.WithTimeout(parent, storageOperationTimeout)
		checkpoint, err := c.store.LoadCheckpoint(loadCtx, source)
		loadContextErr := loadCtx.Err()
		cancelLoad()
		if err != nil || loadContextErr != nil || checkpoint.Validate() != nil {
			if parent.Err() == nil {
				c.recordStorageFailure(source, c.clock.Now().UTC(), nil)
			}
			prepared = append(prepared, preparedSource{source: source})
			continue
		}
		prepared = append(prepared, preparedSource{source: source, checkpoint: checkpoint.Clone(), ready: true})
	}

	type readResult struct {
		source Source
		batch  Batch
	}
	results := make([]readResult, 0, len(prepared))
	laneCtx, cancel := context.WithTimeout(parent, collectionTimeout)
	defer cancel()
	for _, item := range prepared {
		if parent.Err() != nil {
			return
		}
		if !item.ready {
			continue
		}
		batch, ok := c.readSource(parent, laneCtx, item.source, item.checkpoint)
		if !ok {
			if parent.Err() != nil {
				return
			}
			continue
		}
		results = append(results, readResult{source: item.source, batch: batch.Clone()})
	}
	cancel()

	// Persistence starts only after all native readers have left the shared lane,
	// so checkpoint I/O cannot consume another source's fixed native deadline.
	for _, result := range results {
		if parent.Err() != nil {
			return
		}
		commitCtx, cancelCommit := context.WithTimeout(parent, storageOperationTimeout)
		confirmed := c.commitConfirmed(commitCtx, result.source, result.batch)
		cancelCommit()
		if !confirmed {
			if parent.Err() == nil {
				c.recordStorageFailure(result.source, result.batch.QueryStartedAt, &result.batch)
			}
			continue
		}
		c.recordCommit(result.batch)
	}
}

func (c *Collector) readSource(parent, readerCtx context.Context, source Source, checkpoint Checkpoint) (Batch, bool) {
	queryStartedAt := c.clock.Now().UTC()
	request := ReadRequest{Source: source, Checkpoint: checkpoint.Clone(), QueryStartedAt: queryStartedAt}
	if request.Validate() != nil {
		if parent.Err() == nil {
			c.recordStorageFailure(source, queryStartedAt, nil)
		}
		return Batch{}, false
	}
	batch, readErr := c.reader.Read(readerCtx, request.Clone())
	if parent.Err() != nil {
		return Batch{}, false
	}
	if readErr != nil || readerCtx.Err() != nil || !validBatchForRequest(batch, request) {
		batch = c.readerFailureBatch(request)
	}
	return batch.Clone(), true
}

func validBatchForRequest(batch Batch, request ReadRequest) bool {
	if batch.Validate() != nil || batch.Source != request.Source || batch.ExpectedRevision != request.Checkpoint.Revision ||
		!batch.QueryStartedAt.Equal(request.QueryStartedAt) {
		return false
	}
	if request.Checkpoint.ResetPending {
		return batch.Kind == BatchResetPending || batch.Kind == BatchResetEstablished
	}
	if len(request.Checkpoint.Opaque) == 0 {
		return batch.Kind == BatchNormal
	}
	return true
}

func (c *Collector) readerFailureBatch(request ReadRequest) Batch {
	reason := ReasonReaderFailed
	kind := BatchNormal
	if request.Checkpoint.ResetPending {
		kind = BatchResetPending
	}
	finishedAt := c.clock.Now().UTC()
	if finishedAt.Before(request.QueryStartedAt) {
		finishedAt = request.QueryStartedAt
	}
	return Batch{
		Kind: kind, Source: request.Source, ExpectedRevision: request.Checkpoint.Revision,
		QueryStartedAt: request.QueryStartedAt, StartedAt: request.QueryStartedAt, FinishedAt: finishedAt,
		SupportState: SupportUnavailable, CollectionState: CollectionFailed, ReasonCode: &reason,
	}
}

func (c *Collector) commitConfirmed(ctx context.Context, source Source, batch Batch) bool {
	if ctx.Err() != nil {
		return false
	}
	if err := c.store.CommitBatch(ctx, batch.Clone()); err == nil {
		return ctx.Err() == nil
	} else if errors.Is(err, ErrRevisionConflict) {
		return false
	}
	if ctx.Err() != nil || batch.ExpectedRevision == ^uint64(0) {
		return false
	}
	checkpoint, err := c.store.LoadCheckpoint(ctx, source)
	if err != nil || checkpoint.Validate() != nil {
		return false
	}
	if checkpoint.Revision == batch.ExpectedRevision+1 {
		return true
	}
	if checkpoint.Revision != batch.ExpectedRevision {
		return false
	}
	if err := c.store.CommitBatch(ctx, batch.Clone()); err == nil {
		return ctx.Err() == nil
	} else if errors.Is(err, ErrRevisionConflict) {
		return false
	}
	if ctx.Err() != nil {
		return false
	}
	checkpoint, err = c.store.LoadCheckpoint(ctx, source)
	return err == nil && checkpoint.Validate() == nil && checkpoint.Revision == batch.ExpectedRevision+1
}

func (c *Collector) recordCommit(batch Batch) {
	c.mu.Lock()
	defer c.mu.Unlock()
	previous := c.latest[batch.Source]
	c.latest[batch.Source] = StatusAfterBatch(batch, previous)
	delete(c.overlays, batch.Source)
	if len(batch.Events) == 0 {
		return
	}
	c.events = append(c.events, batch.Events...)
	sort.SliceStable(c.events, func(i, j int) bool { return c.events[i].ObservedAt.After(c.events[j].ObservedAt) })
	if len(c.events) > MaxRecentEvents {
		bounded := make([]Event, MaxRecentEvents)
		copy(bounded, c.events[:MaxRecentEvents])
		c.events = bounded
	}
}

// StatusAfterBatch derives the latest source status after a durable commit.
// Callers must validate batch and any non-zero previous status before calling it.
func StatusAfterBatch(batch Batch, previous Status) Status {
	status := Status{
		SupportState: batch.SupportState, CollectionState: batch.CollectionState,
		Freshness: FreshnessUnknown, AttemptedAt: cloneTime(&batch.QueryStartedAt), ReasonCode: cloneReason(batch.ReasonCode),
	}
	if batch.Kind == BatchNormal && (batch.CaughtUp || len(batch.Events) > 0) {
		status.ObservedAt = cloneTime(&batch.QueryStartedAt)
		status.Freshness = FreshnessCurrent
		if batch.CaughtUp {
			status.CoverageThrough = cloneTime(&batch.QueryStartedAt)
		} else if previous.CoverageThrough != nil && !previous.CoverageThrough.After(batch.QueryStartedAt) {
			status.CoverageThrough = cloneTime(previous.CoverageThrough)
		}
		return status
	}
	if (batch.SupportState == SupportSupported && (batch.CollectionState == CollectionPartial || batch.CollectionState == CollectionFailed)) ||
		(batch.SupportState == SupportUnavailable && batch.CollectionState == CollectionFailed) {
		if previous.ObservedAt != nil && !previous.ObservedAt.After(batch.QueryStartedAt) {
			status.ObservedAt = cloneTime(previous.ObservedAt)
			status.CoverageThrough = cloneTime(previous.CoverageThrough)
			status.Freshness = FreshnessStale
		}
	}
	return status
}

func (c *Collector) recordStorageFailure(source Source, attemptedAt time.Time, attempted *Batch) {
	c.mu.Lock()
	defer c.mu.Unlock()
	previous, hasPrevious := c.latest[source]
	support := SupportUnavailable
	if attempted != nil && attempted.SupportState == SupportSupported {
		support = SupportSupported
	} else if attempted == nil && hasPrevious && previous.SupportState == SupportSupported {
		support = SupportSupported
	}
	reason := ReasonLogStorageUnavailable
	status := Status{
		SupportState: support, CollectionState: CollectionFailed, Freshness: FreshnessUnknown,
		AttemptedAt: cloneTime(&attemptedAt), ReasonCode: &reason,
	}
	if hasPrevious && previous.ObservedAt != nil && !previous.ObservedAt.After(attemptedAt) {
		status.ObservedAt = cloneTime(previous.ObservedAt)
		status.CoverageThrough = cloneTime(previous.CoverageThrough)
		status.Freshness = FreshnessStale
	}
	c.latest[source] = status
	c.overlays[source] = status.Clone()
}

func (c *Collector) Stop(ctx context.Context) error {
	c.lifecycleMu.Lock()
	if !c.started {
		c.lifecycleMu.Unlock()
		return nil
	}
	cancel, done := c.cancel, c.done
	c.lifecycleMu.Unlock()
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Collector) Current() Snapshot {
	c.mu.RLock()
	statuses := make([]Status, 0, len(c.sources))
	for _, source := range c.sources {
		if status, ok := c.latest[source]; ok {
			statuses = append(statuses, status.Clone())
		} else {
			reason := ReasonNotYetObserved
			statuses = append(statuses, Status{
				SupportState: SupportUnavailable, CollectionState: CollectionNotRun,
				Freshness: FreshnessUnknown, ReasonCode: &reason,
			})
		}
	}
	events := append([]Event(nil), c.events...)
	c.mu.RUnlock()
	status, err := AggregateStatus(statuses)
	if err != nil {
		reason := ReasonReaderFailed
		status = Status{SupportState: SupportUnavailable, CollectionState: CollectionFailed, Freshness: FreshnessUnknown, ReasonCode: &reason}
	}
	return Snapshot{Status: status, TotalCount: len(events), Events: events}
}

func (c *Collector) Summary(ctx context.Context, query SummaryQuery) (Summary, error) {
	if err := query.Validate(); err != nil {
		return Summary{}, err
	}
	if len(c.sources) == 0 {
		return Summary{
			WindowStart: query.WindowStart, WindowEnd: query.WindowEnd,
			BucketInterval: query.BucketInterval, BucketCount: query.BucketCount,
		}, nil
	}
	storeCtx, cancelStore := context.WithTimeout(ctx, storageOperationTimeout)
	summary, err := c.store.QuerySummary(storeCtx, append([]Source(nil), c.sources...), query)
	storeContextErr := storeCtx.Err()
	cancelStore()
	if err != nil {
		return Summary{}, err
	}
	if storeContextErr != nil {
		return Summary{}, errors.New("log summary store deadline exceeded")
	}
	if err := summary.Validate(); err != nil || !summaryMatchesRequest(summary, query, c.sources) {
		return Summary{}, errors.New("log store returned an invalid summary")
	}
	c.mu.RLock()
	overlays := make(map[Source]Status, len(c.overlays))
	for source, status := range c.overlays {
		overlays[source] = status.Clone()
	}
	c.mu.RUnlock()
	result := summary.Clone()
	for i := range result.Sources {
		if overlay, ok := overlays[result.Sources[i].Source]; ok && statusAttemptIsAfter(overlay, result.Sources[i].Status) {
			result.Sources[i].Status = mergeStorageOverlay(result.Sources[i].Status, overlay)
		}
	}
	if err := result.Validate(); err != nil {
		return Summary{}, errors.New("log summary overlay is invalid")
	}
	return result.Clone(), nil
}

func summaryMatchesRequest(summary Summary, query SummaryQuery, expected []Source) bool {
	if !summary.WindowStart.Equal(query.WindowStart) || !summary.WindowEnd.Equal(query.WindowEnd) ||
		summary.BucketInterval != query.BucketInterval || summary.BucketCount != query.BucketCount ||
		len(summary.Sources) != len(expected) {
		return false
	}
	for i := range summary.Sources {
		if summary.Sources[i].Source != expected[i] {
			return false
		}
	}
	return true
}

func statusAttemptIsAfter(candidate, durable Status) bool {
	return candidate.AttemptedAt != nil && (durable.AttemptedAt == nil || candidate.AttemptedAt.After(*durable.AttemptedAt))
}

func mergeStorageOverlay(stored Status, overlay Status) Status {
	result := overlay.Clone()
	if stored.ObservedAt != nil && (result.ObservedAt == nil || stored.ObservedAt.After(*result.ObservedAt)) {
		result.ObservedAt = cloneTime(stored.ObservedAt)
		result.Freshness = FreshnessStale
	}
	if stored.CoverageThrough != nil && (result.CoverageThrough == nil || stored.CoverageThrough.After(*result.CoverageThrough)) &&
		result.ObservedAt != nil && !stored.CoverageThrough.After(*result.ObservedAt) {
		result.CoverageThrough = cloneTime(stored.CoverageThrough)
	}
	return result
}
