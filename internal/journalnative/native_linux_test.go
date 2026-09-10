//go:build linux && (amd64 || arm64)

package journalnative

import (
	"bytes"
	"errors"
	"os"
	"runtime"
	"testing"
	"unsafe"

	"github.com/braidenm/home-lab-observer/internal/journalreader"
	"github.com/braidenm/home-lab-observer/internal/logobs"
	"golang.org/x/sys/unix"
)

type fakeNative struct {
	calls          nativeCalls
	flags          int32
	threshold      uintptr
	closes         int
	libraryCloses  int
	frees          int
	nextResult     int32
	previousResult int32
	openResult     int32
	seekResult     int32
	dataResult     int32
	cursorResult   int32
	testResult     int32
	realtimeResult int32
	realtime       uint64
	data           []byte
	cursor         []byte
	lastField      string
	lastCursor     string
}

func newFakeNative() *fakeNative {
	fake := &fakeNative{nextResult: 1, previousResult: 1, testResult: 1, realtime: 42}
	fake.calls = nativeCalls{
		open: func(handle *uintptr, flags int32) int32 {
			fake.flags = flags
			if fake.openResult >= 0 {
				*handle = 0x1234
			}
			return fake.openResult
		},
		close:        func(uintptr) { fake.closes++ },
		next:         func(uintptr) int32 { return fake.nextResult },
		previous:     func(uintptr) int32 { return fake.previousResult },
		seekRealtime: func(uintptr, uint64) int32 { return fake.seekResult },
		seekCursor: func(_ uintptr, cursor string) int32 {
			fake.lastCursor = cursor
			return fake.seekResult
		},
		seekTail: func(uintptr) int32 { return fake.seekResult },
		getRealtime: func(_ uintptr, value *uint64) int32 {
			*value = fake.realtime
			return fake.realtimeResult
		},
		setDataThreshold: func(_ uintptr, size uintptr) int32 {
			fake.threshold = size
			return 0
		},
		getData: func(_ uintptr, field string, pointer *unsafe.Pointer, size *uintptr) int32 {
			fake.lastField = field
			if fake.dataResult >= 0 && len(fake.data) > 0 {
				*pointer = unsafe.Pointer(&fake.data[0])
				*size = uintptr(len(fake.data))
			}
			return fake.dataResult
		},
		getCursor: func(_ uintptr, pointer *unsafe.Pointer) int32 {
			if len(fake.cursor) > 0 {
				*pointer = unsafe.Pointer(&fake.cursor[0])
			}
			return fake.cursorResult
		},
		testCursor: func(_ uintptr, cursor string) int32 {
			fake.lastCursor = cursor
			return fake.testResult
		},
		free: func(unsafe.Pointer) { fake.frees++ },
	}
	return fake
}

func openFake(t *testing.T, fake *fakeNative) (*Factory, journalreader.Journal) {
	t.Helper()
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	factory := newFactory(fake.calls, func() error {
		fake.libraryCloses++
		return nil
	})
	opened, err := factory.OpenSystem()
	if err != nil {
		t.Fatalf("open fake journal: %v", err)
	}
	return factory, opened
}

func TestFactoryUsesFixedSystemScopeThresholdAndLifetime(t *testing.T) {
	fake := newFakeNative()
	factory, opened := openFake(t, fake)
	if systemJournalFlags != 5 || fake.flags != systemJournalFlags {
		t.Fatalf("open flags = %d", fake.flags)
	}
	if dataThreshold != logobs.MaxNativeFieldBytes+len("MESSAGE_ID=")+1 || fake.threshold != uintptr(dataThreshold) {
		t.Fatalf("data threshold = %d", fake.threshold)
	}
	if err := factory.Close(); !errors.Is(err, journalreader.ErrReadFailed) {
		t.Fatalf("factory closed around active handle: %v", err)
	}
	if ok, err := opened.Next(); err != nil || !ok {
		t.Fatalf("next = %v, %v", ok, err)
	}
	if ok, err := opened.Previous(); err != nil || !ok {
		t.Fatalf("previous = %v, %v", ok, err)
	}
	if err := opened.SeekRealtime(12); err != nil {
		t.Fatal(err)
	}
	if err := opened.SeekTail(); err != nil {
		t.Fatal(err)
	}
	if err := opened.SeekCursor([]byte("s=cursor")); err != nil || fake.lastCursor != "s=cursor" {
		t.Fatalf("seek cursor = %q, %v", fake.lastCursor, err)
	}
	if exact, err := opened.TestCursor([]byte("s=cursor")); err != nil || !exact {
		t.Fatalf("test cursor = %v, %v", exact, err)
	}
	if micros, err := opened.RealtimeMicros(); err != nil || micros != fake.realtime {
		t.Fatalf("realtime = %d, %v", micros, err)
	}
	opened.Close()
	opened.Close()
	if fake.closes != 1 {
		t.Fatalf("native close count = %d", fake.closes)
	}
	if err := factory.Close(); err != nil || fake.libraryCloses != 1 {
		t.Fatalf("library close = %d, %v", fake.libraryCloses, err)
	}
	if _, err := factory.OpenSystem(); !errors.Is(err, journalreader.ErrUnavailable) {
		t.Fatalf("closed factory reopened: %v", err)
	}
}

func TestFieldsAreFixedBoundedAndOwned(t *testing.T) {
	fake := newFakeNative()
	factory, opened := openFake(t, fake)
	defer factory.Close()
	defer opened.Close()

	fake.data = []byte("PRIORITY=6")
	priority, err := opened.Priority()
	if err != nil || string(priority) != "6" || fake.lastField != "PRIORITY" {
		t.Fatalf("priority = %q, %v", priority, err)
	}
	fake.data[len(fake.data)-1] = '7'
	if string(priority) != "6" {
		t.Fatal("priority aliases borrowed native memory")
	}
	fake.data = []byte("MESSAGE_ID=0123456789abcdef0123456789abcdef")
	messageID, err := opened.MessageID()
	if err != nil || string(messageID) != "0123456789abcdef0123456789abcdef" || fake.lastField != "MESSAGE_ID" {
		t.Fatalf("message ID = %q, %v", messageID, err)
	}
	fake.data = append([]byte("MESSAGE_ID="), bytes.Repeat([]byte("x"), logobs.MaxNativeFieldBytes)...)
	maximum, err := opened.MessageID()
	if err != nil || len(maximum) != logobs.MaxNativeFieldBytes {
		t.Fatalf("maximum field length = %d, %v", len(maximum), err)
	}

	for name, data := range map[string][]byte{
		"wrong prefix":       []byte("MESSAGE=private-canary"),
		"truncated prefix":   []byte("PRIO"),
		"oversized priority": append([]byte("PRIORITY="), bytes.Repeat([]byte("x"), logobs.MaxNativeFieldBytes+1)...),
	} {
		fake.data = data
		value, fieldErr := opened.Priority()
		if len(value) != 0 {
			t.Fatalf("%s: invalid native field returned a payload", name)
		}
		if name == "oversized priority" {
			if !errors.Is(fieldErr, journalreader.ErrFieldTooLarge) {
				t.Fatalf("%s: oversized field = %v", name, fieldErr)
			}
		} else if !errors.Is(fieldErr, journalreader.ErrReadFailed) {
			t.Fatalf("%s: malformed field = %v", name, fieldErr)
		}
	}
}

func TestFieldNativeOversizeSentinelReturnsNoPayload(t *testing.T) {
	fake := newFakeNative()
	factory, opened := openFake(t, fake)
	defer factory.Close()
	defer opened.Close()
	fake.data = []byte("PRIORITY=PRIVATE_NATIVE_CANARY")
	fake.dataResult = -int32(unix.ENOBUFS)
	value, err := opened.Priority()
	if len(value) != 0 || !errors.Is(err, journalreader.ErrFieldTooLarge) {
		t.Fatalf("native oversize sentinel returned %q, %v", value, err)
	}
}

func TestCursorIsBoundedOwnedAndAlwaysFreed(t *testing.T) {
	fake := newFakeNative()
	factory, opened := openFake(t, fake)
	defer factory.Close()
	defer opened.Close()

	fake.cursor = append([]byte("s=synthetic-cursor"), 0)
	cursor, err := opened.Cursor()
	if err != nil || string(cursor) != "s=synthetic-cursor" || fake.frees != 1 {
		t.Fatalf("cursor = %q, frees=%d, err=%v", cursor, fake.frees, err)
	}
	fake.cursor[0] = 'X'
	if string(cursor) != "s=synthetic-cursor" {
		t.Fatal("cursor aliases malloc-owned memory")
	}
	fake.cursor = append(bytes.Repeat([]byte("x"), logobs.MaxCheckpointBytes+1), 0)
	if value, cursorErr := opened.Cursor(); len(value) != 0 || !errors.Is(cursorErr, journalreader.ErrReadFailed) || fake.frees != 2 {
		t.Fatalf("oversized cursor payload=%d frees=%d err=%v", len(value), fake.frees, cursorErr)
	}
	fake.cursor = append([]byte("s=must-free-on-error"), 0)
	fake.cursorResult = -int32(unix.EIO)
	if value, cursorErr := opened.Cursor(); len(value) != 0 || !errors.Is(cursorErr, journalreader.ErrReadFailed) || fake.frees != 3 {
		t.Fatalf("failed cursor payload=%d frees=%d err=%v", len(value), fake.frees, cursorErr)
	}
}

func TestFixedErrnoReductionAndCursorInput(t *testing.T) {
	for _, test := range []struct {
		name string
		code int32
		want error
	}{
		{"permission", -int32(unix.EACCES), journalreader.ErrPermissionDenied},
		{"permission operation", -int32(unix.EPERM), journalreader.ErrPermissionDenied},
		{"missing field", -int32(unix.ENOENT), journalreader.ErrFieldMissing},
		{"compressed too large", -int32(unix.ENOBUFS), journalreader.ErrFieldTooLarge},
		{"architecture too large", -int32(unix.E2BIG), journalreader.ErrFieldTooLarge},
		{"invalid cursor", -int32(unix.EINVAL), journalreader.ErrInvalidCursor},
		{"stale cursor", -int32(unix.ESTALE), journalreader.ErrInvalidCursor},
		{"other", -int32(unix.EIO), journalreader.ErrReadFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			var got error
			switch test.name {
			case "missing field", "compressed too large", "architecture too large":
				got = classifyField(test.code)
			case "invalid cursor", "stale cursor":
				got = classifyCursor(test.code)
			default:
				got = classifyGeneral(test.code)
			}
			if !errors.Is(got, test.want) {
				t.Fatalf("fixed mapping = %v", got)
			}
		})
	}
	for _, cursor := range [][]byte{nil, {}, []byte("has\x00nul"), bytes.Repeat([]byte("x"), logobs.MaxCheckpointBytes+1)} {
		if value, ok := cursorString(cursor); ok || value != "" {
			t.Fatal("invalid private cursor accepted")
		}
	}
}

func TestWrongThreadCannotUseOrCloseHandle(t *testing.T) {
	fake := newFakeNative()
	factory, opened := openFake(t, fake)
	type result struct {
		ok  bool
		err error
	}
	results := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		ok, err := opened.Next()
		opened.Close()
		results <- result{ok: ok, err: err}
	}()
	got := <-results
	if got.ok || !errors.Is(got.err, journalreader.ErrReadFailed) || fake.closes != 0 {
		t.Fatalf("wrong-thread result=%+v closes=%d", got, fake.closes)
	}
	opened.Close()
	if fake.closes != 1 {
		t.Fatalf("owner close count = %d", fake.closes)
	}
	if err := factory.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenFailureMappingAndCleanup(t *testing.T) {
	for _, test := range []struct {
		code int32
		want error
	}{
		{-int32(unix.EACCES), journalreader.ErrPermissionDenied},
		{-int32(unix.ENOENT), journalreader.ErrUnavailable},
	} {
		fake := newFakeNative()
		fake.openResult = test.code
		factory := newFactory(fake.calls, nil)
		opened, err := factory.OpenSystem()
		if opened != nil || !errors.Is(err, test.want) {
			t.Fatalf("open code %d = %v, %v", test.code, opened, err)
		}
	}
}

func TestThresholdFailureClosesHandle(t *testing.T) {
	fake := newFakeNative()
	fake.calls.setDataThreshold = func(uintptr, uintptr) int32 { return -int32(unix.EIO) }
	factory := newFactory(fake.calls, nil)
	opened, err := factory.OpenSystem()
	if opened != nil || !errors.Is(err, journalreader.ErrReadFailed) || fake.closes != 1 {
		t.Fatalf("threshold failure opened=%v closes=%d err=%v", opened, fake.closes, err)
	}
}

func TestFixedLibrarySymbolSetLoadsWhenRequested(t *testing.T) {
	if os.Getenv("OBSERVER_TEST_LIBSYSTEMD_LOAD") != "1" {
		t.Skip("fixed library load is an explicit Linux integration check")
	}
	factory, err := NewFactory()
	if err != nil {
		t.Fatalf("fixed libsystemd symbol set unavailable: %v", err)
	}
	if err := factory.Close(); err != nil {
		t.Fatalf("fixed libsystemd close: %v", err)
	}
}

func TestRegisterBindingsRecoversWithoutCallingAddress(t *testing.T) {
	bindings := []binding{{name: "synthetic", target: 42}}
	if registerBindings(bindings, []uintptr{1}) {
		t.Fatal("invalid purego target did not fail closed")
	}
}
