package history

import (
	"context"
	"database/sql"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

func (s *Store) QuerySummary(ctx context.Context, sources []logobs.Source, query logobs.SummaryQuery) (logobs.Summary, error) {
	if query.Validate() != nil || len(sources) > logobs.MaxSources {
		return logobs.Summary{}, ErrLogStorageUnavailable
	}
	for i, source := range sources {
		if source.Validate() != nil || i > 0 && (sources[i-1] != logobs.SourceSystem || source != logobs.SourceApplication) {
			return logobs.Summary{}, ErrLogStorageUnavailable
		}
	}
	result := logobs.Summary{WindowStart: query.WindowStart, WindowEnd: query.WindowEnd, BucketInterval: query.BucketInterval, BucketCount: query.BucketCount, Sources: make([]logobs.SourceSummary, 0, len(sources))}
	if len(sources) == 0 {
		return result, nil
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return logobs.Summary{}, logPortError(ctx, err)
	}
	defer tx.Rollback()
	// Querying does not create tables or enable acquisition. After restart it may
	// validate existing additive tables but never initialize missing ones.
	if !s.logReady.Load() {
		if err := validateLogSchema(ctx, tx); err != nil {
			return logobs.Summary{}, logPortError(ctx, err)
		}
	}
	for _, source := range append([]logobs.Source(nil), sources...) {
		item, err := queryLogSource(ctx, tx, source, query)
		if err != nil {
			return logobs.Summary{}, logPortError(ctx, err)
		}
		result.Sources = append(result.Sources, item)
	}
	if err := result.Validate(); err != nil {
		return logobs.Summary{}, ErrLogStorageUnavailable
	}
	return result.Clone(), nil
}

func queryLogSource(ctx context.Context, tx *sql.Tx, source logobs.Source, query logobs.SummaryQuery) (logobs.SourceSummary, error) {
	state, err := loadLogState(ctx, tx, source)
	if err != nil {
		return logobs.SourceSummary{}, err
	}
	if state.checkpoint.Revision == 0 {
		reason := logobs.ReasonReaderFailed
		state.status = logobs.Status{SupportState: logobs.SupportUnavailable, CollectionState: logobs.CollectionNotRun, Freshness: logobs.FreshnessUnknown, ReasonCode: &reason}
	}
	segments, err := loadLogCoverage(ctx, tx, source, query.WindowStart, query.WindowEnd)
	if err != nil {
		return logobs.SourceSummary{}, err
	}
	counts, err := queryLogMinuteCounts(ctx, tx, source, query)
	if err != nil {
		return logobs.SourceSummary{}, err
	}
	result := logobs.SourceSummary{Source: source, Status: state.status.Clone(), Buckets: make([]logobs.SummaryBucket, query.BucketCount)}
	allFull, allUnknown, anyGap := true, true, false
	for i := range result.Buckets {
		start := query.WindowStart.Add(time.Duration(i) * query.BucketInterval)
		bucket, err := projectLogCoverage(segments, start, start.Add(query.BucketInterval))
		if err != nil {
			return logobs.SourceSummary{}, err
		}
		if c := counts[i]; c.Captured > 0 || c.Discarded > 0 {
			copy := c
			bucket.Counts = &copy
		}
		result.Buckets[i] = bucket
		result.CoveredSeconds += uint64(bucket.CoveredSeconds)
		allFull = allFull && bucket.CoverageState == logobs.CoverageFull
		allUnknown = allUnknown && bucket.CoverageState == logobs.CoverageUnknown
		anyGap = anyGap || bucket.CoverageState == logobs.CoverageGapState
		if bucket.Counts != nil {
			if result.Counts == nil {
				result.Counts = &logobs.Counts{}
			}
			if result.Counts.Captured > logobs.MaxSafeInteger-bucket.Counts.Captured || result.Counts.Discarded > logobs.MaxSafeInteger-bucket.Counts.Discarded {
				return logobs.SourceSummary{}, ErrLogStorageUnavailable
			}
			result.Counts.Captured += bucket.Counts.Captured
			result.Counts.Discarded += bucket.Counts.Discarded
		}
	}
	switch {
	case allFull:
		result.CoverageState = logobs.CoverageFull
	case allUnknown:
		result.CoverageState = logobs.CoverageUnknown
	case result.CoveredSeconds == 0 && anyGap:
		result.CoverageState = logobs.CoverageGapState
	default:
		result.CoverageState = logobs.CoveragePartial
	}
	return result, nil
}

func queryLogMinuteCounts(ctx context.Context, tx *sql.Tx, source logobs.Source, query logobs.SummaryQuery) ([]logobs.BucketCounts, error) {
	start, err := encodeLogTime(query.WindowStart)
	if err != nil {
		return nil, err
	}
	end, err := encodeLogTime(query.WindowEnd)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT minute,captured,discarded,trace,debug,info,warn,error,critical,unknown FROM log_metadata_minutes WHERE source=? AND minute>=? AND minute<? ORDER BY minute LIMIT 10081`, source, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]logobs.BucketCounts, query.BucketCount)
	for rows.Next() {
		var minute string
		var counts logobs.BucketCounts
		if err := rows.Scan(&minute, &counts.Captured, &counts.Discarded, &counts.Severity.Trace, &counts.Severity.Debug, &counts.Severity.Info, &counts.Severity.Warn, &counts.Severity.Error, &counts.Severity.Critical, &counts.Severity.Unknown); err != nil {
			return nil, err
		}
		at, err := decodeLogTime(minute)
		if err != nil || !at.Equal(at.Truncate(time.Minute)) {
			return nil, ErrLogStorageUnavailable
		}
		index := int(at.Sub(query.WindowStart) / query.BucketInterval)
		if index < 0 || index >= len(result) {
			return nil, ErrLogStorageUnavailable
		}
		if err := addLogCounts(&result[index], counts); err != nil {
			return nil, err
		}
	}
	return result, rows.Err()
}

func addLogCounts(target *logobs.BucketCounts, source logobs.BucketCounts) error {
	left := []*uint64{&target.Captured, &target.Discarded, &target.Severity.Trace, &target.Severity.Debug, &target.Severity.Info, &target.Severity.Warn, &target.Severity.Error, &target.Severity.Critical, &target.Severity.Unknown}
	right := []uint64{source.Captured, source.Discarded, source.Severity.Trace, source.Severity.Debug, source.Severity.Info, source.Severity.Warn, source.Severity.Error, source.Severity.Critical, source.Severity.Unknown}
	var severity uint64
	for _, value := range right[2:] {
		if value > logobs.MaxSafeInteger || severity > logobs.MaxSafeInteger-value {
			return ErrLogStorageUnavailable
		}
		severity += value
	}
	if severity != source.Captured {
		return ErrLogStorageUnavailable
	}
	for i, value := range right {
		if value > logobs.MaxSafeInteger || *left[i] > logobs.MaxSafeInteger-value {
			return ErrLogStorageUnavailable
		}
		*left[i] += value
	}
	return nil
}
