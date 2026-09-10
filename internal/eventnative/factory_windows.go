//go:build windows && (amd64 || arm64)

package eventnative

import (
	"errors"
	"time"

	"github.com/braidenm/home-lab-observer/internal/eventreader"
	"github.com/braidenm/home-lab-observer/internal/logobs"
)

const (
	queryChannelPath      = uint32(0x1)
	queryForwardDirection = uint32(0x100)
	queryReverseDirection = uint32(0x200)
	seekRelativeBookmark  = uint32(0x4)
	seekStrict            = uint32(0x10000)
	maxRenderedValueBytes = uint32(4 << 10)
	maxBookmarkXMLBytes   = uint32(logobs.MaxCheckpointBytes - anchorHeaderBytes - anchorDigestBytes)
)

var selectedPaths = [...]string{
	"Event/System/TimeCreated/@SystemTime",
	"Event/System/Level",
	"Event/System/EventID",
	"Event/System/Provider/@Guid",
	"Event/System/EventRecordID",
}

type handle uintptr

type nativeValues struct {
	fileTime uint64
	level    uint8
	eventID  uint16
	guid     [16]byte
	hasGUID  bool
	recordID uint64
	errs     [5]error
}

type eventAPI interface {
	query(channel, expression string, flags uint32) (handle, error)
	renderContext(paths []string) (handle, error)
	next(query handle) (handle, bool, error)
	seek(query, bookmark handle, offset int64, timeout, flags uint32) error
	createBookmark(xml *string) (handle, error)
	updateBookmark(bookmark, event handle) error
	renderValues(context, event handle, maximum uint32) (nativeValues, error)
	renderBookmark(bookmark handle, maximum uint32) (string, error)
	close(handle)
}

type factory struct{ api eventAPI }

// NewFactory constructs the fixed local WEVTAPI binding. Loading is limited to
// wevtapi.dll through the Windows system-library search path.
func NewFactory() (eventreader.Factory, error) {
	api, err := newSystemAPI()
	if err != nil {
		return nil, eventreader.ErrUnavailable
	}
	return &factory{api: api}, nil
}

func (f *factory) OpenInitial(source logobs.Source, lowerFileTime uint64) (eventreader.Query, error) {
	at, ok := fileTimeToUTC(lowerFileTime)
	if !ok {
		return nil, eventreader.ErrInvalidRequest
	}
	// FILETIME precision is 100ns, so seven fixed decimal places are exact.
	expression := "*[System[TimeCreated[@SystemTime >= '" + at.Format("2006-01-02T15:04:05.0000000Z") + "']]]"
	return f.open(source, expression, queryForwardDirection)
}

func (f *factory) OpenContinuation(source logobs.Source) (eventreader.Query, error) {
	return f.open(source, "*", queryForwardDirection)
}

func (f *factory) OpenTail(source logobs.Source) (eventreader.Query, error) {
	return f.open(source, "*", queryReverseDirection)
}

func (f *factory) open(source logobs.Source, expression string, direction uint32) (eventreader.Query, error) {
	channel, ok := fixedChannel(source)
	if !ok {
		return nil, eventreader.ErrInvalidRequest
	}
	queryHandle, err := f.api.query(channel, expression, queryChannelPath|direction)
	if err != nil {
		if queryHandle != 0 {
			f.api.close(queryHandle)
		}
		return nil, mapNativeError(err)
	}
	if queryHandle == 0 {
		return nil, eventreader.ErrReadFailed
	}
	contextHandle, err := f.api.renderContext(selectedPaths[:])
	if err != nil {
		if contextHandle != 0 {
			f.api.close(contextHandle)
		}
		f.api.close(queryHandle)
		return nil, mapNativeError(err)
	}
	if contextHandle == 0 {
		f.api.close(queryHandle)
		return nil, eventreader.ErrReadFailed
	}
	return &query{api: f.api, source: source, query: queryHandle, context: contextHandle}, nil
}

type query struct {
	api     eventAPI
	source  logobs.Source
	query   handle
	context handle
	closed  bool
}

func (q *query) SeekBookmark(raw []byte) error {
	if q.closed {
		return eventreader.ErrReadFailed
	}
	value, err := decodeAnchor(raw)
	if err != nil || value.source != q.source {
		return eventreader.ErrReadFailed
	}
	bookmark, err := q.api.createBookmark(&value.bookmarkXML)
	if err != nil {
		if bookmark != 0 {
			q.api.close(bookmark)
		}
		return mapNativeError(err)
	}
	if bookmark == 0 {
		return eventreader.ErrReadFailed
	}
	defer q.api.close(bookmark)
	return mapBookmarkError(q.api.seek(q.query, bookmark, 0, 0, seekRelativeBookmark|seekStrict))
}

func (q *query) Next() (eventreader.Record, error) {
	if q.closed {
		return nil, eventreader.ErrReadFailed
	}
	event, exhausted, err := q.api.next(q.query)
	if exhausted {
		if event != 0 {
			q.api.close(event)
			return nil, eventreader.ErrReadFailed
		}
		return nil, nil
	}
	if event == 0 {
		if err != nil {
			return nil, mapNativeError(err)
		}
		return nil, eventreader.ErrReadFailed
	}
	record := &record{api: q.api, source: q.source, event: event}
	if err != nil {
		return record, mapNativeError(err)
	}
	values, renderErr := q.api.renderValues(q.context, event, maxRenderedValueBytes)
	record.values = values
	record.renderErr = mapNativeError(renderErr)
	return record, record.renderErr
}

func (q *query) Close() {
	if q.closed {
		return
	}
	q.closed = true
	if q.context != 0 {
		q.api.close(q.context)
		q.context = 0
	}
	if q.query != 0 {
		q.api.close(q.query)
		q.query = 0
	}
}

type record struct {
	api       eventAPI
	source    logobs.Source
	event     handle
	values    nativeValues
	renderErr error
	anchor    *anchor
	encoded   []byte
	closed    bool
}

func (r *record) TimeCreated() (uint64, error) { return r.values.fileTime, r.fieldError(0) }
func (r *record) Level() (uint8, error)        { return r.values.level, r.fieldError(1) }
func (r *record) EventID() (uint16, error)     { return r.values.eventID, r.fieldError(2) }
func (r *record) ProviderGUID() ([16]byte, error) {
	if r.renderErr != nil {
		return [16]byte{}, r.renderErr
	}
	if r.values.errs[3] != nil {
		return [16]byte{}, r.values.errs[3]
	}
	if !r.values.hasGUID {
		return [16]byte{}, eventreader.ErrFieldMissing
	}
	return r.values.guid, nil
}

func (r *record) fieldError(index int) error {
	if r.renderErr != nil {
		return r.renderErr
	}
	return r.values.errs[index]
}

func (r *record) Bookmark() ([]byte, error) {
	if r.closed || r.renderErr != nil {
		return nil, eventreader.ErrReadFailed
	}
	if r.encoded != nil {
		return append([]byte(nil), r.encoded...), nil
	}
	if err := r.fieldError(4); err != nil || r.values.recordID == 0 {
		return nil, eventreader.ErrFieldInvalid
	}
	flags := byte(0)
	fileTime := r.values.fileTime
	if err := r.fieldError(0); err == nil {
		flags |= anchorTimePresent
	} else if errors.Is(err, eventreader.ErrFieldMissing) {
		fileTime = 0
	} else {
		return nil, eventreader.ErrFieldInvalid
	}
	eventID := r.values.eventID
	if err := r.fieldError(2); err == nil {
		flags |= anchorEventPresent
	} else if errors.Is(err, eventreader.ErrFieldMissing) {
		eventID = 0
	} else {
		return nil, eventreader.ErrFieldInvalid
	}
	guid := r.values.guid
	if r.values.errs[3] != nil {
		if !errors.Is(r.values.errs[3], eventreader.ErrFieldMissing) {
			return nil, eventreader.ErrFieldInvalid
		}
		guid = [16]byte{}
	} else if r.values.hasGUID {
		flags |= anchorGUIDPresent
	} else {
		guid = [16]byte{}
	}
	bookmark, err := r.api.createBookmark(nil)
	if err != nil {
		if bookmark != 0 {
			r.api.close(bookmark)
		}
		return nil, mapNativeError(err)
	}
	if bookmark == 0 {
		return nil, eventreader.ErrReadFailed
	}
	defer r.api.close(bookmark)
	if err := r.api.updateBookmark(bookmark, r.event); err != nil {
		return nil, mapNativeError(err)
	}
	xml, err := r.api.renderBookmark(bookmark, maxBookmarkXMLBytes)
	if err != nil {
		return nil, mapNativeError(err)
	}
	value := anchor{source: r.source, flags: flags, recordID: r.values.recordID, fileTime: fileTime, eventID: eventID, guid: guid, bookmarkXML: xml}
	encoded, err := encodeAnchor(value)
	if err != nil {
		return nil, err
	}
	r.anchor = &value
	r.encoded = encoded
	return append([]byte(nil), encoded...), nil
}

func (r *record) BookmarkMatches(saved []byte) (bool, error) {
	if r.anchor == nil || r.encoded == nil {
		return false, eventreader.ErrReadFailed
	}
	value, err := decodeAnchor(saved)
	if err != nil {
		return false, eventreader.ErrReadFailed
	}
	return anchorsMatch(*r.anchor, value), nil
}

func (r *record) Close() {
	if r.closed {
		return
	}
	r.closed = true
	if r.event != 0 {
		r.api.close(r.event)
		r.event = 0
	}
}

func fixedChannel(source logobs.Source) (string, bool) {
	switch source {
	case logobs.SourceSystem:
		return "System", true
	case logobs.SourceApplication:
		return "Application", true
	default:
		return "", false
	}
}

func fileTimeToUTC(value uint64) (time.Time, bool) {
	const epochTicks = uint64(11644473600) * 10_000_000
	const maxInt64 = uint64(^uint64(0) >> 1)
	var seconds int64
	var nanos int64
	if value >= epochTicks {
		delta := value - epochTicks
		if delta/10_000_000 > maxInt64 {
			return time.Time{}, false
		}
		seconds = int64(delta / 10_000_000)
		nanos = int64(delta%10_000_000) * 100
	} else {
		delta := epochTicks - value
		whole := delta / 10_000_000
		remainder := delta % 10_000_000
		if whole > maxInt64 {
			return time.Time{}, false
		}
		seconds = -int64(whole)
		if remainder != 0 {
			seconds--
			nanos = int64(10_000_000-remainder) * 100
		}
	}
	at := time.Unix(seconds, nanos).UTC()
	return at, at.Year() >= 1 && at.Year() <= 9999
}

func mapBookmarkError(err error) error {
	if errors.Is(err, errNativeNotFound) {
		return eventreader.ErrBookmarkStale
	}
	return mapNativeError(err)
}

func mapNativeError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errNativePermission):
		return eventreader.ErrPermissionDenied
	case errors.Is(err, errNativeUnavailable):
		return eventreader.ErrUnavailable
	case errors.Is(err, eventreader.ErrFieldMissing), errors.Is(err, eventreader.ErrFieldInvalid), errors.Is(err, eventreader.ErrFieldTooLarge):
		return err
	default:
		return eventreader.ErrReadFailed
	}
}

var _ eventreader.Factory = (*factory)(nil)
var _ eventreader.Query = (*query)(nil)
var _ eventreader.Record = (*record)(nil)
