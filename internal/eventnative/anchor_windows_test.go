//go:build windows && (amd64 || arm64)

package eventnative

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/eventreader"
	"github.com/braidenm/home-lab-observer/internal/logobs"
)

func validAnchorFixture() anchor {
	return anchor{
		source: logobs.SourceSystem, flags: anchorTimePresent | anchorEventPresent | anchorGUIDPresent, recordID: 42, fileTime: 133_700_000_000_000_000,
		eventID: 1001, guid: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		bookmarkXML: "<BookmarkList><Bookmark Channel='System'/></BookmarkList>",
	}
}

func TestAnchorRoundTripAndCopy(t *testing.T) {
	expected := validAnchorFixture()
	encoded, err := encodeAnchor(expected)
	if err != nil || len(encoded) > logobs.MaxCheckpointBytes {
		t.Fatalf("encode: len=%d err=%v", len(encoded), err)
	}
	actual, err := decodeAnchor(encoded)
	if err != nil || actual != expected {
		t.Fatalf("decode: %#v err=%v", actual, err)
	}
	encoded[12] ^= 0xff
	if actual.recordID != expected.recordID {
		t.Fatal("decoded anchor borrowed input")
	}
}

func TestAnchorRejectsMalformedInputsWithoutCanary(t *testing.T) {
	valid, _ := encodeAnchor(validAnchorFixture())
	cases := map[string][]byte{
		"short":           valid[:20],
		"trailing":        append(append([]byte(nil), valid...), 0),
		"bad digest":      mutateAnchor(valid, func(value []byte) { value[12] ^= 1 }, false),
		"unknown source":  mutateAnchor(valid, func(value []byte) { value[8] = 9 }, true),
		"unknown flag":    mutateAnchor(valid, func(value []byte) { value[9] = 0x80 }, true),
		"reserved value":  mutateAnchor(valid, func(value []byte) { value[10] = 1 }, true),
		"reserved":        mutateAnchor(valid, func(value []byte) { value[11] = 1 }, true),
		"zero record":     mutateAnchor(valid, func(value []byte) { clear(value[12:20]) }, true),
		"length mismatch": mutateAnchor(valid, func(value []byte) { binary.BigEndian.PutUint32(value[46:50], 1) }, true),
		"nul xml":         mutateAnchor(valid, func(value []byte) { value[51] = 0 }, true),
		"invalid utf8":    mutateAnchor(valid, func(value []byte) { value[51] = 0xff }, true),
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := decodeAnchor(value)
			if !errors.Is(err, eventreader.ErrFieldInvalid) || strings.Contains(err.Error(), "BookmarkList") {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestAnchorOptionalGUIDClosureAndBound(t *testing.T) {
	value := validAnchorFixture()
	value.flags &^= anchorGUIDPresent
	if _, err := encodeAnchor(value); !errors.Is(err, eventreader.ErrFieldInvalid) {
		t.Fatalf("nonzero absent guid: %v", err)
	}
	value.guid = [16]byte{}
	value.bookmarkXML = strings.Repeat("x", logobs.MaxCheckpointBytes-anchorHeaderBytes-anchorDigestBytes)
	encoded, err := encodeAnchor(value)
	if err != nil || len(encoded) != logobs.MaxCheckpointBytes {
		t.Fatalf("exact bound: len=%d err=%v", len(encoded), err)
	}
	value.bookmarkXML += "x"
	if _, err := encodeAnchor(value); !errors.Is(err, eventreader.ErrFieldTooLarge) {
		t.Fatalf("over bound: %v", err)
	}
}

func TestAnchorMissingIdentityRequiresCanonicalZero(t *testing.T) {
	for name, mutations := range map[string][2]func(*anchor){
		"time": {
			func(value *anchor) { value.flags &^= anchorTimePresent },
			func(value *anchor) { value.fileTime = 0 },
		},
		"event": {
			func(value *anchor) { value.flags &^= anchorEventPresent },
			func(value *anchor) { value.eventID = 0 },
		},
		"guid": {
			func(value *anchor) { value.flags &^= anchorGUIDPresent },
			func(value *anchor) { value.guid = [16]byte{} },
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := validAnchorFixture()
			mutations[0](&value)
			if _, err := encodeAnchor(value); !errors.Is(err, eventreader.ErrFieldInvalid) {
				t.Fatalf("nonzero absent value: %v", err)
			}
			mutations[1](&value)
			if _, err := encodeAnchor(value); err != nil {
				t.Fatalf("canonical missing identity: %v", err)
			}
		})
	}
}

func TestAnchorMatchUsesSelectedTupleNotBookmarkXML(t *testing.T) {
	base := validAnchorFixture()
	otherXML := base
	otherXML.bookmarkXML = "<BookmarkList><Bookmark Channel='System' RecordId='42'/></BookmarkList>"
	if !anchorsMatch(base, otherXML) {
		t.Fatal("XML serialization must not define exactness")
	}
	mutations := []func(*anchor){
		func(a *anchor) { a.source = logobs.SourceApplication },
		func(a *anchor) { a.flags &^= anchorTimePresent; a.fileTime = 0 },
		func(a *anchor) { a.recordID++ },
		func(a *anchor) { a.fileTime++ },
		func(a *anchor) { a.eventID++ },
		func(a *anchor) { a.flags &^= anchorGUIDPresent; a.guid = [16]byte{} },
		func(a *anchor) { a.guid[0]++ },
	}
	for index, mutate := range mutations {
		candidate := base
		mutate(&candidate)
		if anchorsMatch(base, candidate) {
			t.Fatalf("mutation %d matched", index)
		}
	}
}

func mutateAnchor(input []byte, mutate func([]byte), resign bool) []byte {
	result := append([]byte(nil), input...)
	mutate(result)
	if resign {
		digestAt := len(result) - sha256.Size
		digest := sha256.Sum256(result[:digestAt])
		copy(result[digestAt:], digest[:])
	}
	return result
}
