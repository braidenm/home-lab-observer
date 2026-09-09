package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type rawRow struct {
	metric MetricID
	at     int64
	value  float64
}
type rollupAccumulator struct {
	metric              MetricID
	bucket              int64
	count               int64
	min, max, sum, last float64
	lastAt              int64
}

func (s *Store) Maintain(ctx context.Context, now time.Time) error {
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	now = now.UTC()
	if _, err := s.rollupBatch(ctx, now.Add(-s.config.RollupAfter)); err != nil {
		s.recordFailure(ctx, "maintenance_failures", "ROLLUP_FAILED")
		return err
	}
	if _, err := s.pruneAge(ctx, now.Add(-s.config.RetentionAge)); err != nil {
		s.recordFailure(ctx, "maintenance_failures", "RETENTION_FAILED")
		return err
	}
	if err := s.incrementalVacuum(ctx); err != nil {
		s.recordFailure(ctx, "maintenance_failures", "VACUUM_FAILED")
		return err
	}
	if err := s.Checkpoint(ctx, true); err != nil {
		return err
	}
	s.refreshSize()
	if s.Health().DatabaseBytes > s.config.MaxBytes {
		removed, err := s.pruneSize(ctx)
		if err != nil {
			s.recordFailure(ctx, "maintenance_failures", "RETENTION_FAILED")
			return err
		}
		if removed > 0 {
			s.addHealthCounter("size_dropped", uint64(removed))
		}
		if err := s.incrementalVacuum(ctx); err != nil {
			s.recordFailure(ctx, "maintenance_failures", "VACUUM_FAILED")
			return err
		}
		if err := s.Checkpoint(ctx, true); err != nil {
			return err
		}
	}
	s.refreshSize()
	if s.Health().DatabaseBytes > s.config.MaxBytes {
		s.degrade("STORAGE_PRESSURE")
		return nil
	}
	s.recoverTransient("ROLLUP_FAILED", "RETENTION_FAILED", "VACUUM_FAILED", "CHECKPOINT_FAILED", "STORAGE_PRESSURE")
	return nil
}

func (s *Store) incrementalVacuum(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "PRAGMA incremental_vacuum("+strconv.Itoa(s.config.BatchSize)+")")
	return err
}

func (s *Store) rollupBatch(ctx context.Context, before time.Time) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT metric,observed_at_ns,value FROM samples WHERE observed_at_ns<? ORDER BY observed_at_ns,metric LIMIT ?`, before.UnixNano(), s.config.BatchSize)
	if err != nil {
		return 0, err
	}
	input := []rawRow{}
	for rows.Next() {
		var r rawRow
		if err := rows.Scan(&r.metric, &r.at, &r.value); err != nil {
			rows.Close()
			return 0, err
		}
		input = append(input, r)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(input) == 0 {
		return 0, nil
	}
	resolutionNS := s.config.RollupResolution.Nanoseconds()
	groups := map[string]*rollupAccumulator{}
	order := []string{}
	for _, row := range input {
		bucket := row.at - row.at%resolutionNS
		key := string(row.metric) + ":" + time.Unix(0, bucket).UTC().Format(time.RFC3339Nano)
		a := groups[key]
		if a == nil {
			a = &rollupAccumulator{metric: row.metric, bucket: bucket, min: row.value, max: row.value, last: row.value, lastAt: row.at}
			groups[key] = a
			order = append(order, key)
		}
		a.count++
		a.sum += row.value
		if row.value < a.min {
			a.min = row.value
		}
		if row.value > a.max {
			a.max = row.value
		}
		if row.at >= a.lastAt {
			a.last = row.value
			a.lastAt = row.at
		}
	}
	upsert, err := tx.PrepareContext(ctx, `INSERT INTO rollups(metric,bucket_start_ns,resolution_seconds,sample_count,minimum,maximum,total,last_value,last_at_ns) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(metric,bucket_start_ns,resolution_seconds) DO UPDATE SET sample_count=sample_count+excluded.sample_count,minimum=MIN(minimum,excluded.minimum),maximum=MAX(maximum,excluded.maximum),total=total+excluded.total,last_value=CASE WHEN excluded.last_at_ns>=last_at_ns THEN excluded.last_value ELSE last_value END,last_at_ns=MAX(last_at_ns,excluded.last_at_ns)`)
	if err != nil {
		return 0, err
	}
	defer upsert.Close()
	for _, key := range order {
		a := groups[key]
		if _, err := upsert.ExecContext(ctx, a.metric, a.bucket, int64(s.config.RollupResolution/time.Second), a.count, a.min, a.max, a.sum, a.last, a.lastAt); err != nil {
			return 0, err
		}
	}
	remove, err := tx.PrepareContext(ctx, `DELETE FROM samples WHERE metric=? AND observed_at_ns=?`)
	if err != nil {
		return 0, err
	}
	defer remove.Close()
	for _, row := range input {
		if _, err := remove.ExecContext(ctx, row.metric, row.at); err != nil {
			return 0, err
		}
	}
	if err := incrementCounter(ctx, tx, "rollup_input_rows", int64(len(input))); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	s.addHealthCounter("rollup_input_rows", uint64(len(input)))
	return int64(len(input)), nil
}

func (s *Store) pruneAge(ctx context.Context, cutoff time.Time) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	total := int64(0)
	result, err := tx.ExecContext(ctx, `DELETE FROM samples WHERE (metric,observed_at_ns) IN (SELECT metric,observed_at_ns FROM samples WHERE observed_at_ns<? ORDER BY observed_at_ns,metric LIMIT ?)`, cutoff.UnixNano(), s.config.BatchSize)
	if err != nil {
		return 0, err
	}
	count, _ := result.RowsAffected()
	total += count
	remaining := s.config.BatchSize - int(count)
	if remaining > 0 {
		result, err = tx.ExecContext(ctx, `DELETE FROM rollups WHERE (metric,bucket_start_ns,resolution_seconds) IN (SELECT metric,bucket_start_ns,resolution_seconds FROM rollups WHERE bucket_start_ns<? ORDER BY bucket_start_ns,metric LIMIT ?)`, cutoff.UnixNano(), remaining)
		if err != nil {
			return 0, err
		}
		count, _ = result.RowsAffected()
		total += count
	}
	if total > 0 {
		if err := incrementCounter(ctx, tx, "age_dropped", total); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if total > 0 {
		s.addHealthCounter("age_dropped", uint64(total))
	}
	return total, nil
}

func (s *Store) pruneSize(ctx context.Context) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	type candidate struct {
		kind       string
		metric     MetricID
		at         int64
		resolution int64
	}
	rows, err := tx.QueryContext(ctx, `SELECT kind,metric,at,resolution_seconds FROM (
		SELECT 'sample' AS kind,metric,observed_at_ns AS at,0 AS resolution_seconds FROM samples
		UNION ALL
		SELECT 'rollup' AS kind,metric,bucket_start_ns AS at,resolution_seconds FROM rollups
	) ORDER BY at,metric,kind,resolution_seconds LIMIT ?`, s.config.BatchSize)
	if err != nil {
		return 0, err
	}
	candidates := make([]candidate, 0, s.config.BatchSize)
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.kind, &item.metric, &item.at, &item.resolution); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	deleteSample, err := tx.PrepareContext(ctx, `DELETE FROM samples WHERE metric=? AND observed_at_ns=?`)
	if err != nil {
		return 0, err
	}
	defer deleteSample.Close()
	deleteRollup, err := tx.PrepareContext(ctx, `DELETE FROM rollups WHERE metric=? AND bucket_start_ns=? AND resolution_seconds=?`)
	if err != nil {
		return 0, err
	}
	defer deleteRollup.Close()
	var count int64
	for _, item := range candidates {
		var result sql.Result
		if item.kind == "sample" {
			result, err = deleteSample.ExecContext(ctx, item.metric, item.at)
		} else {
			result, err = deleteRollup.ExecContext(ctx, item.metric, item.at, item.resolution)
		}
		if err != nil {
			return 0, err
		}
		removed, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		count += removed
	}
	if count > 0 {
		if err := incrementCounter(ctx, tx, "size_dropped", count); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

var errCheckpointBusy = errors.New("database checkpoint is busy")

func (s *Store) Checkpoint(ctx context.Context, truncate bool) error {
	s.checkpointMu.Lock()
	defer s.checkpointMu.Unlock()
	mode := "PASSIVE"
	if truncate {
		mode = "TRUNCATE"
	}
	if err := s.runCheckpoint(ctx, mode); err != nil {
		s.recordCheckpointFailure(ctx)
		return err
	}
	// Persist success only after SQLite has accepted the checkpoint. A second
	// truncate clears the WAL frame created by the durable counter update.
	if err := s.persistCounter(ctx, "checkpoint_count", 1); err != nil {
		s.recordCheckpointFailure(ctx)
		return err
	}
	s.mu.Lock()
	s.health.CheckpointCount++
	s.mu.Unlock()
	if truncate {
		if err := s.runCheckpoint(ctx, mode); err != nil {
			s.recordCheckpointFailure(ctx)
			return err
		}
	}
	s.recoverTransient("CHECKPOINT_FAILED")
	return nil
}

func (s *Store) runCheckpoint(ctx context.Context, mode string) error {
	var busy, logFrames, checkpointed int
	if err := s.db.QueryRowContext(ctx, "PRAGMA wal_checkpoint("+mode+")").Scan(&busy, &logFrames, &checkpointed); err != nil {
		return err
	}
	if busy != 0 {
		return fmt.Errorf("%w: %d WAL frames remain", errCheckpointBusy, logFrames-checkpointed)
	}
	return nil
}

func (s *Store) recordCheckpointFailure(ctx context.Context) {
	s.mu.Lock()
	s.health.CheckpointFailures++
	s.mu.Unlock()
	_ = s.persistCounter(ctx, "checkpoint_failures", 1)
	s.degrade("CHECKPOINT_FAILED")
}

func incrementCounter(ctx context.Context, tx *sql.Tx, key string, amount int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO store_counters(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=value+excluded.value`, key, amount)
	return err
}
func (s *Store) persistCounter(ctx context.Context, key string, amount int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO store_counters(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=value+excluded.value`, key, amount)
	return err
}

func (s *Store) loadCounters(ctx context.Context) {
	rows, err := s.db.QueryContext(ctx, `SELECT key,value FROM store_counters`)
	if err != nil {
		return
	}
	defer rows.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	for rows.Next() {
		var key string
		var value uint64
		if rows.Scan(&key, &value) != nil {
			continue
		}
		switch key {
		case "age_dropped":
			s.health.AgeDropped = value
		case "size_dropped":
			s.health.SizeDropped = value
		case "rollup_input_rows":
			s.health.RollupInputRows = value
		case "checkpoint_count":
			s.health.CheckpointCount = value
		case "checkpoint_failures":
			s.health.CheckpointFailures = value
		case "write_failures":
			s.health.WriteFailures = value
		case "sequence_failures":
			s.health.SequenceFailures = value
		case "maintenance_failures":
			s.health.MaintenanceFailures = value
		}
	}
}
func (s *Store) addHealthCounter(key string, value uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch key {
	case "age_dropped":
		s.health.AgeDropped += value
	case "size_dropped":
		s.health.SizeDropped += value
	case "rollup_input_rows":
		s.health.RollupInputRows += value
	}
}
func (s *Store) refreshSize() {
	size := int64(0)
	for _, path := range []string{s.path, s.path + "-wal", s.path + "-shm"} {
		if info, err := os.Stat(path); err == nil {
			size += info.Size()
		}
	}
	s.mu.Lock()
	s.health.DatabaseBytes = size
	s.mu.Unlock()
}
func (s *Store) degrade(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.health.State = "DEGRADED"
	if s.recoveryReason != "" {
		s.health.ReasonCode = s.recoveryReason
	} else {
		s.health.ReasonCode = reason
	}
}

func (s *Store) recoverTransient(reasons ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recoveryReason != "" || s.health.State != "DEGRADED" {
		return
	}
	for _, reason := range reasons {
		if s.health.ReasonCode == reason {
			s.health.State = "AVAILABLE"
			s.health.ReasonCode = ""
			return
		}
	}
}

func (s *Store) recordFailure(ctx context.Context, key, reason string) {
	s.mu.Lock()
	s.health.State = "DEGRADED"
	if s.recoveryReason != "" {
		s.health.ReasonCode = s.recoveryReason
	} else {
		s.health.ReasonCode = reason
	}
	switch key {
	case "write_failures":
		s.health.WriteFailures++
	case "sequence_failures":
		s.health.SequenceFailures++
	case "maintenance_failures":
		s.health.MaintenanceFailures++
	}
	s.mu.Unlock()
	_ = s.persistCounter(ctx, key, 1)
}
