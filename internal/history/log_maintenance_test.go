package history

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

func maintenanceStore(t *testing.T, budget int) *Store {
	t.Helper()
	c := DefaultConfig(filepath.Join(t.TempDir(), "history.db"))
	c.BatchSize = budget
	s := openTestStore(t, c)
	if err := s.ensureLogSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func insertMaintenanceMinute(t *testing.T, s *Store, source string, at time.Time) {
	t.Helper()
	_, err := s.db.Exec(`INSERT INTO log_metadata_minutes VALUES(?,?,1,0,0,0,0,0,1,0,0)`, source, at.Format(logTimeLayout))
	if err != nil {
		t.Fatal(err)
	}
}

func insertMaintenanceProof(t *testing.T, s *Store, source string, start, end time.Time) {
	t.Helper()
	_, err := s.db.Exec(`INSERT INTO log_metadata_coverage VALUES(?,?,?,'')`, source, start.Format(logTimeLayout), end.Format(logTimeLayout))
	if err != nil {
		t.Fatal(err)
	}
}

func maintenanceCount(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestLogMaintenanceSizeOrderingAcrossFullTimeRange(t *testing.T) {
	s := maintenanceStore(t, 1)
	ancient := time.Date(1, 1, 2, 0, 0, 0, 0, time.UTC)
	future := time.Date(9999, 12, 31, 23, 59, 0, 0, time.UTC)
	insertMaintenanceMinute(t, s, "system", ancient)
	insertMaintenanceMinute(t, s, "application", future)
	if err := s.WriteSamples(context.Background(), []Sample{{Metric: CPUUtilization, At: logTestNow, Value: 1}}); err != nil {
		t.Fatal(err)
	}
	for i, expected := range []struct{ samples, minutes int }{{1, 1}, {0, 1}, {0, 0}} {
		n, err := s.pruneSize(context.Background())
		if err != nil || n != 1 {
			t.Fatalf("pass %d: %d %v", i, n, err)
		}
		if maintenanceCount(t, s, "samples") != expected.samples || maintenanceCount(t, s, "log_metadata_minutes") != expected.minutes {
			t.Fatalf("wrong oldest row pass %d", i)
		}
	}
}

func TestLogMaintenanceBudgetOneInvalidatesProofBeforeCounts(t *testing.T) {
	s := maintenanceStore(t, 1)
	minute := logTestNow.Truncate(time.Minute)
	insertMaintenanceMinute(t, s, "system", minute)
	// Partial positive proof starts after the count row's key, exercising the
	// ancillary invalidation rather than simply choosing older proof first.
	insertMaintenanceProof(t, s, "system", minute.Add(time.Second), minute.Add(2*time.Minute))
	if n, err := s.pruneSize(context.Background()); err != nil || n != 0 {
		t.Fatalf("%d %v", n, err)
	}
	if maintenanceCount(t, s, "log_metadata_minutes") != 1 {
		t.Fatal("count removed outside budget")
	}
	var start string
	if err := s.db.QueryRow(`SELECT start_at FROM log_metadata_coverage`).Scan(&start); err != nil || start != minute.Add(time.Minute).Format(logTimeLayout) {
		t.Fatalf("proof %s %v", start, err)
	}
	if n, err := s.pruneSize(context.Background()); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	if maintenanceCount(t, s, "log_metadata_minutes") != 0 {
		t.Fatal("count did not progress")
	}
}

func TestLogMaintenanceAgeGlobalBudgetAndSevenDayCap(t *testing.T) {
	s := maintenanceStore(t, 2)
	s.config.RetentionAge = 30 * 24 * time.Hour
	oldLog := logTestNow.Add(-8 * 24 * time.Hour).Truncate(time.Minute)
	insertMaintenanceMinute(t, s, "system", oldLog)
	insertMaintenanceProof(t, s, "application", oldLog, logTestNow)
	if err := s.WriteSamples(context.Background(), []Sample{{Metric: CPUUtilization, At: oldLog, Value: 1}}); err != nil {
		t.Fatal(err)
	}
	n, err := s.pruneAge(context.Background(), logTestNow.Add(-s.config.RetentionAge))
	if err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	if maintenanceCount(t, s, "samples") != 1 || maintenanceCount(t, s, "log_metadata_minutes") != 0 {
		t.Fatal("log cutoff used host retention")
	}
	var start string
	if err := s.db.QueryRow(`SELECT start_at FROM log_metadata_coverage`).Scan(&start); err != nil || start != logTestNow.Add(-logRetention).Format(logTimeLayout) {
		t.Fatalf("coverage cutoff %s %v", start, err)
	}
}

func TestLogMaintenanceDoesNotStarveOlderLogsBehindHostBatch(t *testing.T) {
	s := maintenanceStore(t, 512)
	cutoff := logTestNow.Add(-logRetention)
	insertMaintenanceMinute(t, s, "system", cutoff.Add(-2*time.Hour).Truncate(time.Minute))
	for i := 0; i < 512; i++ {
		if err := s.WriteSamples(context.Background(), []Sample{{Metric: CPUUtilization, At: cutoff.Add(-time.Hour + time.Duration(i)*time.Second), Value: 1}}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.pruneAge(context.Background(), cutoff)
	if err != nil || n != 512 {
		t.Fatalf("%d %v", n, err)
	}
	if maintenanceCount(t, s, "log_metadata_minutes") != 0 || maintenanceCount(t, s, "samples") != 1 {
		t.Fatal("host rows starved older log row")
	}
}

func TestLogMaintenanceAbsentOrIncompatibleSchemaLeavesCoreOperational(t *testing.T) {
	for _, version := range []string{"", "999"} {
		t.Run("version"+version, func(t *testing.T) {
			c := DefaultConfig(filepath.Join(t.TempDir(), "history.db"))
			c.BatchSize = 1
			s := openTestStore(t, c)
			if version != "" {
				if _, err := s.db.Exec(`INSERT INTO store_metadata VALUES('log_metadata_schema_version',?)`, version); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.WriteSamples(context.Background(), []Sample{{Metric: CPUUtilization, At: logTestNow, Value: 1}}); err != nil {
				t.Fatal(err)
			}
			if n, err := s.pruneSize(context.Background()); err != nil || n != 1 {
				t.Fatalf("%d %v", n, err)
			}
			var tables int
			if err := s.db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name LIKE 'log_metadata_%'`).Scan(&tables); err != nil || tables != 0 {
				t.Fatalf("created tables %d %v", tables, err)
			}
			if s.Health().QuarantinePath != "" {
				t.Fatal("quarantined core")
			}
		})
	}
}

func TestLogMaintenancePreservesCheckpointAndBoundsPressure(t *testing.T) {
	s := maintenanceStore(t, 2)
	ctx := context.Background()
	b := logSuccess(logTestNow)
	b.NextOpaque = []byte("synthetic-retained-cursor")
	if err := s.CommitBatch(ctx, b); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		insertMaintenanceMinute(t, s, "system", logTestNow.Add(-time.Duration(i)*time.Minute).Truncate(time.Minute))
	}
	s.config.MaxBytes = 1
	if err := s.Maintain(ctx, logTestNow); err != nil {
		t.Fatal(err)
	}
	if s.Health().SizeDropped != 2 || s.Health().ReasonCode != "STORAGE_PRESSURE" {
		t.Fatalf("health %+v", s.Health())
	}
	cp, err := s.LoadCheckpoint(ctx, b.Source)
	if err != nil || cp.Revision != 1 || string(cp.Opaque) != "synthetic-retained-cursor" {
		t.Fatalf("checkpoint %+v %v", cp, err)
	}
}

func TestLogMaintenanceMutationFailureRollsBackProofAndCounts(t *testing.T) {
	s := maintenanceStore(t, 2)
	minute := logTestNow.Truncate(time.Minute)
	insertMaintenanceMinute(t, s, "system", minute)
	insertMaintenanceProof(t, s, "system", minute.Add(time.Second), minute.Add(2*time.Minute))
	if _, err := s.db.Exec(`CREATE TRIGGER reject_retention BEFORE DELETE ON log_metadata_minutes BEGIN SELECT RAISE(ABORT,'synthetic-retention-failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pruneSize(context.Background()); err == nil {
		t.Fatal("accepted failed deletion")
	}
	var start string
	if err := s.db.QueryRow(`SELECT start_at FROM log_metadata_coverage`).Scan(&start); err != nil || start != minute.Add(time.Second).Format(logTimeLayout) {
		t.Fatalf("partial proof mutation escaped rollback: %s %v", start, err)
	}
	if maintenanceCount(t, s, "log_metadata_minutes") != 1 {
		t.Fatal("count escaped rollback")
	}
}

func TestLogMaintenanceOrderIncludesHostRollupsAndCoverage(t *testing.T) {
	s := maintenanceStore(t, 1)
	old := logTestNow.Add(-time.Hour)
	insertMaintenanceProof(t, s, "system", old.Add(-time.Hour), old)
	if _, err := s.db.Exec(`INSERT INTO rollups VALUES(?,?,60,1,1,1,1,1,?)`, CPUUtilization, old.UnixNano(), old.UnixNano()); err != nil {
		t.Fatal(err)
	}
	insertMaintenanceMinute(t, s, "application", logTestNow.Truncate(time.Minute))
	for i, table := range []string{"log_metadata_coverage", "rollups", "log_metadata_minutes"} {
		if n, err := s.pruneSize(context.Background()); err != nil || n != 1 {
			t.Fatalf("pass %d: %d %v", i, n, err)
		}
		if maintenanceCount(t, s, table) != 0 {
			t.Fatalf("wrong global order: %s", table)
		}
	}
}

func TestLogMaintenanceAgeFixedFractionalCutoffAndShortRetention(t *testing.T) {
	s := maintenanceStore(t, 2)
	s.config.RetentionAge = time.Hour
	cutoff := logTestNow.Add(-time.Hour + 123*time.Nanosecond)
	insertMaintenanceProof(t, s, "system", cutoff.Add(-time.Second), cutoff.Add(time.Second))
	insertMaintenanceMinute(t, s, "application", cutoff.Truncate(time.Minute))
	if n, err := s.pruneAge(context.Background(), cutoff); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	var start string
	if err := s.db.QueryRow(`SELECT start_at FROM log_metadata_coverage`).Scan(&start); err != nil || start != cutoff.Format(logTimeLayout) {
		t.Fatalf("lost fractional cutoff: %s %v", start, err)
	}
	if n, err := s.pruneAge(context.Background(), cutoff); err != nil || n != 0 {
		t.Fatalf("cutoff equality not stable: %d %v", n, err)
	}
}

func TestLogMaintenanceIncompatibleExistingTablesUntouched(t *testing.T) {
	s := maintenanceStore(t, 1)
	insertMaintenanceMinute(t, s, "system", logTestNow.Add(-30*24*time.Hour).Truncate(time.Minute))
	if _, err := s.db.Exec(`UPDATE store_metadata SET value='999' WHERE key='log_metadata_schema_version'`); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteSamples(context.Background(), []Sample{{Metric: CPUUtilization, At: logTestNow, Value: 1}}); err != nil {
		t.Fatal(err)
	}
	if n, err := s.pruneSize(context.Background()); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	if maintenanceCount(t, s, "log_metadata_minutes") != 1 || maintenanceCount(t, s, "samples") != 0 {
		t.Fatal("incompatible optional data modified")
	}
}

func TestLogMaintenanceEvictionFrontierMonotonicAndPersistent(t *testing.T) {
	s := maintenanceStore(t, 1)
	minute := logTestNow.Truncate(time.Minute)
	insertMaintenanceMinute(t, s, "system", minute)
	if _, err := s.pruneSize(context.Background()); err != nil {
		t.Fatal(err)
	}
	var frontier string
	if err := s.db.QueryRow(`SELECT value FROM store_metadata WHERE key='log_metadata_evicted_before_system'`).Scan(&frontier); err != nil || frontier != minute.Add(time.Minute).Format(logTimeLayout) {
		t.Fatalf("frontier %s %v", frontier, err)
	}
	insertMaintenanceMinute(t, s, "system", minute.Add(-time.Hour))
	if _, err := s.pruneSize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT value FROM store_metadata WHERE key='log_metadata_evicted_before_system'`).Scan(&frontier); err != nil || frontier != minute.Add(time.Minute).Format(logTimeLayout) {
		t.Fatalf("regressed frontier %s %v", frontier, err)
	}
	path := s.path
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), DefaultConfig(path), fixedClock{logTestNow})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.db.QueryRow(`SELECT value FROM store_metadata WHERE key='log_metadata_evicted_before_system'`).Scan(&frontier); err != nil || frontier != minute.Add(time.Minute).Format(logTimeLayout) {
		t.Fatalf("lost frontier %s %v", frontier, err)
	}
}

func TestLogMaintenanceFrontierFailureRollsBackDeletion(t *testing.T) {
	s := maintenanceStore(t, 1)
	insertMaintenanceMinute(t, s, "system", logTestNow.Truncate(time.Minute))
	if _, err := s.db.Exec(`CREATE TRIGGER reject_frontier BEFORE INSERT ON store_metadata WHEN NEW.key LIKE 'log_metadata_evicted_before_%' BEGIN SELECT RAISE(ABORT,'synthetic-frontier-failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pruneSize(context.Background()); err == nil {
		t.Fatal("accepted frontier failure")
	}
	if maintenanceCount(t, s, "log_metadata_minutes") != 1 {
		t.Fatal("deleted count without frontier")
	}
}

func TestLogMaintenanceFinalMinuteFrontierClamped(t *testing.T) {
	s := maintenanceStore(t, 1)
	insertMaintenanceMinute(t, s, "application", time.Date(9999, 12, 31, 23, 59, 0, 0, time.UTC))
	if _, err := s.pruneSize(context.Background()); err != nil {
		t.Fatal(err)
	}
	var frontier string
	if err := s.db.QueryRow(`SELECT value FROM store_metadata WHERE key='log_metadata_evicted_before_application'`).Scan(&frontier); err != nil || frontier != "9999-12-31T23:59:59.999999999Z" {
		t.Fatalf("frontier %s %v", frontier, err)
	}
}

func TestLogMaintenanceFutureSkewEvictionCannotReproveFullZero(t *testing.T) {
	s := maintenanceStore(t, 512)
	q := logTestNow.Truncate(time.Minute).Add(59 * time.Second)
	b := logSuccess(q)
	b.NextOpaque = []byte("synthetic-skew-cursor")
	b.Events = []logobs.Event{{Source: b.Source, ObservedAt: q.Add(time.Second), Severity: logobs.SeverityError, EventCode: "SYSTEMD_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
	b.ExaminedCount = 1
	ctx := context.Background()
	if err := s.CommitBatch(ctx, b); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pruneSize(ctx); err != nil {
		t.Fatal(err)
	}
	next := logSuccess(q.Add(61 * time.Second))
	next.ExpectedRevision = 1
	if err := s.CommitBatch(ctx, next); err != nil {
		t.Fatal(err)
	}
	start := q.Add(time.Second)
	query := logobs.SummaryQuery{WindowStart: start, WindowEnd: start.Add(time.Hour), BucketInterval: time.Minute, BucketCount: 60}
	summary, err := s.QuerySummary(ctx, []logobs.Source{b.Source}, query)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Sources[0].Buckets[0].CoveredSeconds != 0 || summary.Sources[0].Buckets[0].Counts != nil {
		t.Fatalf("reconstructed false zero: %+v", summary.Sources[0].Buckets[0])
	}
}

func TestLogMaintenanceMixedPressureReclaimsCombinedAllocation(t *testing.T) {
	s := maintenanceStore(t, 512)
	ctx := context.Background()
	if err := s.Checkpoint(ctx, true); err != nil {
		t.Fatal(err)
	}
	s.refreshSize()
	target := s.Health().DatabaseBytes + 32*1024
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := 0; i < 5000; i++ {
		at := logTestNow.Add(-time.Duration(i) * time.Minute)
		if _, err := tx.Exec(`INSERT INTO log_metadata_minutes VALUES('system',?,1,0,0,0,0,0,1,0,0)`, at.Format(logTimeLayout)); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO samples VALUES(?,?,1)`, CPUUtilization, at.UnixNano()); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	s.refreshSize()
	if s.Health().DatabaseBytes <= target {
		t.Fatal("fixture did not create pressure")
	}
	s.config.MaxBytes = target
	s.config.RollupAfter = logRetention
	for pass := 0; pass < 24 && s.Health().DatabaseBytes > target; pass++ {
		before := s.Health().SizeDropped
		if err := s.Maintain(ctx, logTestNow); err != nil {
			t.Fatal(err)
		}
		if s.Health().SizeDropped-before > 512 {
			t.Fatal("shared pressure batch exceeded")
		}
	}
	if s.Health().DatabaseBytes > target {
		t.Fatalf("failed to reclaim combined allocation: %+v", s.Health())
	}
	if s.Health().SizeDropped == 0 {
		t.Fatal("pressure was not observable")
	}
}

func TestLogMaintenanceCandidateCatalogCoversEveryHostMetric(t *testing.T) {
	s := maintenanceStore(t, 512)
	for metric := range allowedMetrics {
		if err := s.WriteSamples(context.Background(), []Sample{{Metric: metric, At: logTestNow, Value: 1}}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := s.pruneSize(context.Background()); err != nil || n != int64(len(allowedMetrics)) {
		t.Fatalf("unprunable metric: %d %v", n, err)
	}
}
