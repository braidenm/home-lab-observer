package logobs

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

const maxSummaryBuckets = 168

var (
	reasonPattern       = regexp.MustCompile(`^[A-Z0-9_]{1,64}$`)
	windowsCodePattern  = regexp.MustCompile(`^WIN_(?:([0-9]{1,10})|([0-9a-f]{32})_([0-9]{1,10}))$`)
	systemdCodePattern  = regexp.MustCompile(`^SYSTEMD_([0-9a-f]{32})$`)
	priorityCodePattern = regexp.MustCompile(`^SYSTEMD_PRIORITY_([0-7])$`)
)

func (s Source) Validate() error {
	if s != SourceSystem && s != SourceApplication {
		return errors.New("invalid log source")
	}
	return nil
}

func (s Severity) Validate() error {
	switch s {
	case SeverityTrace, SeverityDebug, SeverityInfo, SeverityWarn, SeverityError, SeverityCritical, SeverityUnknown:
		return nil
	default:
		return errors.New("invalid log severity")
	}
}

func (s SupportState) Validate() error {
	switch s {
	case SupportSupported, SupportDisabled, SupportUnavailable, SupportPermissionDenied, SupportUnsupported:
		return nil
	default:
		return errors.New("invalid support state")
	}
}

func (s CollectionState) Validate() error {
	switch s {
	case CollectionOK, CollectionPartial, CollectionFailed, CollectionNotRun:
		return nil
	default:
		return errors.New("invalid collection state")
	}
}

func (f Freshness) Validate() error {
	switch f {
	case FreshnessCurrent, FreshnessStale, FreshnessUnknown:
		return nil
	default:
		return errors.New("invalid freshness")
	}
}

func (r ReasonCode) Validate() error {
	if !reasonPattern.MatchString(string(r)) || !knownReason(r) {
		return errors.New("invalid reason code")
	}
	return nil
}

func knownReason(reason ReasonCode) bool {
	switch reason {
	case ReasonCheckpointReset, ReasonPermissionDenied, ReasonDeadlineExceeded,
		ReasonInvalidResponse, ReasonResponseTooLarge, ReasonReaderFailed,
		ReasonBacklogDeferred, ReasonMissedCollection, ReasonNotYetObserved,
		ReasonLogStorageUnavailable, ReasonNoVisibleJournal, ReasonLogHelperUnavailable,
		ReasonLogHelperMismatch, ReasonPlatformUnsupported, ReasonLogSourcesDisabled,
		ReasonSourcePartial:
		return true
	default:
		return false
	}
}

func (k BatchKind) Validate() error {
	switch k {
	case BatchNormal, BatchResetPending, BatchResetEstablished:
		return nil
	default:
		return errors.New("invalid batch kind")
	}
}

func (c CoverageState) Validate() error {
	switch c {
	case CoverageFull, CoveragePartial, CoverageGapState, CoverageUnknown:
		return nil
	default:
		return errors.New("invalid coverage state")
	}
}

func HistoricalReasonRank(reason ReasonCode) (uint8, bool) {
	for rank, candidate := range [...]ReasonCode{
		ReasonCheckpointReset,
		ReasonPermissionDenied,
		ReasonDeadlineExceeded,
		ReasonInvalidResponse,
		ReasonResponseTooLarge,
		ReasonReaderFailed,
		ReasonBacklogDeferred,
		ReasonMissedCollection,
		ReasonNotYetObserved,
	} {
		if reason == candidate {
			return uint8(rank), true
		}
	}
	return 0, false
}

func (e Event) Validate() error {
	if err := validateUTC(e.ObservedAt, "event observed time"); err != nil {
		return err
	}
	if err := e.Source.Validate(); err != nil {
		return err
	}
	if err := e.Severity.Validate(); err != nil {
		return err
	}
	if len(e.EventCode) == 0 || len(e.EventCode) > 64 || !validEventCode(e.EventCode) {
		return errors.New("invalid event code")
	}
	return nil
}

func validEventCode(code string) bool {
	if match := windowsCodePattern.FindStringSubmatch(code); match != nil {
		number := match[1]
		if number == "" {
			number = match[3]
		}
		return canonicalUint32(number)
	}
	return systemdCodePattern.MatchString(code) || priorityCodePattern.MatchString(code)
}

func canonicalUint32(value string) bool {
	parsed, err := strconv.ParseUint(value, 10, 32)
	return err == nil && strconv.FormatUint(parsed, 10) == value
}

func (s Status) Validate() error {
	if err := s.SupportState.Validate(); err != nil {
		return err
	}
	if err := s.CollectionState.Validate(); err != nil {
		return err
	}
	if err := s.Freshness.Validate(); err != nil {
		return err
	}
	if err := validateReasonPointer(s.ReasonCode); err != nil {
		return err
	}
	for name, value := range map[string]*time.Time{
		"observed time": s.ObservedAt, "attempted time": s.AttemptedAt, "coverage time": s.CoverageThrough,
	} {
		if value != nil {
			if err := validateUTC(*value, name); err != nil {
				return err
			}
		}
	}
	if s.Freshness == FreshnessCurrent || s.Freshness == FreshnessStale {
		if s.ObservedAt == nil {
			return errors.New("current or stale status requires observed time")
		}
	} else if s.ObservedAt != nil {
		return errors.New("unknown freshness cannot have observed time")
	}
	if s.CoverageThrough != nil && s.ObservedAt == nil {
		return errors.New("coverage time requires observed time")
	}
	if s.ObservedAt != nil && s.AttemptedAt == nil {
		return errors.New("observed time requires attempted time")
	}
	if s.CoverageThrough != nil && s.CoverageThrough.After(*s.ObservedAt) {
		return errors.New("coverage time exceeds observed time")
	}
	if s.AttemptedAt != nil {
		if s.ObservedAt != nil && s.ObservedAt.After(*s.AttemptedAt) {
			return errors.New("observed time exceeds attempted time")
		}
		if s.CoverageThrough != nil && s.CoverageThrough.After(*s.AttemptedAt) {
			return errors.New("coverage time exceeds attempted time")
		}
	}
	if s.CollectionState == CollectionOK {
		if s.SupportState != SupportSupported || s.ReasonCode != nil || s.ObservedAt == nil || s.AttemptedAt == nil || s.Freshness == FreshnessUnknown {
			return errors.New("successful status is inconsistent")
		}
	} else if s.ReasonCode == nil {
		return errors.New("non-success status requires reason")
	}
	if (s.CollectionState == CollectionPartial) && s.SupportState != SupportSupported {
		return errors.New("partial status requires supported source")
	}
	if s.CollectionState == CollectionNotRun && s.SupportState == SupportSupported {
		return errors.New("supported status cannot be not-run")
	}
	if s.SupportState != SupportSupported {
		if s.SupportState != SupportUnavailable && s.CollectionState != CollectionNotRun {
			return errors.New("non-supported status implies collection")
		}
		if s.CollectionState == CollectionNotRun && (s.Freshness != FreshnessUnknown || s.ObservedAt != nil) {
			return errors.New("not-run status must have unknown freshness")
		}
	}
	if s.CollectionState == CollectionFailed && s.Freshness == FreshnessCurrent {
		return errors.New("failed status cannot be current")
	}
	return nil
}

func (c Checkpoint) Validate() error {
	if len(c.Opaque) > MaxCheckpointBytes {
		return errors.New("checkpoint exceeds size bound")
	}
	if c.ResetPending && len(c.Opaque) != 0 {
		return errors.New("reset-pending checkpoint cannot have opaque data")
	}
	if c.ResetPending && c.Revision == 0 {
		return errors.New("initial checkpoint cannot be reset-pending")
	}
	if c.Revision > 0 && c.PreviousAttemptAt == nil {
		return errors.New("durable checkpoint requires previous attempt")
	}
	if c.Revision == 0 && (len(c.Opaque) != 0 || c.PreviousAttemptAt != nil || c.CoverageThrough != nil) {
		return errors.New("initial checkpoint cannot have durable state")
	}
	if c.PreviousAttemptAt != nil {
		if err := validateUTC(*c.PreviousAttemptAt, "previous attempt time"); err != nil {
			return err
		}
	}
	if c.CoverageThrough != nil {
		if err := validateUTC(*c.CoverageThrough, "coverage time"); err != nil {
			return err
		}
		if c.PreviousAttemptAt == nil || c.CoverageThrough.After(*c.PreviousAttemptAt) {
			return errors.New("checkpoint coverage exceeds previous attempt")
		}
	}
	return nil
}

func (r ReadRequest) Validate() error {
	if err := r.Source.Validate(); err != nil {
		return err
	}
	if err := r.Checkpoint.Validate(); err != nil {
		return err
	}
	if err := validateUTC(r.QueryStartedAt, "query start time"); err != nil {
		return err
	}
	if previous := r.Checkpoint.PreviousAttemptAt; previous != nil && !r.QueryStartedAt.After(*previous) {
		return errors.New("query start must follow previous attempt")
	}
	if coverage := r.Checkpoint.CoverageThrough; coverage != nil && coverage.After(r.QueryStartedAt) {
		return errors.New("coverage exceeds query start")
	}
	return nil
}

func (b Batch) Validate() error {
	if err := b.Kind.Validate(); err != nil {
		return err
	}
	if err := b.Source.Validate(); err != nil {
		return err
	}
	if err := b.SupportState.Validate(); err != nil {
		return err
	}
	if err := b.CollectionState.Validate(); err != nil {
		return err
	}
	if err := validateReasonPointer(b.ReasonCode); err != nil {
		return err
	}
	if err := validateOrderedTimes(b.QueryStartedAt, b.StartedAt, b.FinishedAt); err != nil {
		return err
	}
	if len(b.NextOpaque) > MaxCheckpointBytes {
		return errors.New("next checkpoint exceeds size bound")
	}
	if len(b.Events) > MaxAcceptedEvents || len(b.Discards) > MaxAcceptedEvents || b.ExaminedCount > MaxExaminedEvents || b.ProbeCount > MaxProbeEvents {
		return errors.New("batch exceeds event bounds")
	}
	if b.CollectionState == CollectionOK {
		if b.SupportState != SupportSupported || b.ReasonCode != nil {
			return errors.New("successful batch is inconsistent")
		}
	} else if b.ReasonCode == nil {
		return errors.New("non-success batch requires reason")
	}
	if b.CollectionState == CollectionPartial && b.SupportState != SupportSupported {
		return errors.New("partial batch requires supported source")
	}
	if b.CollectionState == CollectionNotRun && b.SupportState == SupportSupported {
		return errors.New("supported batch cannot be not-run")
	}
	if b.SupportState != SupportSupported && b.SupportState != SupportUnavailable && b.CollectionState != CollectionNotRun {
		return errors.New("non-supported batch implies collection")
	}
	if err := b.validateNativeState(); err != nil {
		return err
	}

	var discarded uint64
	for _, discard := range b.Discards {
		if err := validateUTC(discard.At, "discard time"); err != nil {
			return err
		}
		if discard.Count == 0 {
			return errors.New("discard count must be positive")
		}
		var ok bool
		discarded, ok = addSafe(discarded, uint64(discard.Count))
		if !ok {
			return errors.New("discard count exceeds safe integer")
		}
	}
	if discarded != uint64(b.DiscardedCount) {
		return errors.New("discard counts are inconsistent")
	}
	for _, event := range b.Events {
		if err := event.Validate(); err != nil {
			return err
		}
		if event.Source != b.Source {
			return errors.New("event source does not match batch")
		}
	}

	if b.Kind != BatchNormal {
		return b.validateResetBatch()
	}
	if b.ProbeCount > 1 {
		return errors.New("normal batch exceeds probe bound")
	}
	acceptedAndDiscarded := uint64(len(b.Events)) + discarded
	if acceptedAndDiscarded > MaxAcceptedEvents {
		return errors.New("batch exceeds accepted event bound")
	}
	expectedExamined := acceptedAndDiscarded + uint64(b.ProbeCount)
	if b.Deferred {
		expectedExamined++
	}
	if uint64(b.ExaminedCount) != expectedExamined {
		return errors.New("examined count is inconsistent")
	}
	if b.Deferred && b.CaughtUp {
		return errors.New("deferred batch cannot be caught up")
	}
	if b.CollectionState == CollectionOK && !b.CaughtUp {
		return errors.New("successful batch requires caught-up proof")
	}
	if b.CollectionState == CollectionOK && b.DiscardedCount != 0 {
		return errors.New("successful batch cannot have discarded rows")
	}
	if b.CaughtUp && b.CollectionState == CollectionPartial && (b.ReasonCode == nil || *b.ReasonCode != ReasonInvalidResponse || b.DiscardedCount == 0) {
		return errors.New("caught-up partial batch is inconsistent")
	}
	if (acceptedAndDiscarded > 0 || b.ProbeCount > 0) && len(b.NextOpaque) == 0 {
		return errors.New("observed batch requires next checkpoint")
	}
	if b.CaughtUp && b.CollectionState == CollectionFailed {
		return errors.New("failed batch cannot be caught up")
	}
	if b.CollectionState == CollectionFailed || b.CollectionState == CollectionNotRun {
		if len(b.Events) != 0 || len(b.Discards) != 0 || b.ExaminedCount != 0 || b.ProbeCount != 0 || b.DiscardedCount != 0 || b.Deferred || b.CaughtUp || len(b.NextOpaque) != 0 {
			return errors.New("failed or not-run batch cannot carry observations")
		}
	}
	return nil
}

func (b Batch) validateNativeState() error {
	reason := ReasonCode("")
	if b.ReasonCode != nil {
		reason = *b.ReasonCode
	}
	switch b.SupportState {
	case SupportSupported:
		switch b.CollectionState {
		case CollectionOK:
			return nil
		case CollectionPartial:
			if b.Kind != BatchNormal && reason == ReasonCheckpointReset {
				return nil
			}
			if reason == ReasonInvalidResponse || reason == ReasonDeadlineExceeded || reason == ReasonResponseTooLarge || reason == ReasonBacklogDeferred {
				return nil
			}
		case CollectionFailed:
			if reason == ReasonDeadlineExceeded || reason == ReasonInvalidResponse || reason == ReasonResponseTooLarge || reason == ReasonReaderFailed {
				return nil
			}
		}
	case SupportUnavailable:
		if b.CollectionState == CollectionFailed && reason == ReasonReaderFailed {
			return nil
		}
		if b.CollectionState == CollectionNotRun && (reason == ReasonNoVisibleJournal || reason == ReasonLogHelperUnavailable || reason == ReasonLogHelperMismatch || reason == ReasonReaderFailed) {
			return nil
		}
	case SupportPermissionDenied:
		if b.CollectionState == CollectionNotRun && reason == ReasonPermissionDenied {
			return nil
		}
	case SupportUnsupported:
		if b.CollectionState == CollectionNotRun && reason == ReasonPlatformUnsupported {
			return nil
		}
	}
	return errors.New("native batch state or reason is inconsistent")
}

func (b Batch) validateResetBatch() error {
	if len(b.Events) != 0 || len(b.Discards) != 0 || b.DiscardedCount != 0 || b.Deferred || b.CaughtUp {
		return errors.New("reset batch cannot carry observations")
	}
	if b.ExaminedCount != b.ProbeCount {
		return errors.New("reset examined count is inconsistent")
	}
	if b.Kind == BatchResetEstablished {
		if b.ProbeCount == 0 || b.ProbeCount > MaxProbeEvents || len(b.NextOpaque) == 0 || b.ReasonCode == nil || *b.ReasonCode != ReasonCheckpointReset ||
			b.SupportState != SupportSupported || b.CollectionState != CollectionPartial {
			return errors.New("established reset batch is inconsistent")
		}
		return nil
	}
	if b.ProbeCount > MaxProbeEvents || len(b.NextOpaque) != 0 || b.CollectionState == CollectionOK || b.ReasonCode == nil {
		return errors.New("pending reset batch is inconsistent")
	}
	return nil
}

func (q SummaryQuery) Validate() error {
	if err := validateUTC(q.WindowStart, "window start"); err != nil {
		return err
	}
	if err := validateUTC(q.WindowEnd, "window end"); err != nil {
		return err
	}
	if !q.WindowStart.Before(q.WindowEnd) {
		return errors.New("summary window is inverted")
	}
	validGrid := (q.BucketInterval == time.Minute && q.BucketCount == 60) ||
		(q.BucketInterval == 5*time.Minute && q.BucketCount == 72) ||
		(q.BucketInterval == 15*time.Minute && q.BucketCount == 96) ||
		(q.BucketInterval == time.Hour && q.BucketCount == 168)
	if !validGrid || q.BucketCount > maxSummaryBuckets {
		return errors.New("invalid summary grid")
	}
	if q.WindowEnd.Sub(q.WindowStart) != time.Duration(q.BucketCount)*q.BucketInterval {
		return errors.New("summary window does not match grid")
	}
	seconds := int64(q.BucketInterval / time.Second)
	if q.WindowStart.Nanosecond() != 0 || q.WindowEnd.Nanosecond() != 0 || q.WindowStart.Unix()%seconds != 0 || q.WindowEnd.Unix()%seconds != 0 {
		return errors.New("summary window is not epoch aligned")
	}
	return nil
}

func (s Summary) Validate() error {
	query := SummaryQuery{WindowStart: s.WindowStart, WindowEnd: s.WindowEnd, BucketInterval: s.BucketInterval, BucketCount: s.BucketCount}
	if err := query.Validate(); err != nil {
		return err
	}
	if len(s.Sources) > MaxSources {
		return errors.New("summary exceeds source bound")
	}
	previousRank := -1
	var totalCaptured, totalDiscarded uint64
	for i := range s.Sources {
		rank := sourceRank(s.Sources[i].Source)
		if rank < 0 || rank <= previousRank {
			return errors.New("summary sources are not in fixed order")
		}
		previousRank = rank
		if err := validateSourceSummary(s.Sources[i], query); err != nil {
			return fmt.Errorf("invalid source summary: %w", err)
		}
		if counts := s.Sources[i].Counts; counts != nil {
			var ok bool
			if totalCaptured, ok = addSafe(totalCaptured, counts.Captured); !ok {
				return errors.New("cross-source captured count exceeds safe integer")
			}
			if totalDiscarded, ok = addSafe(totalDiscarded, counts.Discarded); !ok {
				return errors.New("cross-source discarded count exceeds safe integer")
			}
		}
	}
	return nil
}

func sourceRank(source Source) int {
	switch source {
	case SourceSystem:
		return 0
	case SourceApplication:
		return 1
	default:
		return -1
	}
}

func validateSourceSummary(source SourceSummary, query SummaryQuery) error {
	if err := source.Source.Validate(); err != nil {
		return err
	}
	if err := source.Status.Validate(); err != nil {
		return err
	}
	if err := validateSourceStatus(source.Status); err != nil {
		return err
	}
	if err := source.CoverageState.Validate(); err != nil {
		return err
	}
	if len(source.Buckets) != query.BucketCount {
		return errors.New("source has wrong bucket count")
	}
	var covered uint64
	var totals *Counts
	states := make([]CoverageState, len(source.Buckets))
	for i, bucket := range source.Buckets {
		expectedAt := query.WindowStart.Add(time.Duration(i) * query.BucketInterval)
		if !bucket.At.Equal(expectedAt) || bucket.At.Location() != time.UTC {
			return errors.New("summary buckets are not on the fixed grid")
		}
		if err := validateSummaryBucket(bucket, uint32(query.BucketInterval/time.Second)); err != nil {
			return err
		}
		var ok bool
		covered, ok = addSafe(covered, uint64(bucket.CoveredSeconds))
		if !ok {
			return errors.New("covered seconds exceed safe integer")
		}
		states[i] = bucket.CoverageState
		if bucket.Counts != nil {
			if totals == nil {
				totals = &Counts{}
			}
			if totals.Captured, ok = addSafe(totals.Captured, bucket.Counts.Captured); !ok {
				return errors.New("captured count exceeds safe integer")
			}
			if totals.Discarded, ok = addSafe(totals.Discarded, bucket.Counts.Discarded); !ok {
				return errors.New("discarded count exceeds safe integer")
			}
		}
	}
	if covered != source.CoveredSeconds || !equalCounts(totals, source.Counts) {
		return errors.New("source aggregates are inconsistent")
	}
	if source.CoverageState != aggregateCoverage(states) {
		return errors.New("source coverage is inconsistent")
	}
	return nil
}

func validateSourceStatus(status Status) error {
	reason := ReasonCode("")
	if status.ReasonCode != nil {
		reason = *status.ReasonCode
	}
	switch status.SupportState {
	case SupportSupported:
		switch status.CollectionState {
		case CollectionOK:
			if reason == "" {
				return nil
			}
		case CollectionPartial:
			if reason == ReasonCheckpointReset || reason == ReasonInvalidResponse || reason == ReasonDeadlineExceeded ||
				reason == ReasonResponseTooLarge || reason == ReasonBacklogDeferred {
				return nil
			}
		case CollectionFailed:
			if reason == ReasonDeadlineExceeded || reason == ReasonInvalidResponse || reason == ReasonResponseTooLarge ||
				reason == ReasonReaderFailed || reason == ReasonLogStorageUnavailable {
				return nil
			}
		}
	case SupportUnavailable:
		if status.CollectionState == CollectionNotRun && (reason == ReasonNoVisibleJournal || reason == ReasonLogHelperUnavailable ||
			reason == ReasonLogHelperMismatch || reason == ReasonReaderFailed) {
			return nil
		}
		if status.CollectionState == CollectionFailed && (reason == ReasonReaderFailed || reason == ReasonLogStorageUnavailable) {
			return nil
		}
	case SupportPermissionDenied:
		if status.CollectionState == CollectionNotRun && reason == ReasonPermissionDenied {
			return nil
		}
	case SupportUnsupported:
		if status.CollectionState == CollectionNotRun && reason == ReasonPlatformUnsupported {
			return nil
		}
	}
	return errors.New("source summary status or reason is inconsistent")
}

func validateSummaryBucket(bucket SummaryBucket, intervalSeconds uint32) error {
	if err := bucket.CoverageState.Validate(); err != nil {
		return err
	}
	if err := validateReasonPointer(bucket.ReasonCode); err != nil {
		return err
	}
	switch bucket.CoverageState {
	case CoverageFull:
		if bucket.CoveredSeconds != intervalSeconds || bucket.ReasonCode != nil || bucket.Counts == nil {
			return errors.New("full bucket is inconsistent")
		}
	case CoveragePartial:
		if bucket.CoveredSeconds == 0 || bucket.CoveredSeconds >= intervalSeconds || !isHistoricalReason(bucket.ReasonCode) || bucket.Counts == nil {
			return errors.New("partial bucket is inconsistent")
		}
	case CoverageGapState:
		if bucket.CoveredSeconds != 0 || !isGapReason(bucket.ReasonCode) {
			return errors.New("gap bucket is inconsistent")
		}
	case CoverageUnknown:
		if bucket.CoveredSeconds != 0 || bucket.ReasonCode == nil || *bucket.ReasonCode != ReasonNotYetObserved {
			return errors.New("unknown bucket is inconsistent")
		}
	}
	if bucket.Counts != nil {
		if err := validateBucketCounts(*bucket.Counts); err != nil {
			return err
		}
		if (bucket.CoverageState == CoverageGapState || bucket.CoverageState == CoverageUnknown) && bucket.Counts.Captured == 0 && bucket.Counts.Discarded == 0 {
			return errors.New("zero counts cannot stand for unknown")
		}
	}
	return nil
}

func validateBucketCounts(counts BucketCounts) error {
	values := []uint64{counts.Captured, counts.Discarded, counts.Severity.Trace, counts.Severity.Debug, counts.Severity.Info, counts.Severity.Warn, counts.Severity.Error, counts.Severity.Critical, counts.Severity.Unknown}
	for _, value := range values {
		if value > MaxSafeInteger {
			return errors.New("summary count exceeds safe integer")
		}
	}
	var severityTotal uint64
	for _, value := range values[2:] {
		var ok bool
		severityTotal, ok = addSafe(severityTotal, value)
		if !ok {
			return errors.New("severity count exceeds safe integer")
		}
	}
	if severityTotal != counts.Captured {
		return errors.New("captured and severity counts differ")
	}
	return nil
}

func (s Snapshot) Validate() error {
	if err := s.Status.Validate(); err != nil {
		return err
	}
	if s.TotalCount < 0 || s.TotalCount > MaxRecentEvents || s.TotalCount != len(s.Events) {
		return errors.New("snapshot count is inconsistent")
	}
	for i, event := range s.Events {
		if err := event.Validate(); err != nil {
			return err
		}
		if i > 0 && event.ObservedAt.After(s.Events[i-1].ObservedAt) {
			return errors.New("snapshot events are not newest first")
		}
	}
	return nil
}

func aggregateCoverage(states []CoverageState) CoverageState {
	if len(states) == 0 {
		return CoverageUnknown
	}
	allFull, allUnknown, anyGap := true, true, false
	for _, state := range states {
		allFull = allFull && state == CoverageFull
		allUnknown = allUnknown && state == CoverageUnknown
		anyGap = anyGap || state == CoverageGapState
	}
	if allFull {
		return CoverageFull
	}
	if allUnknown {
		return CoverageUnknown
	}
	if anyGap {
		allZeroCoverage := true
		for _, state := range states {
			if state == CoverageFull || state == CoveragePartial {
				allZeroCoverage = false
				break
			}
		}
		if allZeroCoverage {
			return CoverageGapState
		}
	}
	return CoveragePartial
}

func isGapReason(reason *ReasonCode) bool {
	if reason == nil || *reason == ReasonNotYetObserved {
		return false
	}
	_, ok := HistoricalReasonRank(*reason)
	return ok
}

func isHistoricalReason(reason *ReasonCode) bool {
	if reason == nil {
		return false
	}
	_, ok := HistoricalReasonRank(*reason)
	return ok
}

func equalCounts(left, right *Counts) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Captured == right.Captured && left.Discarded == right.Discarded
}

func validateReasonPointer(reason *ReasonCode) error {
	if reason == nil {
		return nil
	}
	return reason.Validate()
}

func validateUTC(value time.Time, name string) error {
	if value.IsZero() || value.Location() != time.UTC || value.Year() < 1 || value.Year() > 9999 {
		return fmt.Errorf("%s must be non-zero UTC", name)
	}
	return nil
}

func validateOrderedTimes(query, started, finished time.Time) error {
	if err := validateUTC(query, "query start time"); err != nil {
		return err
	}
	if err := validateUTC(started, "batch start time"); err != nil {
		return err
	}
	if err := validateUTC(finished, "batch finish time"); err != nil {
		return err
	}
	if started.Before(query) || finished.Before(started) {
		return errors.New("batch times are inverted")
	}
	return nil
}

func addSafe(left, right uint64) (uint64, bool) {
	if left > MaxSafeInteger || right > MaxSafeInteger || right > MaxSafeInteger-left {
		return 0, false
	}
	return left + right, true
}
