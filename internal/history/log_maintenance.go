package history

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

// One candidate per table keeps memory constant. All mutations, including proof
// invalidation needed before removing counts, share the caller's batch budget.
type retentionCandidate struct {
	kind           int
	key            string
	at             time.Time
	ns, resolution int64
	text, end      string
}

// Each scalar minimum is a primary-key seek within one fixed metric. Sorting
// the six candidates avoids repeatedly scanning the entire host history.
const retentionMetrics = `WITH metrics(metric) AS (VALUES
('cpu.utilization.percent'),('memory.utilization.percent'),('filesystem.aggregate.utilization.percent'),
('network.receive.bytes_per_second'),('network.transmit.bytes_per_second'),('process.count')) `

func (s *Store) pruneShared(ctx context.Context, tx *sql.Tx, cutoff *time.Time) (int64, error) {
	logCutoff := cutoff
	if cutoff != nil && s.config.RetentionAge > logRetention {
		at := cutoff.Add(s.config.RetentionAge - logRetention)
		logCutoff = &at
	}
	var work, dropped int64
	frontiers := map[logobs.Source]time.Time{}
	for work < int64(s.config.BatchSize) {
		item, found, err := oldestRetentionCandidate(ctx, tx, cutoff, logCutoff)
		if err != nil {
			return dropped, err
		}
		if !found {
			break
		}
		if item.kind == 2 {
			// Removing proof first is safe even when the budget ends before the
			// corresponding count deletion: known counts remain, coverage is unknown.
			changed, removed, err := trimPositiveLogProof(ctx, tx, item.key, item.at.Add(time.Minute))
			if err != nil {
				return dropped, err
			}
			if changed {
				work++
				if removed {
					dropped++
				}
				continue
			}
		}
		var errMutation error
		removed := true
		switch item.kind {
		case 0:
			_, errMutation = tx.ExecContext(ctx, `DELETE FROM samples WHERE metric=? AND observed_at_ns=?`, item.key, item.ns)
		case 1:
			_, errMutation = tx.ExecContext(ctx, `DELETE FROM rollups WHERE metric=? AND bucket_start_ns=? AND resolution_seconds=?`, item.key, item.ns, item.resolution)
		case 2:
			_, errMutation = tx.ExecContext(ctx, `DELETE FROM log_metadata_minutes WHERE source=? AND minute=?`, item.key, item.text)
			frontier := item.at.Add(time.Minute)
			if frontier.Year() > 9999 {
				frontier = time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC)
			}
			source := logobs.Source(item.key)
			if frontier.After(frontiers[source]) {
				frontiers[source] = frontier
			}
		case 3:
			if logCutoff != nil && item.end > logCutoff.UTC().Format(logTimeLayout) {
				removed = false
				_, errMutation = tx.ExecContext(ctx, `UPDATE log_metadata_coverage SET start_at=? WHERE source=? AND start_at=?`, logCutoff.UTC().Format(logTimeLayout), item.key, item.text)
			} else {
				_, errMutation = tx.ExecContext(ctx, `DELETE FROM log_metadata_coverage WHERE source=? AND start_at=?`, item.key, item.text)
			}
		}
		if errMutation != nil {
			return dropped, errMutation
		}
		work++
		if removed {
			dropped++
		}
	}
	for _, source := range []logobs.Source{logobs.SourceSystem, logobs.SourceApplication} {
		frontier, present := frontiers[source]
		if !present {
			continue
		}
		previous, err := loadLogEvictionFrontier(ctx, tx, source)
		if err != nil {
			return dropped, err
		}
		if previous != nil && !frontier.After(*previous) {
			continue
		}
		key, err := logEvictionKey(source)
		if err != nil {
			return dropped, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO store_metadata(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, frontier.Format(logTimeLayout)); err != nil {
			return dropped, err
		}
	}
	return dropped, nil
}

func oldestRetentionCandidate(ctx context.Context, tx *sql.Tx, cutoff, logCutoff *time.Time) (retentionCandidate, bool, error) {
	var oldest retentionCandidate
	found := false
	queries := [...]string{
		retentionMetrics + `SELECT metric,(SELECT min(observed_at_ns) FROM samples WHERE samples.metric=metrics.metric) AS at,0 FROM metrics WHERE at IS NOT NULL ORDER BY at,metric LIMIT 1`,
		retentionMetrics + `SELECT metric,(SELECT min(bucket_start_ns) FROM rollups WHERE rollups.metric=metrics.metric) AS at,(SELECT min(resolution_seconds) FROM rollups WHERE rollups.metric=metrics.metric AND bucket_start_ns=(SELECT min(bucket_start_ns) FROM rollups WHERE rollups.metric=metrics.metric)) FROM metrics WHERE at IS NOT NULL ORDER BY at,metric LIMIT 1`,
		`SELECT source,minute,'' FROM (SELECT * FROM (SELECT source,minute FROM log_metadata_minutes WHERE source='system' ORDER BY minute LIMIT 1) UNION ALL SELECT * FROM (SELECT source,minute FROM log_metadata_minutes WHERE source='application' ORDER BY minute LIMIT 1)) ORDER BY minute,source LIMIT 1`,
		`SELECT source,start_at,end_at FROM (SELECT * FROM (SELECT source,start_at,end_at FROM log_metadata_coverage WHERE source='system' ORDER BY start_at LIMIT 1) UNION ALL SELECT * FROM (SELECT source,start_at,end_at FROM log_metadata_coverage WHERE source='application' ORDER BY start_at LIMIT 1)) ORDER BY start_at,source LIMIT 1`,
	}
	for kind, query := range queries {
		item := retentionCandidate{kind: kind}
		var err error
		if kind < 2 {
			err = tx.QueryRowContext(ctx, query).Scan(&item.key, &item.ns, &item.resolution)
			item.at = time.Unix(0, item.ns).UTC()
		} else {
			err = tx.QueryRowContext(ctx, query).Scan(&item.key, &item.text, &item.end)
			if err == nil {
				item.at, err = decodeLogTime(item.text)
			}
		}
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return oldest, false, err
		}
		boundary := cutoff
		if kind >= 2 {
			boundary = logCutoff
		}
		if boundary != nil && !item.at.Before(*boundary) {
			continue
		}
		if !found || item.at.Before(oldest.at) {
			oldest, found = item, true
		}
	}
	return oldest, found, nil
}

func trimPositiveLogProof(ctx context.Context, tx *sql.Tx, source string, frontier time.Time) (bool, bool, error) {
	// Frontier may be year 10000 for the final representable minute. No stored
	// timestamp exceeds year 9999; compare decoded time rather than text then.
	var startText, endText string
	err := tx.QueryRowContext(ctx, `SELECT start_at,end_at FROM log_metadata_coverage WHERE source=? AND reason='' ORDER BY start_at LIMIT 1`, source).Scan(&startText, &endText)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	start, err := decodeLogTime(startText)
	if err != nil {
		return false, false, err
	}
	end, err := decodeLogTime(endText)
	if err != nil {
		return false, false, err
	}
	if !start.Before(frontier) {
		return false, false, nil
	}
	if end.After(frontier) {
		_, err = tx.ExecContext(ctx, `UPDATE log_metadata_coverage SET start_at=? WHERE source=? AND start_at=?`, frontier.Format(logTimeLayout), source, startText)
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM log_metadata_coverage WHERE source=? AND start_at=?`, source, startText)
	}
	return err == nil, !end.After(frontier) && err == nil, err
}
