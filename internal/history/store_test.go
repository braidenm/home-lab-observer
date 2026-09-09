package history

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

func openTestStore(t *testing.T, config Config) *Store {
	t.Helper()
	store, err := Open(context.Background(), config, fixedClock{time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestMigrationWALAndRestartContinuity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	store := openTestStore(t, DefaultConfig(path))
	ctx := context.Background()
	var version int
	if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("version=%d err=%v", version, err)
	}
	var mode string
	if err := store.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("mode=%s err=%v", mode, err)
	}
	if health := store.Health(); versionLess(health.SQLiteVersion, MinimumSQLiteVersion) || health.SQLiteVersion != ExpectedSQLiteVersion {
		t.Fatalf("unexpected or unsafe sqlite version %s", health.SQLiteVersion)
	}
	if stats := store.db.Stats(); stats.MaxOpenConnections != 1 {
		t.Fatalf("writers are not serialized: %+v", stats)
	}
	first, _ := store.NextSequence(ctx)
	second, _ := store.NextSequence(ctx)
	if first != 1 || second != 2 {
		t.Fatalf("sequences=%d,%d", first, second)
	}
	at := time.Date(2026, 9, 9, 11, 0, 0, 0, time.UTC)
	if err := store.WriteSamples(ctx, []Sample{{Metric: CPUUtilization, At: at, Value: 12.5}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store2, err := Open(ctx, DefaultConfig(path), fixedClock{at})
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	third, _ := store2.NextSequence(ctx)
	if third != 3 {
		t.Fatalf("sequence after restart=%d", third)
	}
	samples, err := store2.Samples(ctx, CPUUtilization, at.Add(-time.Minute), at.Add(time.Minute), 10)
	if err != nil || len(samples) != 1 {
		t.Fatalf("samples=%v err=%v", samples, err)
	}
}

func TestAllowlistRejectsArbitraryMetric(t *testing.T) {
	store := openTestStore(t, DefaultConfig(filepath.Join(t.TempDir(), "history.db")))
	err := store.WriteSamples(context.Background(), []Sample{{Metric: "token.by_user", At: time.Now(), Value: 1}})
	if !errors.Is(err, ErrMetricNotAllowed) {
		t.Fatalf("err=%v", err)
	}
}

func TestRollupAndIncrementalAgeRetention(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	config := DefaultConfig(filepath.Join(t.TempDir(), "history.db"))
	config.RollupAfter = time.Hour
	config.RetentionAge = 24 * time.Hour
	config.BatchSize = 2
	store := openTestStore(t, config)
	ctx := context.Background()
	for index, value := range []float64{10, 20, 30} {
		at := now.Add(-2*time.Hour + time.Duration(index)*time.Second)
		if err := store.WriteSamples(ctx, []Sample{{Metric: CPUUtilization, At: at, Value: value}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Maintain(ctx, now); err != nil {
		t.Fatal(err)
	}
	rollups, err := store.Rollups(ctx, CPUUtilization, now.Add(-3*time.Hour), now, 10)
	if err != nil || len(rollups) != 1 || rollups[0].Count != 2 || rollups[0].Minimum != 10 || rollups[0].Maximum != 20 {
		t.Fatalf("rollups=%+v err=%v", rollups, err)
	}
	remaining, _ := store.Samples(ctx, CPUUtilization, now.Add(-3*time.Hour), now, 10)
	if len(remaining) != 1 {
		t.Fatalf("remaining=%d", len(remaining))
	}
	old := now.Add(-48 * time.Hour)
	if err := store.WriteSamples(ctx, []Sample{{Metric: MemoryUtilization, At: old, Value: 5}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Maintain(ctx, now); err != nil {
		t.Fatal(err)
	}
	if store.Health().AgeDropped == 0 {
		t.Fatal("age drop counter was not incremented")
	}
}

func TestSizeRetentionIsIncrementalAndObservable(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	config := DefaultConfig(filepath.Join(t.TempDir(), "history.db"))
	config.MaxBytes = 1
	config.BatchSize = 3
	config.RollupAfter = 365 * 24 * time.Hour
	store := openTestStore(t, config)
	ctx := context.Background()
	for index := 0; index < 10; index++ {
		if err := store.WriteSamples(ctx, []Sample{{Metric: ProcessCount, At: now.Add(time.Duration(index) * time.Second), Value: float64(index)}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Maintain(ctx, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := store.Health().SizeDropped; got != 3 {
		t.Fatalf("size dropped=%d", got)
	}
	remaining, _ := store.Samples(ctx, ProcessCount, now.Add(-time.Hour), now.Add(time.Hour), 20)
	if len(remaining) != 7 {
		t.Fatalf("remaining=%d", len(remaining))
	}
}

func TestCorruptAndIncompatibleDatabasesAreQuarantined(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		prepare func(*testing.T, string)
	}{{"corrupt", func(t *testing.T, path string) {
		if err := os.WriteFile(path, []byte("not sqlite and token=synthetic"), 0o600); err != nil {
			t.Fatal(err)
		}
	}}, {"incompatible", func(t *testing.T, path string) {
		store := openTestStore(t, DefaultConfig(path))
		if _, err := store.db.Exec("PRAGMA user_version=999"); err != nil {
			t.Fatal(err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}}} {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "history.db")
			testCase.prepare(t, path)
			store, err := Open(context.Background(), DefaultConfig(path), fixedClock{time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)})
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			health := store.Health()
			if health.State != "DEGRADED" || health.QuarantinePath == "" {
				t.Fatalf("health=%+v", health)
			}
			if _, err := os.Stat(health.QuarantinePath); err != nil {
				t.Fatalf("quarantine missing: %v", err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("replacement missing: %v", err)
			}
		})
	}
}

func TestCheckpointAndCountersPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	store := openTestStore(t, DefaultConfig(path))
	if err := store.Checkpoint(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	before := store.Health().CheckpointCount
	if before == 0 {
		t.Fatal("checkpoint not counted")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := Open(context.Background(), DefaultConfig(path), fixedClock{time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if again.Health().CheckpointCount < before {
		t.Fatalf("counter did not persist: %d < %d", again.Health().CheckpointCount, before)
	}
}
