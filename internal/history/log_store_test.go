package history

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

func TestLogSchemaIsLazyAdditiveAndCheckpointSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "history.db")
	s := openTestStore(t, DefaultConfig(path))
	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name LIKE 'log_metadata_%'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("eager schema %d %v", count, err)
	}
	cp, err := s.LoadCheckpoint(ctx, logobs.SourceSystem)
	if err != nil || cp.Revision != 0 {
		t.Fatalf("initial %#v %v", cp, err)
	}
	b := logSuccess(logTestNow)
	b.NextOpaque = []byte("synthetic-private-cursor")
	if err := s.CommitBatch(ctx, b); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, DefaultConfig(path), fixedClock{logTestNow})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cp, err = reopened.LoadCheckpoint(ctx, logobs.SourceSystem)
	if err != nil || cp.Revision != 1 || string(cp.Opaque) != "synthetic-private-cursor" || !cp.CoverageThrough.Equal(logTestNow) {
		t.Fatalf("checkpoint %#v %v", cp, err)
	}
	var version int
	if err := reopened.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 2 {
		t.Fatalf("core version %d %v", version, err)
	}
	if reopened.Health().QuarantinePath != "" {
		t.Fatal("valid host database quarantined")
	}
}

func TestLogCommitCASAndAtomicRollback(t *testing.T) {
	s := openTestStore(t, DefaultConfig(filepath.Join(t.TempDir(), "history.db")))
	ctx := context.Background()
	b := logSuccess(logTestNow)
	b.NextOpaque = []byte("synthetic-cursor")
	b.ExaminedCount = 1
	b.Events = []logobs.Event{{ObservedAt: logTestNow.Add(-time.Minute), Source: b.Source, Severity: logobs.SeverityError, EventCode: "WIN_9"}}
	if err := s.CommitBatch(ctx, b); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitBatch(ctx, b); !errors.Is(err, logobs.ErrRevisionConflict) {
		t.Fatalf("replay %v", err)
	}
	query := logobs.SummaryQuery{WindowStart: logTestNow.Add(-time.Hour), WindowEnd: logTestNow, BucketInterval: time.Minute, BucketCount: 60}
	summary, err := s.QuerySummary(ctx, []logobs.Source{logobs.SourceSystem}, query)
	if err != nil || summary.Validate() != nil || summary.Sources[0].Counts.Captured != 1 || summary.Sources[0].CoveredSeconds != 300 {
		t.Fatalf("summary %#v %v", summary, err)
	}
	if summary.Sources[0].Buckets[0].Counts != nil {
		t.Fatal("unknown interval fabricated zero")
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_log_state BEFORE UPDATE ON log_metadata_state BEGIN SELECT RAISE(ABORT,'synthetic-write-failure'); END`); err != nil {
		t.Fatal(err)
	}
	b.QueryStartedAt = b.QueryStartedAt.Add(time.Minute)
	b.StartedAt = b.QueryStartedAt
	b.FinishedAt = b.QueryStartedAt
	b.ExpectedRevision = 1
	if err := s.CommitBatch(ctx, b); err == nil {
		t.Fatal("injected write failure ignored")
	}
	cp, err := s.LoadCheckpoint(ctx, b.Source)
	if err != nil || cp.Revision != 1 {
		t.Fatalf("failed transaction advanced %#v %v", cp, err)
	}
	after, err := s.QuerySummary(ctx, []logobs.Source{b.Source}, query)
	if err != nil || after.Sources[0].Counts.Captured != 1 {
		t.Fatalf("counts escaped rollback %#v %v", after, err)
	}
}

func TestIncompatibleLogSchemaDoesNotReplaceHostHistory(t *testing.T) {
	s := openTestStore(t, DefaultConfig(filepath.Join(t.TempDir(), "history.db")))
	ctx := context.Background()
	if _, err := s.db.Exec(`INSERT INTO store_metadata(key,value) VALUES('log_metadata_schema_version','999')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadCheckpoint(ctx, logobs.SourceSystem); err == nil {
		t.Fatal("accepted future log schema")
	}
	if err := s.WriteSamples(ctx, []Sample{{Metric: CPUUtilization, At: logTestNow, Value: 12}}); err != nil {
		t.Fatal(err)
	}
	if s.Health().QuarantinePath != "" {
		t.Fatal("log schema quarantined host state")
	}
}

func TestLogTimeEncodingRoundTripsWithoutUnixNanoOverflow(t *testing.T) {
	for _, at := range []time.Time{time.Date(1, 1, 1, 0, 0, 0, 1, time.UTC), time.Date(1677, 9, 1, 0, 0, 0, 19, time.UTC), logTestNow.Add(123 * time.Nanosecond), time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC)} {
		encoded, err := encodeLogTime(at)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeLogTime(encoded)
		if err != nil || !decoded.Equal(at) {
			t.Fatalf("%s: %s %v", at, decoded, err)
		}
	}
	for _, text := range []string{"", "0001-01-01T00:00:00.000000000Z", "2026-09-09T12:00:00Z", "2026-09-09T12:00:00.000000000+00:00"} {
		if _, err := decodeLogTime(text); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
}

func TestLogGapCountsAndLatestFailureAreIndependent(t *testing.T) {
	s := openTestStore(t, DefaultConfig(filepath.Join(t.TempDir(), "history.db")))
	ctx := context.Background()
	q := logTestNow
	first := logSuccess(q)
	first.NextOpaque = []byte("synthetic-cursor")
	if err := s.CommitBatch(ctx, first); err != nil {
		t.Fatal(err)
	}
	reason := logobs.ReasonBacklogDeferred
	second := logSuccess(q.Add(10 * time.Minute))
	second.ExpectedRevision = 1
	second.CaughtUp = false
	second.CollectionState = logobs.CollectionPartial
	second.ReasonCode = &reason
	second.ExaminedCount = 2
	second.Deferred = true
	second.NextOpaque = []byte("synthetic-later-cursor")
	second.Events = []logobs.Event{{ObservedAt: q.Add(5 * time.Minute), Source: second.Source, Severity: logobs.SeverityWarn, EventCode: "WIN_4"}}
	if err := s.CommitBatch(ctx, second); err != nil {
		t.Fatal(err)
	}
	third := logSuccess(q.Add(11 * time.Minute))
	third.ExpectedRevision = 2
	third.CaughtUp = false
	third.SupportState = logobs.SupportPermissionDenied
	third.CollectionState = logobs.CollectionNotRun
	reason = logobs.ReasonPermissionDenied
	third.ReasonCode = &reason
	if err := s.CommitBatch(ctx, third); err != nil {
		t.Fatal(err)
	}
	query := logobs.SummaryQuery{WindowStart: q.Add(-49 * time.Minute), WindowEnd: q.Add(11 * time.Minute), BucketInterval: time.Minute, BucketCount: 60}
	got, err := s.QuerySummary(ctx, []logobs.Source{logobs.SourceSystem}, query)
	if err != nil {
		t.Fatal(err)
	}
	source := got.Sources[0]
	bucket := source.Buckets[54]
	if source.Status.SupportState != logobs.SupportPermissionDenied || source.Status.ObservedAt != nil {
		t.Fatalf("latest quality %+v", source.Status)
	}
	if source.Counts.Captured != 1 || source.CoveredSeconds != 300 || bucket.Counts.Captured != 1 || bucket.CoverageState != logobs.CoverageGapState || *bucket.ReasonCode != logobs.ReasonBacklogDeferred {
		t.Fatalf("history erased/promoted %+v", source)
	}
	cp, err := s.LoadCheckpoint(ctx, logobs.SourceSystem)
	if err != nil || !cp.CoverageThrough.Equal(q) || string(cp.Opaque) != "synthetic-later-cursor" {
		t.Fatalf("watermark %#v %v", cp, err)
	}
	// Summary/checkpoint ports return owned snapshots.
	got.Sources[0].Buckets[54].Counts.Captured = 900
	cp.Opaque[0] = 'X'
	again, err := s.QuerySummary(ctx, []logobs.Source{logobs.SourceSystem}, query)
	if err != nil || again.Sources[0].Counts.Captured != 1 {
		t.Fatal("summary retained caller mutation")
	}
	cp, err = s.LoadCheckpoint(ctx, logobs.SourceSystem)
	if err != nil || string(cp.Opaque) != "synthetic-later-cursor" {
		t.Fatal("checkpoint retained caller mutation")
	}
}

func TestLogCounterOverflowRollsBackCheckpointAndEverySource(t *testing.T) {
	s := openTestStore(t, DefaultConfig(filepath.Join(t.TempDir(), "history.db")))
	ctx := context.Background()
	first := logSuccess(logTestNow)
	if err := s.CommitBatch(ctx, first); err != nil {
		t.Fatal(err)
	}
	minute, _ := encodeLogTime(logTestNow.Add(-time.Minute))
	if _, err := s.db.Exec(`INSERT INTO log_metadata_minutes(source,minute,captured,discarded,trace,debug,info,warn,error,critical,unknown) VALUES('system',?,?,0,0,0,?,0,0,0,0)`, minute, logobs.MaxSafeInteger, logobs.MaxSafeInteger); err != nil {
		t.Fatal(err)
	}
	second := logSuccess(logTestNow)
	second.Source = logobs.SourceApplication
	second.ExaminedCount = 1
	second.NextOpaque = []byte("synthetic-cursor")
	second.Events = []logobs.Event{{ObservedAt: logTestNow.Add(-time.Minute), Source: second.Source, Severity: logobs.SeverityInfo, EventCode: "WIN_1"}}
	if err := s.CommitBatch(ctx, second); err == nil {
		t.Fatal("cross-source safe integer overflow accepted")
	}
	cp, err := s.LoadCheckpoint(ctx, second.Source)
	if err != nil || cp.Revision != 0 {
		t.Fatalf("overflow advanced %#v %v", cp, err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM log_metadata_minutes WHERE source='application'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("overflow persisted counts")
	}
	second.Source = logobs.SourceSystem
	second.ExpectedRevision = 1
	second.QueryStartedAt = second.QueryStartedAt.Add(time.Minute)
	second.StartedAt = second.QueryStartedAt
	second.FinishedAt = second.QueryStartedAt
	second.Events[0].Source = second.Source
	if err := s.CommitBatch(ctx, second); err == nil {
		t.Fatal("individual minute overflow accepted")
	}
	cp, err = s.LoadCheckpoint(ctx, second.Source)
	if err != nil || cp.Revision != 1 {
		t.Fatal("minute overflow advanced checkpoint")
	}
}

func TestLogSummaryReadOnlyAndBoundedInputs(t *testing.T) {
	s := openTestStore(t, DefaultConfig(filepath.Join(t.TempDir(), "history.db")))
	ctx := context.Background()
	query := logobs.SummaryQuery{WindowStart: logTestNow.Add(-time.Hour), WindowEnd: logTestNow, BucketInterval: time.Minute, BucketCount: 60}
	if _, err := s.QuerySummary(ctx, []logobs.Source{logobs.SourceSystem}, query); err == nil {
		t.Fatal("missing schema pretended available")
	}
	var tables int
	if err := s.db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name LIKE 'log_metadata_%'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatal("summary initialized schema")
	}
	if _, err := s.QuerySummary(ctx, nil, query); err != nil {
		t.Fatal(err)
	}
	for _, sources := range [][]logobs.Source{{"arbitrary"}, {logobs.SourceSystem, logobs.SourceSystem}, {logobs.SourceApplication, logobs.SourceSystem}, {logobs.SourceSystem, logobs.SourceApplication, logobs.SourceSystem}} {
		if _, err := s.QuerySummary(ctx, sources, query); err == nil {
			t.Fatalf("accepted source list %v", sources)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.LoadCheckpoint(canceled, logobs.SourceSystem); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel %v", err)
	}
	if _, err := s.LoadCheckpoint(ctx, logobs.SourceSystem); err != nil {
		t.Fatalf("cancelled init poisoned future attempts: %v", err)
	}
}

func TestLogCompactRollupsDoNotPersistCodesOrOldBacklog(t *testing.T) {
	s := openTestStore(t, DefaultConfig(filepath.Join(t.TempDir(), "history.db")))
	ctx := context.Background()
	b := logSuccess(logTestNow)
	b.NextOpaque = []byte("synthetic-cursor")
	b.ExaminedCount = 2
	b.Events = []logobs.Event{{ObservedAt: logTestNow.Add(-8 * 24 * time.Hour), Source: b.Source, Severity: logobs.SeverityInfo, EventCode: "WIN_7"}, {ObservedAt: logTestNow.Add(-time.Minute), Source: b.Source, Severity: logobs.SeverityCritical, EventCode: "WIN_987654321"}}
	if err := s.CommitBatch(ctx, b); err != nil {
		t.Fatal(err)
	}
	var captured, rows int
	if err := s.db.QueryRow(`SELECT sum(captured),count(*) FROM log_metadata_minutes`).Scan(&captured, &rows); err != nil || captured != 1 || rows != 1 {
		t.Fatalf("old backlog retained %d %d %v", captured, rows, err)
	}
	var columns int
	if err := s.db.QueryRow(`SELECT count(*) FROM pragma_table_info('log_metadata_minutes') WHERE name LIKE '%code%' OR name LIKE '%body%' OR name LIKE '%message%'`).Scan(&columns); err != nil || columns != 0 {
		t.Fatal("raw-event storage added")
	}
	for i := 1; i <= 30; i++ {
		b = logSuccess(logTestNow.Add(time.Duration(i) * time.Minute))
		b.ExpectedRevision = uint64(i)
		if err := s.CommitBatch(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.db.QueryRow(`SELECT count(*) FROM log_metadata_coverage`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("coverage not compact %d %v", rows, err)
	}
}

func TestLogUint64RevisionDoesNotNarrowToSQLiteSignedInteger(t *testing.T) {
	s := openTestStore(t, DefaultConfig(filepath.Join(t.TempDir(), "history.db")))
	ctx := context.Background()
	b := logSuccess(logTestNow)
	if err := s.CommitBatch(ctx, b); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE log_metadata_state SET revision='18446744073709551615'`); err != nil {
		t.Fatal(err)
	}
	cp, err := s.LoadCheckpoint(ctx, b.Source)
	if err != nil || cp.Revision != ^uint64(0) {
		t.Fatalf("narrowed revision %#v %v", cp, err)
	}
	b.ExpectedRevision = cp.Revision
	b.QueryStartedAt = b.QueryStartedAt.Add(time.Minute)
	b.StartedAt = b.QueryStartedAt
	b.FinishedAt = b.QueryStartedAt
	if err := s.CommitBatch(ctx, b); err == nil {
		t.Fatal("revision wrapped")
	}
}
