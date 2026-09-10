package history

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

const maxLogCoverageRows = 2*7*24*60 + 2

func writeLogMinutes(ctx context.Context, tx *sql.Tx, batch logobs.Batch, floor time.Time) error {
	minutes := make(map[string]*logobs.BucketCounts)
	get := func(at time.Time) (*logobs.BucketCounts, error) {
		key, err := encodeLogTime(at.Truncate(time.Minute))
		if err != nil {
			return nil, err
		}
		if minutes[key] == nil {
			minutes[key] = &logobs.BucketCounts{}
		}
		return minutes[key], nil
	}
	for _, event := range batch.Events {
		if event.ObservedAt.Before(floor) {
			continue
		}
		counts, err := get(event.ObservedAt)
		if err != nil {
			return err
		}
		counts.Captured++
		switch event.Severity {
		case logobs.SeverityTrace:
			counts.Severity.Trace++
		case logobs.SeverityDebug:
			counts.Severity.Debug++
		case logobs.SeverityInfo:
			counts.Severity.Info++
		case logobs.SeverityWarn:
			counts.Severity.Warn++
		case logobs.SeverityError:
			counts.Severity.Error++
		case logobs.SeverityCritical:
			counts.Severity.Critical++
		case logobs.SeverityUnknown:
			counts.Severity.Unknown++
		}
	}
	for _, discard := range batch.Discards {
		if discard.At.Before(floor) {
			continue
		}
		counts, err := get(discard.At)
		if err != nil {
			return err
		}
		counts.Discarded += uint64(discard.Count)
	}
	keys := make([]string, 0, len(minutes))
	for key := range minutes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		c := minutes[key]
		if _, err := tx.ExecContext(ctx, `INSERT INTO log_metadata_minutes(source,minute,captured,discarded,trace,debug,info,warn,error,critical,unknown) VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(source,minute) DO UPDATE SET captured=captured+excluded.captured,discarded=discarded+excluded.discarded,trace=trace+excluded.trace,debug=debug+excluded.debug,info=info+excluded.info,warn=warn+excluded.warn,error=error+excluded.error,critical=critical+excluded.critical,unknown=unknown+excluded.unknown`,
			batch.Source, key, c.Captured, c.Discarded, c.Severity.Trace, c.Severity.Debug, c.Severity.Info, c.Severity.Warn, c.Severity.Error, c.Severity.Critical, c.Severity.Unknown); err != nil {
			return err
		}
	}
	return nil
}

func loadLogCoverage(ctx context.Context, tx *sql.Tx, source logobs.Source, start, end time.Time) ([]logCoverageSegment, error) {
	left, err := encodeLogTime(start)
	if err != nil {
		return nil, err
	}
	right, err := encodeLogTime(end)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT start_at,end_at,reason FROM log_metadata_coverage WHERE source=? AND end_at>? AND start_at<? ORDER BY start_at LIMIT ?`, source, left, right, maxLogCoverageRows+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var segments []logCoverageSegment
	for rows.Next() {
		var a, b string
		var reason logobs.ReasonCode
		if err := rows.Scan(&a, &b, &reason); err != nil {
			return nil, err
		}
		from, err := decodeLogTime(a)
		if err != nil {
			return nil, err
		}
		to, err := decodeLogTime(b)
		if err != nil {
			return nil, err
		}
		segment := logCoverageSegment{from, to, reason}
		if !validLogSegment(segment) || len(segments) > 0 && segments[len(segments)-1].End.After(from) {
			return nil, ErrLogStorageUnavailable
		}
		segments = append(segments, segment)
		if len(segments) > maxLogCoverageRows {
			return nil, ErrLogStorageUnavailable
		}
	}
	return segments, rows.Err()
}

// Delta writes avoid rewriting seven days of evidence on each minute poll.
// Old rows outside the queried retention window belong to shared maintenance.
func writeLogCoverage(ctx context.Context, tx *sql.Tx, source logobs.Source, old, next []logCoverageSegment) error {
	oldByStart := make(map[time.Time]logCoverageSegment, len(old))
	nextByStart := make(map[time.Time]logCoverageSegment, len(next))
	for _, s := range old {
		oldByStart[s.Start] = s
	}
	for _, s := range next {
		nextByStart[s.Start] = s
	}
	for _, s := range old {
		if _, ok := nextByStart[s.Start]; ok {
			continue
		}
		at, err := encodeLogTime(s.Start)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM log_metadata_coverage WHERE source=? AND start_at=?`, source, at); err != nil {
			return err
		}
	}
	for _, s := range next {
		if previous, ok := oldByStart[s.Start]; ok && previous == s {
			continue
		}
		start, err := encodeLogTime(s.Start)
		if err != nil {
			return err
		}
		end, err := encodeLogTime(s.End)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO log_metadata_coverage(source,start_at,end_at,reason) VALUES(?,?,?,?) ON CONFLICT(source,start_at) DO UPDATE SET end_at=excluded.end_at,reason=excluded.reason`, source, start, end, s.Reason); err != nil {
			return err
		}
	}
	return nil
}
