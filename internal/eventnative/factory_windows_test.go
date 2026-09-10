//go:build windows && (amd64 || arm64)

package eventnative

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/eventreader"
	"github.com/braidenm/home-lab-observer/internal/logobs"
)

type queryCall struct {
	channel, expression string
	flags               uint32
}
type seekCall struct {
	query, bookmark handle
	offset          int64
	timeout, flags  uint32
}

type fakeAPI struct {
	queries       []queryCall
	paths         []string
	nextEvent     handle
	nextExhausted bool
	nextErr       error
	values        nativeValues
	renderErr     error
	bookmarkXML   string
	createdXML    *string
	seekRecorded  seekCall
	seekErr       error
	queryErr      error
	contextErr    error
	createErr     error
	updateErr     error
	bookmarkErr   error
	closed        []handle
	updated       [2]handle
}

func (f *fakeAPI) query(channel, expression string, flags uint32) (handle, error) {
	f.queries = append(f.queries, queryCall{channel, expression, flags})
	return 10, f.queryErr
}
func (f *fakeAPI) renderContext(paths []string) (handle, error) {
	f.paths = append([]string(nil), paths...)
	return 11, f.contextErr
}
func (f *fakeAPI) next(handle) (handle, bool, error) { return f.nextEvent, f.nextExhausted, f.nextErr }
func (f *fakeAPI) seek(query, bookmark handle, offset int64, timeout, flags uint32) error {
	f.seekRecorded = seekCall{query, bookmark, offset, timeout, flags}
	return f.seekErr
}
func (f *fakeAPI) createBookmark(xml *string) (handle, error) {
	if xml != nil {
		copied := *xml
		f.createdXML = &copied
	}
	return 20, f.createErr
}
func (f *fakeAPI) updateBookmark(bookmark, event handle) error {
	f.updated = [2]handle{bookmark, event}
	return f.updateErr
}
func (f *fakeAPI) renderValues(handle, handle, uint32) (nativeValues, error) {
	return f.values, f.renderErr
}
func (f *fakeAPI) renderBookmark(handle, uint32) (string, error) { return f.bookmarkXML, f.bookmarkErr }
func (f *fakeAPI) close(value handle)                            { f.closed = append(f.closed, value) }

func validNativeValues() nativeValues {
	return nativeValues{fileTime: 133_700_000_000_000_000, level: 4, eventID: 55, recordID: 99, hasGUID: true, guid: [16]byte{1, 2, 3}}
}

func TestFactoryUsesOnlyFixedChannelsQueriesAndPaths(t *testing.T) {
	api := &fakeAPI{}
	f := &factory{api: api}
	initial, err := f.OpenInitial(logobs.SourceSystem, 133_700_000_001_234_567)
	if err != nil {
		t.Fatal(err)
	}
	initial.Close()
	continuation, err := f.OpenContinuation(logobs.SourceApplication)
	if err != nil {
		t.Fatal(err)
	}
	continuation.Close()
	tail, err := f.OpenTail(logobs.SourceSystem)
	if err != nil {
		t.Fatal(err)
	}
	tail.Close()
	if len(api.queries) != 3 {
		t.Fatalf("queries=%#v", api.queries)
	}
	if got := api.queries[0]; got.channel != "System" || got.flags != queryChannelPath|queryForwardDirection || got.expression != "*[System[TimeCreated[@SystemTime >= '2024-09-05T08:53:20.1234567Z']]]" {
		t.Fatalf("initial=%#v", got)
	}
	if got := api.queries[1]; got != (queryCall{"Application", "*", queryChannelPath | queryForwardDirection}) {
		t.Fatalf("continuation=%#v", got)
	}
	if got := api.queries[2]; got != (queryCall{"System", "*", queryChannelPath | queryReverseDirection}) {
		t.Fatalf("tail=%#v", got)
	}
	if !reflect.DeepEqual(api.paths, selectedPaths[:]) {
		t.Fatalf("paths=%#v", api.paths)
	}
	for _, forbidden := range []string{"EventData", "UserData", "Provider/@Name", "Computer", "Security", "Execution"} {
		if strings.Contains(strings.Join(api.paths, "\n"), forbidden) {
			t.Fatalf("selected forbidden path %s", forbidden)
		}
	}
}

func TestFactoryRejectsUnknownSourceAndInvalidTimeBeforeNativeCall(t *testing.T) {
	api := &fakeAPI{}
	f := &factory{api: api}
	for _, open := range []func() (eventreader.Query, error){
		func() (eventreader.Query, error) { return f.OpenContinuation("other") },
		func() (eventreader.Query, error) { return f.OpenTail("other") },
		func() (eventreader.Query, error) { return f.OpenInitial(logobs.SourceSystem, ^uint64(0)) },
	} {
		if _, err := open(); !errors.Is(err, eventreader.ErrInvalidRequest) {
			t.Fatalf("got %v", err)
		}
	}
	if len(api.queries) != 0 {
		t.Fatalf("native calls=%#v", api.queries)
	}
}

func TestFactoryClosesQueryWhenRenderContextFails(t *testing.T) {
	api := &fakeAPI{contextErr: errors.New("canary-secret-path")}
	_, err := (&factory{api: api}).OpenContinuation(logobs.SourceSystem)
	if !errors.Is(err, eventreader.ErrReadFailed) || strings.Contains(err.Error(), "canary") {
		t.Fatalf("err=%v", err)
	}
	if !reflect.DeepEqual(api.closed, []handle{11, 10}) {
		t.Fatalf("closed=%#v", api.closed)
	}
}

func TestFactoryClosesEveryHandleReturnedWithAnError(t *testing.T) {
	api := &fakeAPI{queryErr: errors.New("private")}
	if _, err := (&factory{api: api}).OpenContinuation(logobs.SourceSystem); !errors.Is(err, eventreader.ErrReadFailed) {
		t.Fatalf("query error=%v", err)
	}
	if !reflect.DeepEqual(api.closed, []handle{10}) {
		t.Fatalf("query closed=%#v", api.closed)
	}
	api = &fakeAPI{contextErr: errors.New("private")}
	if _, err := (&factory{api: api}).OpenContinuation(logobs.SourceSystem); !errors.Is(err, eventreader.ErrReadFailed) {
		t.Fatalf("context error=%v", err)
	}
	if !reflect.DeepEqual(api.closed, []handle{11, 10}) {
		t.Fatalf("context closed=%#v", api.closed)
	}
}

func TestQuerySeekDecodesPrivateAnchorAndUsesStrictZeroOffset(t *testing.T) {
	value := validAnchorFixture()
	raw, _ := encodeAnchor(value)
	api := &fakeAPI{}
	q := &query{api: api, source: logobs.SourceSystem, query: 10, context: 11}
	if err := q.SeekBookmark(raw); err != nil {
		t.Fatal(err)
	}
	if api.createdXML == nil || *api.createdXML != value.bookmarkXML {
		t.Fatalf("xml=%v", api.createdXML)
	}
	if api.seekRecorded != (seekCall{10, 20, 0, 0, seekRelativeBookmark | seekStrict}) {
		t.Fatalf("seek=%#v", api.seekRecorded)
	}
	if !reflect.DeepEqual(api.closed, []handle{20}) {
		t.Fatalf("closed=%#v", api.closed)
	}
	other := value
	other.source = logobs.SourceApplication
	otherRaw, _ := encodeAnchor(other)
	if err := q.SeekBookmark(otherRaw); !errors.Is(err, eventreader.ErrReadFailed) {
		t.Fatalf("wrong source=%v", err)
	}
	if len(api.closed) != 1 {
		t.Fatal("wrong-source anchor reached native API")
	}
}

func TestQuerySeekMapsOnlyNotFoundToStale(t *testing.T) {
	raw, _ := encodeAnchor(validAnchorFixture())
	for name, nativeErr := range map[string]error{"not found": errNativeNotFound, "query stale": errNativeFailed, "permission": errNativePermission} {
		t.Run(name, func(t *testing.T) {
			api := &fakeAPI{seekErr: nativeErr}
			err := (&query{api: api, source: logobs.SourceSystem, query: 1}).SeekBookmark(raw)
			if name == "not found" && !errors.Is(err, eventreader.ErrBookmarkStale) {
				t.Fatalf("err=%v", err)
			}
			if name == "query stale" && !errors.Is(err, eventreader.ErrReadFailed) {
				t.Fatalf("err=%v", err)
			}
			if name == "permission" && !errors.Is(err, eventreader.ErrPermissionDenied) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestQuerySeekDoesNotTreatBookmarkDecodeFailureAsNativeStale(t *testing.T) {
	raw, _ := encodeAnchor(validAnchorFixture())
	api := &fakeAPI{createErr: errNativeNotFound}
	err := (&query{api: api, source: logobs.SourceSystem, query: 1}).SeekBookmark(raw)
	if !errors.Is(err, eventreader.ErrReadFailed) || errors.Is(err, eventreader.ErrBookmarkStale) {
		t.Fatalf("create bookmark error=%v", err)
	}
}

func TestQueryNextAndHandleOwnership(t *testing.T) {
	api := &fakeAPI{nextEvent: 30, values: validNativeValues(), bookmarkXML: "<BookmarkList/>"}
	q := &query{api: api, source: logobs.SourceSystem, query: 10, context: 11}
	recordValue, err := q.Next()
	if err != nil || recordValue == nil {
		t.Fatalf("next=%v err=%v", recordValue, err)
	}
	raw, err := recordValue.Bookmark()
	if err != nil {
		t.Fatal(err)
	}
	decoded, _ := decodeAnchor(raw)
	if decoded.recordID != 99 || decoded.eventID != 55 || decoded.flags != anchorTimePresent|anchorEventPresent|anchorGUIDPresent || decoded.bookmarkXML != "<BookmarkList/>" {
		t.Fatalf("anchor=%#v", decoded)
	}
	if api.updated != [2]handle{20, 30} {
		t.Fatalf("updated=%#v", api.updated)
	}
	if exact, err := recordValue.BookmarkMatches(raw); err != nil || !exact {
		t.Fatalf("exact=%v err=%v", exact, err)
	}
	callCount := len(api.closed)
	if exact, err := recordValue.BookmarkMatches(raw); err != nil || !exact || len(api.closed) != callCount {
		t.Fatal("match reacquired native data")
	}
	recordValue.Close()
	recordValue.Close()
	q.Close()
	q.Close()
	if !reflect.DeepEqual(api.closed, []handle{20, 30, 11, 10}) {
		t.Fatalf("closed=%#v", api.closed)
	}
}

func TestQueryNextExhaustionAndReturnedErrorRecord(t *testing.T) {
	exhausted := &fakeAPI{nextExhausted: true}
	q := &query{api: exhausted, query: 1}
	if record, err := q.Next(); record != nil || err != nil {
		t.Fatalf("exhausted record=%v err=%v", record, err)
	}
	failed := &fakeAPI{nextEvent: 8, nextErr: errors.New("private-native-error")}
	q = &query{api: failed, query: 1}
	record, err := q.Next()
	if record == nil || !errors.Is(err, eventreader.ErrReadFailed) || strings.Contains(err.Error(), "private") {
		t.Fatalf("record=%v err=%v", record, err)
	}
	record.Close()
	if !reflect.DeepEqual(failed.closed, []handle{8}) {
		t.Fatalf("closed=%#v", failed.closed)
	}
}

func TestRecordMissingLevelDoesNotAlterIdentityAnchor(t *testing.T) {
	values := validNativeValues()
	values.errs[1] = eventreader.ErrFieldMissing
	api := &fakeAPI{nextEvent: 30, values: values, bookmarkXML: "<BookmarkList/>"}
	q := &query{api: api, source: logobs.SourceSystem, query: 1, context: 2}
	r, err := q.Next()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := r.Bookmark()
	if err != nil {
		t.Fatal(err)
	}
	decoded, _ := decodeAnchor(raw)
	if decoded.flags != anchorTimePresent|anchorEventPresent|anchorGUIDPresent {
		t.Fatalf("flags=%d", decoded.flags)
	}
	// Missing selected identity stays explicit so the kernel can discard the
	// public row while safely advancing its private bookmark.
	values.errs[2] = eventreader.ErrFieldMissing
	api = &fakeAPI{nextEvent: 31, values: values, bookmarkXML: "<BookmarkList/>"}
	q = &query{api: api, source: logobs.SourceSystem, query: 1, context: 2}
	r, _ = q.Next()
	raw, err = r.Bookmark()
	if err != nil {
		t.Fatalf("missing identity: %v", err)
	}
	decoded, _ = decodeAnchor(raw)
	if decoded.flags&anchorEventPresent != 0 || decoded.eventID != 0 {
		t.Fatalf("missing event identity=%#v", decoded)
	}
}

func TestRecordMalformedIdentityFailsBeforeBookmarkUpdate(t *testing.T) {
	for _, field := range []int{0, 2, 3, 4} {
		values := validNativeValues()
		values.errs[field] = eventreader.ErrFieldInvalid
		api := &fakeAPI{nextEvent: 30, values: values, bookmarkXML: "<BookmarkList/>"}
		q := &query{api: api, source: logobs.SourceSystem, query: 1, context: 2}
		r, err := q.Next()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Bookmark(); !errors.Is(err, eventreader.ErrFieldInvalid) {
			t.Fatalf("field %d: %v", field, err)
		}
		if api.updated != [2]handle{} {
			t.Fatalf("field %d updated bookmark", field)
		}
	}
}

func TestFileTimeFormattingBoundary(t *testing.T) {
	epoch := uint64(11644473600) * 10_000_000
	at, ok := fileTimeToUTC(epoch)
	if !ok || !at.Equal(time.Unix(0, 0).UTC()) {
		t.Fatalf("at=%v ok=%v", at, ok)
	}
	before, ok := fileTimeToUTC(epoch - 1)
	if !ok || before.Format(time.RFC3339Nano) != "1969-12-31T23:59:59.9999999Z" {
		t.Fatalf("pre-Unix FILETIME=%v ok=%v", before, ok)
	}
	start, ok := fileTimeToUTC(0)
	if !ok || start.Format(time.RFC3339) != "1601-01-01T00:00:00Z" {
		t.Fatalf("FILETIME epoch=%v ok=%v", start, ok)
	}
}

func TestInitialQueryAcceptsFILETIMEEpoch(t *testing.T) {
	api := &fakeAPI{}
	query, err := (&factory{api: api}).OpenInitial(logobs.SourceSystem, 0)
	if err != nil {
		t.Fatal(err)
	}
	query.Close()
	if len(api.queries) != 1 || api.queries[0].expression != "*[System[TimeCreated[@SystemTime >= '1601-01-01T00:00:00.0000000Z']]]" {
		t.Fatalf("query=%#v", api.queries)
	}
}
