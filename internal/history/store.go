package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"modernc.org/sqlite"
)

const (
	defaultMaxBytes    = int64(250 * 1024 * 1024)
	defaultBatchSize   = 512
	defaultRollupAfter = 6 * time.Hour
	defaultResolution  = 5 * time.Minute
)

type Clock interface{ Now() time.Time }
type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type Config struct {
	Path             string
	RetentionAge     time.Duration
	MaxBytes         int64
	BatchSize        int
	RollupAfter      time.Duration
	RollupResolution time.Duration
}

func DefaultConfig(path string) Config {
	return Config{Path: path, RetentionAge: 7 * 24 * time.Hour, MaxBytes: defaultMaxBytes, BatchSize: defaultBatchSize, RollupAfter: defaultRollupAfter, RollupResolution: defaultResolution}
}

type Store struct {
	db             *sql.DB
	path           string
	clock          Clock
	config         Config
	insertSample   *sql.Stmt
	nextSequence   *sql.Stmt
	mu             sync.RWMutex
	maintenanceMu  sync.Mutex
	checkpointMu   sync.Mutex
	lock           *fileLock
	health         Health
	recoveryReason string
}

func Open(ctx context.Context, config Config, clock Clock) (*Store, error) {
	if strings.TrimSpace(config.Path) == "" {
		return nil, errors.New("history path is required")
	}
	config = normalizeConfig(config)
	if clock == nil {
		clock = realClock{}
	}
	if err := os.MkdirAll(filepath.Dir(config.Path), 0o700); err != nil {
		return nil, fmt.Errorf("create history directory: %w", err)
	}
	lock, err := acquireFileLock(config.Path + ".lock")
	if err != nil {
		return nil, fmt.Errorf("acquire history ownership: %w", err)
	}
	keepLock := false
	defer func() {
		if !keepLock {
			_ = lock.Close()
		}
	}()
	existed := fileExists(config.Path)
	recoveryReason, quarantinePath := "", ""
	db, err := openDatabase(config.Path)
	if err != nil {
		return nil, err
	}
	if existed {
		version, inspectErr := inspectDatabase(ctx, db)
		if inspectErr != nil && !isCorruptionError(inspectErr) {
			_ = db.Close()
			return nil, fmt.Errorf("inspect history database: %w", inspectErr)
		}
		if inspectErr != nil || version > SchemaVersion {
			_ = db.Close()
			recoveryReason = "DATABASE_CORRUPT"
			if version > SchemaVersion {
				recoveryReason = "DATABASE_VERSION_INCOMPATIBLE"
			}
			quarantinePath, err = quarantine(config.Path, clock.Now())
			if err != nil {
				return nil, fmt.Errorf("quarantine history database: %w", err)
			}
			db, err = openDatabase(config.Path)
			if err != nil {
				return nil, err
			}
		}
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate history database: %w", err)
	}
	if err := configureDatabase(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure history database: %w", err)
	}
	engineVersion, err := sqliteVersion(ctx, db)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("read sqlite version: %w", err)
	}
	if versionLess(engineVersion, MinimumSQLiteVersion) {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite %s is older than required %s", engineVersion, MinimumSQLiteVersion)
	}
	if recoveryReason != "" {
		if err := persistRecovery(ctx, db, recoveryReason, quarantinePath); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("persist recovery status: %w", err)
		}
	} else {
		recoveryReason, quarantinePath = loadRecovery(ctx, db)
	}
	insertSample, err := db.PrepareContext(ctx, `INSERT INTO samples(metric, observed_at_ns, value) VALUES(?, ?, ?) ON CONFLICT(metric, observed_at_ns) DO UPDATE SET value=excluded.value`)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	nextSequence, err := db.PrepareContext(ctx, `UPDATE runtime_state SET value=value+1 WHERE key='sequence' RETURNING value`)
	if err != nil {
		_ = insertSample.Close()
		_ = db.Close()
		return nil, err
	}
	store := &Store{db: db, path: config.Path, clock: clock, config: config, insertSample: insertSample, nextSequence: nextSequence, lock: lock, health: Health{State: "AVAILABLE", SQLiteVersion: engineVersion}, recoveryReason: recoveryReason}
	if recoveryReason != "" {
		store.health.State, store.health.ReasonCode, store.health.QuarantinePath = "DEGRADED", recoveryReason, quarantinePath
	}
	store.loadCounters(ctx)
	store.refreshSize()
	keepLock = true
	return store, nil
}

func normalizeConfig(config Config) Config {
	if config.RetentionAge <= 0 {
		config.RetentionAge = 7 * 24 * time.Hour
	}
	if config.MaxBytes <= 0 {
		config.MaxBytes = defaultMaxBytes
	}
	if config.BatchSize <= 0 || config.BatchSize > 10000 {
		config.BatchSize = defaultBatchSize
	}
	if config.RollupAfter <= 0 {
		config.RollupAfter = defaultRollupAfter
	}
	if config.RollupResolution <= 0 {
		config.RollupResolution = defaultResolution
	}
	return config
}

func openDatabase(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open history database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db, nil
}

func configureDatabase(ctx context.Context, db *sql.DB) error {
	for _, statement := range []string{"PRAGMA journal_mode=WAL", "PRAGMA synchronous=NORMAL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON"} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

var errIntegrity = errors.New("database integrity check failed")

func inspectDatabase(ctx context.Context, db *sql.DB) (int, error) {
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result); err != nil {
		return 0, err
	}
	if result != "ok" {
		return 0, errIntegrity
	}
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func isCorruptionError(err error) bool {
	if errors.Is(err, errIntegrity) {
		return true
	}
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	primary := sqliteErr.Code() & 0xff
	return primary == 11 || primary == 26
}

func sqliteVersion(ctx context.Context, db *sql.DB) (string, error) {
	var version string
	err := db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&version)
	return version, err
}
func versionLess(actual, minimum string) bool {
	parse := func(value string) [3]int {
		var result [3]int
		_, _ = fmt.Sscanf(value, "%d.%d.%d", &result[0], &result[1], &result[2])
		return result
	}
	a, m := parse(actual), parse(minimum)
	for index := 0; index < 3; index++ {
		if a[index] != m[index] {
			return a[index] < m[index]
		}
	}
	return false
}

func migrate(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version == SchemaVersion {
		return nil
	}
	if version > SchemaVersion {
		return errors.New("database schema is newer than runtime")
	}
	if version == 0 {
		if _, err := db.ExecContext(ctx, "PRAGMA auto_vacuum=INCREMENTAL"); err != nil {
			return err
		}
	} else if version == 1 {
		if _, err := db.ExecContext(ctx, "PRAGMA auto_vacuum=INCREMENTAL"); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, "VACUUM"); err != nil {
			return err
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS samples(metric TEXT NOT NULL CHECK(metric IN ('cpu.utilization.percent','memory.utilization.percent','filesystem.aggregate.utilization.percent','network.receive.bytes_per_second','network.transmit.bytes_per_second','process.count')), observed_at_ns INTEGER NOT NULL, value REAL NOT NULL, PRIMARY KEY(metric, observed_at_ns)) WITHOUT ROWID`,
		`CREATE TABLE IF NOT EXISTS rollups(metric TEXT NOT NULL CHECK(metric IN ('cpu.utilization.percent','memory.utilization.percent','filesystem.aggregate.utilization.percent','network.receive.bytes_per_second','network.transmit.bytes_per_second','process.count')), bucket_start_ns INTEGER NOT NULL, resolution_seconds INTEGER NOT NULL, sample_count INTEGER NOT NULL, minimum REAL NOT NULL, maximum REAL NOT NULL, total REAL NOT NULL, last_value REAL NOT NULL, last_at_ns INTEGER NOT NULL, PRIMARY KEY(metric, bucket_start_ns, resolution_seconds)) WITHOUT ROWID`,
		`CREATE TABLE IF NOT EXISTS runtime_state(key TEXT PRIMARY KEY, value INTEGER NOT NULL) WITHOUT ROWID`,
		`INSERT OR IGNORE INTO runtime_state(key,value) VALUES('sequence',0)`,
		`CREATE TABLE IF NOT EXISTS store_counters(key TEXT PRIMARY KEY, value INTEGER NOT NULL) WITHOUT ROWID`,
		`CREATE TABLE IF NOT EXISTS store_metadata(key TEXT PRIMARY KEY, value TEXT NOT NULL) WITHOUT ROWID`,
		`PRAGMA user_version=2`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func persistRecovery(ctx context.Context, db *sql.DB, reason, path string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for key, value := range map[string]string{"recovery_reason": reason, "quarantine_path": path} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO store_metadata(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func loadRecovery(ctx context.Context, db *sql.DB) (string, string) {
	values := map[string]string{}
	rows, err := db.QueryContext(ctx, `SELECT key,value FROM store_metadata WHERE key IN ('recovery_reason','quarantine_path')`)
	if err != nil {
		return "", ""
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if rows.Scan(&key, &value) == nil {
			values[key] = value
		}
	}
	return values["recovery_reason"], values["quarantine_path"]
}

func (s *Store) NextSequence(ctx context.Context) (valueOut uint64, returnErr error) {
	defer func() {
		if returnErr != nil {
			s.recordFailure(ctx, "sequence_failures", "SEQUENCE_FAILED")
		}
	}()
	var value int64
	if err := s.nextSequence.QueryRowContext(ctx).Scan(&value); err != nil {
		return 0, err
	}
	if value < 0 {
		return 0, errors.New("invalid durable sequence")
	}
	return uint64(value), nil
}

func (s *Store) WriteSamples(ctx context.Context, samples []Sample) (returnErr error) {
	if len(samples) == 0 {
		return nil
	}
	if len(samples) > len(allowedMetrics) {
		return errors.New("too many metric samples")
	}
	for _, sample := range samples {
		if err := sample.validate(); err != nil {
			return err
		}
	}
	defer func() {
		if returnErr != nil {
			s.recordFailure(ctx, "write_failures", "WRITE_FAILED")
		}
	}()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statement := tx.StmtContext(ctx, s.insertSample)
	for _, sample := range samples {
		if _, err := statement.ExecContext(ctx, sample.Metric, sample.At.UTC().UnixNano(), sample.Value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Samples(ctx context.Context, metric MetricID, from, to time.Time, limit int) ([]Sample, error) {
	if _, ok := allowedMetrics[metric]; !ok {
		return nil, ErrMetricNotAllowed
	}
	if limit < 1 || limit > 10000 {
		return nil, errors.New("invalid sample limit")
	}
	return querySamples(ctx, s.db, metric, from, to, limit)
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func querySamples(ctx context.Context, query queryer, metric MetricID, from, to time.Time, limit int) ([]Sample, error) {
	rows, err := query.QueryContext(ctx, `SELECT observed_at_ns,value FROM samples WHERE metric=? AND observed_at_ns>=? AND observed_at_ns<=? ORDER BY observed_at_ns LIMIT ?`, metric, from.UTC().UnixNano(), to.UTC().UnixNano(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Sample{}
	for rows.Next() {
		var at int64
		var value float64
		if err := rows.Scan(&at, &value); err != nil {
			return nil, err
		}
		result = append(result, Sample{Metric: metric, At: time.Unix(0, at).UTC(), Value: value})
	}
	return result, rows.Err()
}

func (s *Store) Rollups(ctx context.Context, metric MetricID, from, to time.Time, limit int) ([]Rollup, error) {
	if _, ok := allowedMetrics[metric]; !ok {
		return nil, ErrMetricNotAllowed
	}
	if limit < 1 || limit > 10000 {
		return nil, errors.New("invalid rollup limit")
	}
	return queryRollups(ctx, s.db, metric, from, to, limit)
}

func queryRollups(ctx context.Context, query queryer, metric MetricID, from, to time.Time, limit int) ([]Rollup, error) {
	rows, err := query.QueryContext(ctx, `SELECT bucket_start_ns,resolution_seconds,sample_count,minimum,maximum,total,last_value,last_at_ns FROM rollups WHERE metric=? AND bucket_start_ns>=? AND bucket_start_ns<=? ORDER BY bucket_start_ns LIMIT ?`, metric, from.UTC().UnixNano(), to.UTC().UnixNano(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Rollup{}
	for rows.Next() {
		var r Rollup
		var at, lastAt int64
		r.Metric = metric
		if err := rows.Scan(&at, &r.ResolutionSeconds, &r.Count, &r.Minimum, &r.Maximum, &r.Sum, &r.Last, &lastAt); err != nil {
			return nil, err
		}
		r.BucketStart = time.Unix(0, at).UTC()
		r.LastAt = time.Unix(0, lastAt).UTC()
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) ReadMetric(ctx context.Context, metric MetricID, from, to time.Time, limit int) ([]Sample, []Rollup, error) {
	if _, ok := allowedMetrics[metric]; !ok {
		return nil, nil, ErrMetricNotAllowed
	}
	if limit < 1 || limit > 10000 {
		return nil, nil, errors.New("invalid metric limit")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	samples, err := querySamples(ctx, tx, metric, from, to, limit)
	if err != nil {
		return nil, nil, err
	}
	rollups, err := queryRollups(ctx, tx, metric, from, to, limit)
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return samples, rollups, nil
}

func (s *Store) Health() Health { s.mu.RLock(); defer s.mu.RUnlock(); return s.health }

func (s *Store) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.Checkpoint(ctx, true)
	err1 := s.insertSample.Close()
	err2 := s.nextSequence.Close()
	err3 := s.db.Close()
	err4 := s.lock.Close()
	return errors.Join(err1, err2, err3, err4)
}

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }
func quarantine(path string, at time.Time) (string, error) {
	base := path + ".quarantine-" + at.UTC().Format("20060102T150405.000000000Z")
	destination := base
	for attempt := 0; fileExists(destination) || fileExists(destination+"-wal") || fileExists(destination+"-shm"); attempt++ {
		if attempt >= 999 {
			return "", errors.New("no quarantine name available")
		}
		destination = fmt.Sprintf("%s-%03d", base, attempt+1)
	}
	moved := [][2]string{}
	move := func(source, target string) error {
		if err := os.Rename(source, target); err != nil {
			return err
		}
		moved = append(moved, [2]string{source, target})
		return nil
	}
	if err := move(path, destination); err != nil {
		return "", err
	}
	for _, sidecar := range []string{"-wal", "-shm"} {
		source := path + sidecar
		if _, err := os.Stat(source); err == nil {
			if err := move(source, destination+sidecar); err != nil {
				for index := len(moved) - 1; index >= 0; index-- {
					_ = os.Rename(moved[index][1], moved[index][0])
				}
				return "", err
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	return destination, nil
}
