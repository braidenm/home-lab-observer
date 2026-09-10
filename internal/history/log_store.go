package history

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

var _ logobs.Store = (*Store)(nil)

type logState struct {
	checkpoint logobs.Checkpoint
	status     logobs.Status
}

func logPortError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, logobs.ErrRevisionConflict) {
		return logobs.ErrRevisionConflict
	}
	return ErrLogStorageUnavailable
}

func (s *Store) LoadCheckpoint(ctx context.Context, source logobs.Source) (logobs.Checkpoint, error) {
	if err := source.Validate(); err != nil {
		return logobs.Checkpoint{}, ErrLogStorageUnavailable
	}
	if err := s.ensureLogSchema(ctx); err != nil {
		return logobs.Checkpoint{}, logPortError(ctx, err)
	}
	state, err := loadLogState(ctx, s.db, source)
	if err != nil {
		return logobs.Checkpoint{}, logPortError(ctx, err)
	}
	return state.checkpoint.Clone(), nil
}

func loadLogState(ctx context.Context, reader logSQLReader, source logobs.Source) (logState, error) {
	var state logState
	var revision, attempted string
	var coverage, observed, statusCoverage, reason sql.NullString
	err := reader.QueryRowContext(ctx, `SELECT revision,reset_pending,opaque,attempted_at,coverage_through,support,collection,freshness,observed_at,status_coverage_through,reason FROM log_metadata_state WHERE source=?`, source).
		Scan(&revision, &state.checkpoint.ResetPending, &state.checkpoint.Opaque, &attempted, &coverage,
			&state.status.SupportState, &state.status.CollectionState, &state.status.Freshness, &observed, &statusCoverage, &reason)
	if errors.Is(err, sql.ErrNoRows) {
		return logState{}, nil
	}
	if err != nil {
		return logState{}, err
	}
	state.checkpoint.Revision, err = strconv.ParseUint(revision, 10, 64)
	if err != nil || state.checkpoint.Revision == 0 || strconv.FormatUint(state.checkpoint.Revision, 10) != revision {
		return logState{}, ErrLogStorageUnavailable
	}
	at, err := decodeLogTime(attempted)
	if err != nil {
		return logState{}, err
	}
	state.checkpoint.PreviousAttemptAt = &at
	state.status.AttemptedAt = timePtrCopy(at)
	state.checkpoint.CoverageThrough, err = scanNullableLogTime(coverage)
	if err != nil {
		return logState{}, err
	}
	state.status.ObservedAt, err = scanNullableLogTime(observed)
	if err != nil {
		return logState{}, err
	}
	state.status.CoverageThrough, err = scanNullableLogTime(statusCoverage)
	if err != nil {
		return logState{}, err
	}
	if reason.Valid {
		code := logobs.ReasonCode(reason.String)
		state.status.ReasonCode = &code
	}
	if state.checkpoint.Validate() != nil || state.status.Validate() != nil {
		return logState{}, ErrLogStorageUnavailable
	}
	return state, nil
}

func (s *Store) CommitBatch(ctx context.Context, batch logobs.Batch) error {
	if batch.Validate() != nil {
		return ErrLogStorageUnavailable
	}
	return logPortError(ctx, s.commitLogBatch(ctx, batch.Clone()))
}

func (s *Store) commitLogBatch(ctx context.Context, batch logobs.Batch) error {
	if err := s.ensureLogSchema(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	prior, err := loadLogState(ctx, tx, batch.Source)
	if err != nil {
		return err
	}
	if prior.checkpoint.Revision != batch.ExpectedRevision {
		return logobs.ErrRevisionConflict
	}
	if batch.ExpectedRevision == ^uint64(0) {
		return ErrLogStorageUnavailable
	}
	addition, err := deriveLogCoverage(prior.checkpoint, batch)
	if err != nil {
		return err
	}
	floor := batch.QueryStartedAt.Add(-min(s.config.RetentionAge, logRetention))
	if err := writeLogMinutes(ctx, tx, batch, floor); err != nil {
		return err
	}
	oldSegments, err := loadLogCoverage(ctx, tx, batch.Source, floor, batch.QueryStartedAt)
	if err != nil {
		return err
	}
	segments, err := mergeLogCoverage(oldSegments, addition, floor, batch.QueryStartedAt)
	if err != nil {
		return err
	}
	if len(segments) > maxLogCoverageRows {
		return ErrLogStorageUnavailable
	}
	if err := writeLogCoverage(ctx, tx, batch.Source, oldSegments, segments); err != nil {
		return err
	}
	next := prior.checkpoint.Clone()
	next.Revision++
	next.PreviousAttemptAt = timePtrCopy(batch.QueryStartedAt)
	switch batch.Kind {
	case logobs.BatchResetPending:
		next.ResetPending = true
		next.Opaque = nil
	case logobs.BatchResetEstablished:
		next.ResetPending = false
		next.Opaque = append([]byte(nil), batch.NextOpaque...)
	case logobs.BatchNormal:
		if len(batch.NextOpaque) > 0 {
			next.Opaque = append([]byte(nil), batch.NextOpaque...)
		}
		if batch.CaughtUp {
			next.CoverageThrough = timePtrCopy(batch.QueryStartedAt)
		}
	}
	status := logobs.StatusAfterBatch(batch, prior.status)
	if next.Validate() != nil || status.Validate() != nil {
		return ErrLogStorageUnavailable
	}
	if err := writeLogState(ctx, tx, batch.Source, batch.ExpectedRevision, logState{next, status}); err != nil {
		return err
	}
	// No response may overflow a JSON-safe aggregate, including a two-source query.
	// SQL integer overflow also aborts this transaction if externally corrupted data exists.
	var captured, discarded uint64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(captured),0),COALESCE(SUM(discarded),0) FROM log_metadata_minutes`).Scan(&captured, &discarded); err != nil {
		return err
	}
	if captured > logobs.MaxSafeInteger || discarded > logobs.MaxSafeInteger {
		return ErrLogStorageUnavailable
	}
	return tx.Commit()
}

func writeLogState(ctx context.Context, tx *sql.Tx, source logobs.Source, expected uint64, state logState) error {
	at, err := encodeLogTime(*state.checkpoint.PreviousAttemptAt)
	if err != nil {
		return err
	}
	coverage, err := nullableLogTime(state.checkpoint.CoverageThrough)
	if err != nil {
		return err
	}
	observed, err := nullableLogTime(state.status.ObservedAt)
	if err != nil {
		return err
	}
	statusCoverage, err := nullableLogTime(state.status.CoverageThrough)
	if err != nil {
		return err
	}
	var reason any
	if state.status.ReasonCode != nil {
		reason = string(*state.status.ReasonCode)
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO log_metadata_state(source,revision,reset_pending,opaque,attempted_at,coverage_through,support,collection,freshness,observed_at,status_coverage_through,reason) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(source) DO UPDATE SET revision=excluded.revision,reset_pending=excluded.reset_pending,opaque=excluded.opaque,attempted_at=excluded.attempted_at,coverage_through=excluded.coverage_through,support=excluded.support,collection=excluded.collection,freshness=excluded.freshness,observed_at=excluded.observed_at,status_coverage_through=excluded.status_coverage_through,reason=excluded.reason WHERE log_metadata_state.revision=?`,
		source, strconv.FormatUint(state.checkpoint.Revision, 10), state.checkpoint.ResetPending, state.checkpoint.Opaque, at, coverage,
		state.status.SupportState, state.status.CollectionState, state.status.Freshness, observed, statusCoverage, reason, strconv.FormatUint(expected, 10))
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return logobs.ErrRevisionConflict
	}
	return nil
}

func timePtrCopy(at time.Time) *time.Time { return &at }
