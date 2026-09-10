//go:build windows && (amd64 || arm64)

package eventnative

import (
	"errors"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/braidenm/home-lab-observer/internal/eventreader"
)

func TestParseValuesCopiesTypedScalarsAndCanonicalGUID(t *testing.T) {
	const variantBytes = int(unsafe.Sizeof(evtVariant{}))
	buffer := make([]byte, len(selectedPaths)*variantBytes+int(unsafe.Sizeof(rawGUID{})))
	variants := unsafe.Slice((*evtVariant)(unsafe.Pointer(&buffer[0])), len(selectedPaths))
	variants[0] = evtVariant{value: uintptr(133_700_000_000_000_000), typeID: evtVariantFileTime}
	variants[1] = evtVariant{value: 3, typeID: evtVariantByte}
	variants[2] = evtVariant{value: 4096, typeID: evtVariantUInt16}
	guid := (*rawGUID)(unsafe.Pointer(&buffer[len(selectedPaths)*variantBytes]))
	*guid = rawGUID{data1: 0x00112233, data2: 0x4455, data3: 0x6677, data4: [8]byte{0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}}
	variants[3] = evtVariant{value: uintptr(unsafe.Pointer(guid)), typeID: evtVariantGUID}
	variants[4] = evtVariant{value: 987, typeID: evtVariantUInt64}
	actual, err := parseValues(buffer)
	if err != nil {
		t.Fatal(err)
	}
	expectedGUID := [16]byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	if actual.fileTime != uint64(variants[0].value) || actual.level != 3 || actual.eventID != 4096 || actual.recordID != 987 || !actual.hasGUID || actual.guid != expectedGUID {
		t.Fatalf("values=%#v", actual)
	}
	// Parsing copied the GUID; later native-buffer mutation cannot change it.
	guid.data4[0] = 0
	if actual.guid != expectedGUID {
		t.Fatal("GUID borrowed native render memory")
	}
}

func TestParseValuesPreservesPerFieldMissingAndInvalid(t *testing.T) {
	const variantBytes = int(unsafe.Sizeof(evtVariant{}))
	buffer := make([]byte, len(selectedPaths)*variantBytes)
	variants := unsafe.Slice((*evtVariant)(unsafe.Pointer(&buffer[0])), len(selectedPaths))
	variants[0] = evtVariant{typeID: evtVariantNull}
	variants[1] = evtVariant{value: 3, typeID: evtVariantUInt16}
	variants[2] = evtVariant{value: 7, typeID: evtVariantUInt16 | evtVariantArray}
	variants[3] = evtVariant{value: 1, typeID: evtVariantGUID}
	variants[4] = evtVariant{value: 4, typeID: evtVariantUInt64 | evtVariantArray}
	actual, err := parseValues(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(actual.errs[0], eventreader.ErrFieldMissing) || !errors.Is(actual.errs[1], eventreader.ErrFieldInvalid) || !errors.Is(actual.errs[2], eventreader.ErrFieldInvalid) || !errors.Is(actual.errs[3], eventreader.ErrFieldInvalid) || !errors.Is(actual.errs[4], eventreader.ErrFieldInvalid) {
		t.Fatalf("errors=%#v", actual.errs)
	}
}

func TestParseValuesAcceptsAbsentOptionalGUID(t *testing.T) {
	const variantBytes = int(unsafe.Sizeof(evtVariant{}))
	buffer := make([]byte, len(selectedPaths)*variantBytes)
	variants := unsafe.Slice((*evtVariant)(unsafe.Pointer(&buffer[0])), len(selectedPaths))
	variants[0] = evtVariant{value: 1, typeID: evtVariantFileTime}
	variants[1] = evtVariant{value: 0, typeID: evtVariantByte}
	variants[2] = evtVariant{value: 0, typeID: evtVariantUInt16}
	variants[3] = evtVariant{typeID: evtVariantNull}
	variants[4] = evtVariant{value: 1, typeID: evtVariantUInt64}
	actual, err := parseValues(buffer)
	if err != nil || actual.hasGUID || actual.errs[3] != nil {
		t.Fatalf("values=%#v err=%v", actual, err)
	}
}

func TestParseValuesRejectsTruncatedBufferAndEscapedGUIDPointer(t *testing.T) {
	if _, err := parseValues(make([]byte, 3)); !errors.Is(err, errNativeFailed) {
		t.Fatalf("truncated=%v", err)
	}
	const variantBytes = int(unsafe.Sizeof(evtVariant{}))
	buffer := make([]byte, len(selectedPaths)*variantBytes)
	variants := unsafe.Slice((*evtVariant)(unsafe.Pointer(&buffer[0])), len(selectedPaths))
	variants[3] = evtVariant{value: uintptr(unsafe.Pointer(&buffer[len(buffer)-1])), typeID: evtVariantGUID}
	actual, err := parseValues(buffer)
	if err != nil || !errors.Is(actual.errs[3], eventreader.ErrFieldInvalid) {
		t.Fatalf("values=%#v err=%v", actual, err)
	}
}

func TestNativeErrorClassificationIsClosed(t *testing.T) {
	cases := []struct {
		native   syscall.Errno
		expected error
	}{
		{syscall.Errno(5), errNativePermission},
		{errorEventChannelMissing, errNativeUnavailable},
		{syscall.Errno(2), errNativeUnavailable},
		{syscall.Errno(3), errNativeUnavailable},
		{errorNotFound, errNativeNotFound},
		{errorEventQueryStale, errNativeFailed},
		{syscall.Errno(1460), errNativeFailed},
	}
	for _, test := range cases {
		if actual := classifySyscall(test.native); !errors.Is(actual, test.expected) {
			t.Fatalf("native=%d got=%v want=%v", test.native, actual, test.expected)
		}
	}
}

func TestUTF16ValidationRejectsUnpairedSurrogates(t *testing.T) {
	for _, value := range [][]uint16{{'a', 0xd83d, 0xde00}, {'a'}} {
		if !validUTF16(value) {
			t.Fatalf("rejected valid %#v", value)
		}
	}
	for _, value := range [][]uint16{{0xd83d}, {0xde00}, {0xd83d, 'x'}} {
		if validUTF16(value) {
			t.Fatalf("accepted invalid %#v", value)
		}
	}
}

func TestNarrowVariantsReadOnlyActiveUnionMember(t *testing.T) {
	// Native C unions do not define inactive upper bytes as part of ByteVal or
	// UInt16Val. These are valid scalar values, not numeric overflows.
	level, err := optionalByte(evtVariant{value: uintptr(0xabcdef1234560104), typeID: evtVariantByte})
	if err != nil || level != 4 {
		t.Fatal("ByteVal read inactive upper union storage")
	}
	id, err := requiredUint16(evtVariant{value: uintptr(0xabcdef1234560037), typeID: evtVariantUInt16})
	if err != nil || id != 55 {
		t.Fatal("UInt16Val read inactive upper union storage")
	}
	for _, value := range []evtVariant{{value: 4, typeID: evtVariantUInt16}, {value: 4, typeID: evtVariantByte | evtVariantArray}} {
		if _, err := optionalByte(value); err != eventreader.ErrFieldInvalid {
			t.Fatal("wrong ByteVal type accepted")
		}
	}
	for _, value := range []evtVariant{{value: 55, typeID: evtVariantByte}, {value: 55, typeID: evtVariantUInt16 | evtVariantArray}} {
		if _, err := requiredUint16(value); err != eventreader.ErrFieldInvalid {
			t.Fatal("wrong UInt16Val type accepted")
		}
	}
}

func TestNextSyscallResultsCloseEveryFailureHandle(t *testing.T) {
	for _, test := range []struct {
		name          string
		result, event uintptr
		returned      uint32
		native        error
		want          error
		eof           bool
	}{
		{"permission-handle", 0, 42, 1, windows.ERROR_ACCESS_DENIED, errNativePermission, false},
		{"generic-handle", 0, 42, 1, syscall.Errno(1460), errNativeFailed, false},
		{"eof-handle", 0, 42, 1, errorNoMoreItems, errNativeFailed, false},
		{"eof-count-without-handle", 0, 0, 1, errorNoMoreItems, errNativeFailed, false},
		{"eof", 0, 0, 0, errorNoMoreItems, nil, true},
		{"success-wrong-count", 1, 42, 0, nil, errNativeFailed, false},
		{"success-null", 1, 0, 1, nil, errNativeFailed, false},
		{"success", 1, 42, 1, nil, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var closed []handle
			event, eof, err := nextResult(test.result, test.event, test.returned, test.native, func(value handle) { closed = append(closed, value) })
			if err != test.want || eof != test.eof {
				t.Fatal("syscall result classification mismatch")
			}
			if test.name == "success" {
				if event != 42 || len(closed) != 0 {
					t.Fatal("successful handle not transferred")
				}
				return
			}
			if event != 0 {
				t.Fatal("failed syscall leaked ownership to caller")
			}
			wantClosed := 0
			if test.event != 0 {
				wantClosed = 1
			}
			if len(closed) != wantClosed || (wantClosed == 1 && closed[0] != 42) {
				t.Fatal("returned failure handle not closed exactly once")
			}
		})
	}
}

func TestRenderSizeSyscallResultPreservesPermissionAndBounds(t *testing.T) {
	for _, test := range []struct {
		name            string
		result          uintptr
		needed, maximum uint32
		native          error
		want            error
	}{
		{"permission", 0, 0, 4096, windows.ERROR_ACCESS_DENIED, errNativePermission},
		{"unavailable", 0, 0, 4096, errorEventChannelMissing, errNativeUnavailable},
		{"generic", 0, 0, 4096, syscall.Errno(1460), errNativeFailed},
		{"unexpected-success", 1, 64, 4096, windows.ERROR_ACCESS_DENIED, errNativeFailed},
		{"zero", 0, 0, 4096, windows.ERROR_INSUFFICIENT_BUFFER, errNativeFailed},
		{"exact", 0, 4096, 4096, windows.ERROR_INSUFFICIENT_BUFFER, nil},
		{"over", 0, 4097, 4096, windows.ERROR_INSUFFICIENT_BUFFER, eventreader.ErrFieldTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := renderSizeResult(test.result, test.needed, test.maximum, test.native); err != test.want {
				t.Fatal("render size result mapping mismatch")
			}
		})
	}
}
