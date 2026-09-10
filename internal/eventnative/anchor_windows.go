//go:build windows && (amd64 || arm64)

// Package eventnative binds the fixed eventreader seam to the local Windows
// Event Log API. It does not expose channel names, query text, sessions, or
// native payloads to callers.
package eventnative

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"unicode/utf8"

	"github.com/braidenm/home-lab-observer/internal/eventreader"
	"github.com/braidenm/home-lab-observer/internal/logobs"
)

const (
	anchorHeaderBytes  = 50
	anchorDigestBytes  = sha256.Size
	anchorTimePresent  = byte(1 << 0)
	anchorEventPresent = byte(1 << 1)
	anchorGUIDPresent  = byte(1 << 2)
	anchorKnownFlags   = anchorTimePresent | anchorEventPresent | anchorGUIDPresent
)

var anchorMagic = [8]byte{'H', 'L', 'O', 'W', 'E', 'V', '1', 0}

type anchor struct {
	source      logobs.Source
	flags       byte
	recordID    uint64
	fileTime    uint64
	eventID     uint16
	guid        [16]byte
	bookmarkXML string
}

func encodeAnchor(value anchor) ([]byte, error) {
	if !validAnchor(value) {
		return nil, eventreader.ErrFieldInvalid
	}
	if len(value.bookmarkXML) > logobs.MaxCheckpointBytes-anchorHeaderBytes-anchorDigestBytes {
		return nil, eventreader.ErrFieldTooLarge
	}
	result := make([]byte, anchorHeaderBytes+len(value.bookmarkXML)+anchorDigestBytes)
	copy(result[:8], anchorMagic[:])
	source, ok := sourceByte(value.source)
	if !ok {
		return nil, eventreader.ErrFieldInvalid
	}
	result[8] = source
	result[9] = value.flags
	binary.BigEndian.PutUint64(result[12:20], value.recordID)
	binary.BigEndian.PutUint64(result[20:28], value.fileTime)
	binary.BigEndian.PutUint16(result[28:30], value.eventID)
	copy(result[30:46], value.guid[:])
	binary.BigEndian.PutUint32(result[46:50], uint32(len(value.bookmarkXML)))
	copy(result[50:], value.bookmarkXML)
	digestAt := len(result) - anchorDigestBytes
	digest := sha256.Sum256(result[:digestAt])
	copy(result[digestAt:], digest[:])
	return result, nil
}

func decodeAnchor(raw []byte) (anchor, error) {
	if len(raw) < anchorHeaderBytes+anchorDigestBytes || len(raw) > logobs.MaxCheckpointBytes {
		return anchor{}, eventreader.ErrFieldInvalid
	}
	digestAt := len(raw) - anchorDigestBytes
	digest := sha256.Sum256(raw[:digestAt])
	if subtle.ConstantTimeCompare(raw[digestAt:], digest[:]) != 1 || !equalMagic(raw[:8]) {
		return anchor{}, eventreader.ErrFieldInvalid
	}
	source, ok := byteSource(raw[8])
	if !ok || raw[9]&^anchorKnownFlags != 0 || raw[10] != 0 || raw[11] != 0 {
		return anchor{}, eventreader.ErrFieldInvalid
	}
	xmlBytes := int(binary.BigEndian.Uint32(raw[46:50]))
	if xmlBytes <= 0 || anchorHeaderBytes+xmlBytes != digestAt {
		return anchor{}, eventreader.ErrFieldInvalid
	}
	xml := raw[50 : 50+xmlBytes]
	if !utf8.Valid(xml) || containsNUL(xml) {
		return anchor{}, eventreader.ErrFieldInvalid
	}
	value := anchor{
		source: source, flags: raw[9],
		recordID:    binary.BigEndian.Uint64(raw[12:20]),
		fileTime:    binary.BigEndian.Uint64(raw[20:28]),
		eventID:     binary.BigEndian.Uint16(raw[28:30]),
		bookmarkXML: string(xml),
	}
	copy(value.guid[:], raw[30:46])
	if !validAnchor(value) {
		return anchor{}, eventreader.ErrFieldInvalid
	}
	return value, nil
}

func anchorsMatch(left, right anchor) bool {
	return left.source == right.source && left.flags == right.flags &&
		left.recordID == right.recordID && left.fileTime == right.fileTime &&
		left.eventID == right.eventID && left.guid == right.guid
}

func validAnchor(value anchor) bool {
	if value.recordID == 0 || value.flags&^anchorKnownFlags != 0 || value.bookmarkXML == "" ||
		!utf8.ValidString(value.bookmarkXML) || containsNUL([]byte(value.bookmarkXML)) {
		return false
	}
	if _, ok := sourceByte(value.source); !ok {
		return false
	}
	var zero [16]byte
	if value.flags&anchorTimePresent == 0 && value.fileTime != 0 {
		return false
	}
	if value.flags&anchorEventPresent == 0 && value.eventID != 0 {
		return false
	}
	return value.flags&anchorGUIDPresent != 0 || value.guid == zero
}

func sourceByte(source logobs.Source) (byte, bool) {
	switch source {
	case logobs.SourceSystem:
		return 1, true
	case logobs.SourceApplication:
		return 2, true
	default:
		return 0, false
	}
}

func byteSource(value byte) (logobs.Source, bool) {
	switch value {
	case 1:
		return logobs.SourceSystem, true
	case 2:
		return logobs.SourceApplication, true
	default:
		return "", false
	}
}

func equalMagic(value []byte) bool {
	return len(value) == len(anchorMagic) && subtle.ConstantTimeCompare(value, anchorMagic[:]) == 1
}

func containsNUL(value []byte) bool {
	for _, current := range value {
		if current == 0 {
			return true
		}
	}
	return false
}
