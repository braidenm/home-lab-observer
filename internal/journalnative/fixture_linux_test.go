//go:build linux && (amd64 || arm64)

package journalnative

import (
	"os"
	"runtime"
	"testing"

	"github.com/ebitengine/purego"
)

// TestSyntheticJournalDirectory exercises the real ABI only when CI supplies
// an owned synthetic journal directory. Production has no directory-opening
// path; this seam and its dynamic symbol exist only in the test binary.
func TestSyntheticJournalDirectory(t *testing.T) {
	directory := os.Getenv("OBSERVER_TEST_SYNTHETIC_JOURNAL_DIR")
	if directory == "" {
		t.Skip("owned synthetic native journal fixture was not supplied")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	factory, err := NewFactory()
	if err != nil {
		t.Fatalf("load fixed libsystemd binding: %v", err)
	}
	defer factory.Close()

	library, err := purego.Dlopen(systemdSONAME, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Fatal("test-only directory symbol library unavailable")
	}
	defer purego.Dlclose(library)
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

	opened, err := factory.OpenSystem()
	if err != nil {
		t.Fatalf("open owned synthetic journal directory: %v", err)
	}
	defer opened.Close()
	if err := opened.SeekTail(); err != nil {
		t.Fatal(err)
	}
	visited, err := opened.Previous()
	if err != nil || !visited {
		t.Fatalf("synthetic fixture tail = %v, %v", visited, err)
	}
	cursor, err := opened.Cursor()
	if err != nil || len(cursor) == 0 {
		t.Fatalf("synthetic fixture cursor = %d, %v", len(cursor), err)
	}
	priority, err := opened.Priority()
	if err != nil || string(priority) != "6" {
		t.Fatalf("synthetic fixture priority = %q, %v", priority, err)
	}
	messageID, err := opened.MessageID()
	if err != nil || string(messageID) != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("synthetic fixture message ID = %q, %v", messageID, err)
	}
	if micros, err := opened.RealtimeMicros(); err != nil || micros == 0 {
		t.Fatalf("synthetic fixture realtime = %d, %v", micros, err)
	}
	if err := opened.SeekCursor(cursor); err != nil {
		t.Fatal(err)
	}
	if visited, err = opened.Next(); err != nil || !visited {
		t.Fatalf("synthetic fixture cursor next = %v, %v", visited, err)
	}
	if exact, err := opened.TestCursor(cursor); err != nil || !exact {
		t.Fatalf("synthetic fixture exact cursor = %v, %v", exact, err)
	}
	messageID, err = opened.MessageID()
	if err != nil || string(messageID) != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("synthetic fixture metadata after exact cursor proof = %q, %v", messageID, err)
	}
}
