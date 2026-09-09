package diagnostics

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDisabledDoesNotTouchFilesystem(t *testing.T) {
	state := filepath.Join(canonicalTempDir(t), "missing")
	writer := New(Config{StateDir: state, Enabled: false})
	health := writer.Health()
	if health.Enabled || health.Available || health.State != "DISABLED" || health.ReasonCode != "DIAGNOSTICS_DISABLED" {
		t.Fatalf("unexpected health: %+v", health)
	}
	if _, err := os.Lstat(state); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disabled writer touched disk: %v", err)
	}
}

func TestRecordWritesOnlyClosedStructuredVocabulary(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.FixedZone("test", -6*60*60))
	state := testStateDir(t)
	writer := New(Config{StateDir: state, Enabled: true, Now: func() time.Time { return now }})
	defer writer.Close()
	writer.Record(Event{Kind: EventRuntimeStarted, Code: CodeOK, Version: "0.2.0-preview.1", Count: 1, Duration: 2500 * time.Millisecond})

	path := filepath.Join(state, "diagnostics", "observer.jsonl")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(contents))), &got); err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{"observed_at", "event", "code", "version", "count", "duration_ms"}
	if len(got) != len(wantKeys) {
		t.Fatalf("record fields = %v", got)
	}
	for _, key := range wantKeys {
		if _, ok := got[key]; !ok {
			t.Fatalf("record missing %s: %v", key, got)
		}
	}
	if got["observed_at"] != "2026-09-09T18:00:00Z" || got["duration_ms"] != float64(2500) {
		t.Fatalf("record = %v", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o", info.Mode().Perm())
	}
}

func TestInvalidOrSensitiveLookingValuesAreDropped(t *testing.T) {
	state := testStateDir(t)
	writer := New(Config{StateDir: state, Enabled: true})
	defer writer.Close()
	secret := "github" + "_pat_" + strings.Repeat("A", 30)
	writer.Record(Event{Kind: EventRuntimeStarted, Code: CodeOK, Version: secret})
	writer.Record(Event{Kind: "USER_MESSAGE", Code: CodeOK, Version: "dev"})
	writer.Record(Event{Kind: EventRuntimeReady, Code: "TOKEN", Version: "dev"})
	health := writer.Health()
	if health.DroppedRecords != 3 || health.TotalBytes != 0 || health.FileCount != 0 {
		t.Fatalf("unexpected health: %+v", health)
	}
}

func TestRotationAndTotalBounds(t *testing.T) {
	state := testStateDir(t)
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	writer := New(Config{StateDir: state, Enabled: true, Now: func() time.Time { return now }})
	defer writer.Close()
	directory := filepath.Join(state, "diagnostics")
	for index := 0; index < MaxFiles; index++ {
		path := filepath.Join(directory, diagnosticName(index))
		size := int64(32)
		if index == 0 {
			size = MaxFileBytes
		}
		writePaddedRecord(t, path, now, size)
	}
	writer.Record(Event{Kind: EventRuntimeReady, Code: CodeOK, Version: "dev"})
	health := writer.Health()
	if !health.Available || health.FileCount != MaxFiles || health.TotalBytes > MaxTotalBytes {
		t.Fatalf("unexpected health: %+v", health)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || info.Size() > MaxFileBytes {
			t.Fatalf("unbounded rotation %s: %v, %v", entry.Name(), info, err)
		}
	}
}

func TestAgeUsesOldestRecordAndHousekeepingRunsWithoutNewRecord(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	state := testStateDir(t)
	writer := New(Config{StateDir: state, Enabled: true, Now: func() time.Time { return now }})
	defer writer.Close()
	writer.Record(Event{Kind: EventRuntimeStarted, Code: CodeOK, Version: "dev"})
	path := filepath.Join(state, "diagnostics", "observer.jsonl")
	now = now.Add(7*24*time.Hour + time.Second)
	// Appending would refresh mtime; retention still keys off the first record.
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatal(err)
	}
	writer.housekeep()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired active file retained: %v", err)
	}
}

func TestWriteFailureIsNonFatalAndVisible(t *testing.T) {
	state := testStateDir(t)
	writer := New(Config{StateDir: state, Enabled: true})
	defer writer.Close()
	writer.openFile = func(string, int, os.FileMode) (*os.File, error) { return nil, errors.New("synthetic write failure") }
	writer.Record(Event{Kind: EventRuntimeReady, Code: CodeOK, Version: "dev"})
	health := writer.Health()
	if health.Available || health.State != "UNAVAILABLE" || health.ReasonCode != "DIAGNOSTICS_UNAVAILABLE" || health.WriteFailures != 1 || health.DroppedRecords != 1 {
		t.Fatalf("unexpected health: %+v", health)
	}
}

func TestUnknownSymlinkAndHardLinkEntriesAreRefusedWithoutDeletion(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string, string)
	}{
		{"unknown", func(t *testing.T, directory, _ string) {
			mustWrite(t, filepath.Join(directory, "notes.txt"), []byte("synthetic-canary"))
		}},
		{"symlink", func(t *testing.T, directory, target string) {
			if err := os.Symlink(target, filepath.Join(directory, "observer.jsonl")); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
		}},
		{"hardlink", func(t *testing.T, directory, target string) {
			if err := os.Link(target, filepath.Join(directory, "observer.jsonl")); err != nil {
				t.Skipf("hard links unavailable: %v", err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := testStateDir(t)
			directory := filepath.Join(state, "diagnostics")
			if err := os.Mkdir(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(state, "outside")
			mustWrite(t, target, []byte("synthetic-canary"))
			test.setup(t, directory, target)
			writer := New(Config{StateDir: state, Enabled: true})
			defer writer.Close()
			if writer.Health().ReasonCode != "UNSAFE_DIAGNOSTICS_PATH" {
				t.Fatalf("health = %+v", writer.Health())
			}
			contents, err := os.ReadFile(target)
			if err != nil || string(contents) != "synthetic-canary" {
				t.Fatalf("outside file changed: %q, %v", contents, err)
			}
		})
	}
}

func TestConcurrentRecordsRemainWhole(t *testing.T) {
	state := testStateDir(t)
	writer := New(Config{StateDir: state, Enabled: true})
	defer writer.Close()
	var group sync.WaitGroup
	for index := 0; index < 100; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			writer.Record(Event{Kind: EventCollectionCycle, Code: CodeOK, Version: "dev", Count: 1, Duration: time.Millisecond})
		}()
	}
	group.Wait()
	file, err := os.Open(filepath.Join(state, "diagnostics", "observer.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner, count := bufio.NewScanner(file), 0
	for scanner.Scan() {
		var record diskRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 100 {
		t.Fatalf("record count = %d", count)
	}
}

func writePaddedRecord(t *testing.T, path string, at time.Time, size int64) {
	t.Helper()
	record, err := json.Marshal(diskRecord{ObservedAt: at.Format(time.RFC3339Nano), Event: EventRuntimeReady, Code: CodeOK, Version: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	contents := append(record, '\n')
	if int64(len(contents)) > size {
		size = int64(len(contents))
	}
	contents = append(contents, make([]byte, size-int64(len(contents)))...)
	mustWrite(t, path, contents)
}

func mustWrite(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}

func testStateDir(t *testing.T) string {
	t.Helper()
	state := filepath.Join(canonicalTempDir(t), "state")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	return state
}

func canonicalTempDir(t *testing.T) string {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return directory
}
