//go:build linux && (amd64 || arm64)

package journalnative

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/journalreader"
	"github.com/braidenm/home-lab-observer/internal/logobs"
	"github.com/ebitengine/purego"
)

const fixtureMessageID = "0123456789abcdef0123456789abcdef"
const fixtureMarker = "observer-owned-synthetic-journal-v1\n"

// TestSyntheticJournalDirectory exercises the real ABI only when CI supplies
// an owned synthetic journal directory. Production has no directory-opening
// path; this seam and its dynamic symbol exist only in the test binary.
func TestSyntheticJournalDirectory(t *testing.T) {
	directory := os.Getenv("OBSERVER_TEST_SYNTHETIC_JOURNAL_DIR")
	queryMicrosText := os.Getenv("OBSERVER_TEST_SYNTHETIC_QUERY_MICROS")
	if directory == "" && queryMicrosText == "" {
		t.Skip("owned synthetic native journal fixture was not supplied")
	}
	if directory == "" || queryMicrosText == "" {
		t.Fatal("synthetic fixture directory and query time must be supplied together")
	}
	assertOwnedFixtureDirectory(t, directory)
	queryMicros, err := strconv.ParseInt(queryMicrosText, 10, 64)
	if err != nil || queryMicros <= 0 {
		t.Fatal("synthetic fixture query time is invalid")
	}
	queryStartedAt := time.UnixMicro(queryMicros).UTC()
	factory := openDirectoryFactory(t, directory)

	clock := queryStartedAt.Add(time.Millisecond)
	reader, err := journalreader.New(journalreader.Config{Factory: factory, Now: func() time.Time { return clock }})
	if err != nil {
		t.Fatal("construct native fixture reader")
	}
	request := logobs.ReadRequest{Source: logobs.SourceSystem, QueryStartedAt: queryStartedAt}
	batch, err := reader.Read(context.Background(), request)
	if err != nil || batch.Validate() != nil {
		t.Fatalf("read synthetic native fixture: %v", err)
	}
	if batch.Kind != logobs.BatchNormal || batch.SupportState != logobs.SupportSupported ||
		batch.CollectionState != logobs.CollectionPartial || batch.ReasonCode == nil ||
		*batch.ReasonCode != logobs.ReasonInvalidResponse || !batch.CaughtUp || batch.Deferred {
		t.Fatal("synthetic fixture quality state is incorrect")
	}
	if batch.ExaminedCount != 3 || batch.ProbeCount != 0 || batch.DiscardedCount != 1 ||
		len(batch.Discards) != 1 || len(batch.Events) != 2 || len(batch.NextOpaque) == 0 {
		t.Fatal("synthetic fixture bounds or counts are incorrect")
	}
	if batch.Events[0].ObservedAt != queryStartedAt.Add(-3*time.Second) ||
		batch.Events[0].Severity != logobs.SeverityWarn || batch.Events[0].EventCode != "SYSTEMD_PRIORITY_4" {
		t.Fatal("first synthetic event was not normalized from native metadata")
	}
	if batch.Events[1].ObservedAt != queryStartedAt.Add(-time.Second) ||
		batch.Events[1].Severity != logobs.SeverityInfo || batch.Events[1].EventCode != "SYSTEMD_"+fixtureMessageID {
		t.Fatal("last synthetic event was not normalized from native metadata")
	}
	if batch.Discards[0].At != queryStartedAt.Add(-2*time.Second) || batch.Discards[0].Count != 1 {
		t.Fatal("oversized native field did not become one bounded discard")
	}
	encoded, err := json.Marshal(batch)
	if err != nil || bytes.Contains(encoded, []byte("PRIVATE_FIXTURE_BODY_MUST_NOT_ESCAPE")) || bytes.Contains(encoded, []byte("MESSAGE=")) {
		t.Fatal("unselected fixture body entered the normalized batch")
	}

	firstCursor := append([]byte(nil), batch.NextOpaque...)
	previous := queryStartedAt
	request = logobs.ReadRequest{Source: logobs.SourceSystem, QueryStartedAt: queryStartedAt.Add(time.Minute),
		Checkpoint: logobs.Checkpoint{Revision: 1, Opaque: firstCursor, PreviousAttemptAt: &previous}}
	clock = request.QueryStartedAt.Add(time.Millisecond)
	continued, err := reader.Read(context.Background(), request)
	if err != nil || continued.Validate() != nil {
		t.Fatalf("continue synthetic native fixture: %v", err)
	}
	if continued.Kind != logobs.BatchNormal || continued.CollectionState != logobs.CollectionOK ||
		!continued.CaughtUp || continued.ExaminedCount != 1 || continued.ProbeCount != 1 ||
		len(continued.Events) != 0 || continued.DiscardedCount != 0 || !bytes.Equal(continued.NextOpaque, firstCursor) {
		t.Fatal("synthetic native cursor continuation replayed or skipped a record")
	}
	if err := factory.Close(); err != nil {
		t.Fatal("close native fixture factory")
	}
}

func assertOwnedFixtureDirectory(t *testing.T, directory string) {
	t.Helper()
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		t.Fatal("synthetic fixture directory must be a clean absolute path")
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("synthetic fixture directory is not an owned real directory")
	}
	markerPath := filepath.Join(directory, ".observer-owned-fixture")
	info, err = os.Lstat(markerPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != int64(len(fixtureMarker)) {
		t.Fatal("synthetic fixture ownership marker is invalid")
	}
	marker, err := os.ReadFile(markerPath)
	if err != nil || string(marker) != fixtureMarker {
		t.Fatal("synthetic fixture ownership marker does not match")
	}
}

func openDirectoryFactory(t *testing.T, directory string) *Factory {
	t.Helper()
	factory, err := NewFactory()
	if err != nil {
		t.Fatalf("load fixed libsystemd binding: %v", err)
	}
	library, err := purego.Dlopen(systemdSONAME, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		_ = factory.Close()
		t.Fatal("test-only directory symbol library unavailable")
	}
	t.Cleanup(func() {
		_ = factory.Close()
		_ = purego.Dlclose(library)
	})
	address, err := purego.Dlsym(library, "sd_journal_open_directory")
	if err != nil || address == 0 {
		t.Fatal("test-only directory symbol unavailable")
	}
	var openDirectory func(*uintptr, string, int32) int32
	if !registerBindings([]binding{{name: "sd_journal_open_directory", target: &openDirectory}}, []uintptr{address}) {
		t.Fatal("test-only directory symbol registration failed")
	}
	factory.calls.open = func(handle *uintptr, flags int32) int32 {
		if flags != systemJournalFlags {
			return -1
		}
		return openDirectory(handle, directory, 0)
	}
	return factory
}

func TestFixedLibraryMissingMapsToUnavailable(t *testing.T) {
	for _, loader := range []func() (nativeCalls, func() error, error){nil, func() (nativeCalls, func() error, error) {
		return nativeCalls{}, nil, errNativeUnavailable
	}} {
		factory, err := newFactoryFromLoader(loader)
		if factory != nil || !errors.Is(err, journalreader.ErrUnavailable) {
			t.Fatalf("missing library result = %v, %v", factory, err)
		}
	}
}
