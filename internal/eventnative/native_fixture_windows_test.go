//go:build windows && (amd64 || arm64)

package eventnative

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/eventreader"
	"github.com/braidenm/home-lab-observer/internal/logobs"
	"golang.org/x/sys/windows"
)

const (
	fixtureEnabled = "OBSERVER_TEST_OWNED_WINDOWS_EVENT_FIXTURE"
	fixtureMarker  = "HLO_OWNED_WINDOWS_EVENT_FIXTURE_V1\n"
	queryFilePath  = uint32(0x2)
)

var fixtureProviderGUID = [16]byte{0x4f, 0x0a, 0x8e, 0xad, 0x52, 0x3c, 0x4f, 0x1d, 0x9d, 0x5e, 0x30, 0xd0, 0x3a, 0x30, 0xf8, 0x1b}

type fixtureFiles struct {
	systemBefore, systemAfter           string
	applicationBefore, applicationAfter string
}

func ownedFixtureFiles(t *testing.T) fixtureFiles {
	t.Helper()
	if os.Getenv(fixtureEnabled) != "1" {
		t.Skip("owned Windows native fixture is not enabled")
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("RUNNER_ENVIRONMENT") != "github-hosted" {
		t.Fatal("owned Windows native fixture requires an ephemeral GitHub-hosted runner")
	}
	root := canonicalDirectory(t, os.Getenv("OBSERVER_TEST_EVTX_ROOT"))
	runnerRoot := canonicalDirectory(t, os.Getenv("RUNNER_TEMP"))
	if !stringsEqualPath(filepath.Dir(root), runnerRoot) {
		t.Fatal("owned fixture root is not a dedicated RUNNER_TEMP child")
	}
	marker, err := os.ReadFile(filepath.Join(root, ".hlo-owned-windows-event-fixture-v1"))
	if err != nil || string(marker) != fixtureMarker {
		t.Fatal("owned fixture marker is missing or invalid")
	}
	result := fixtureFiles{
		systemBefore: filepath.Join(root, "system-before.evtx"), systemAfter: filepath.Join(root, "system-after.evtx"),
		applicationBefore: filepath.Join(root, "application-before.evtx"), applicationAfter: filepath.Join(root, "application-after.evtx"),
	}
	for _, filename := range []string{result.systemBefore, result.systemAfter, result.applicationBefore, result.applicationAfter} {
		info, err := os.Lstat(filename)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() < 1 || info.Size() > 2<<20 {
			t.Fatal("owned EVTX fixture is missing, linked, or outside its size bound")
		}
	}
	return result
}

func stringsEqualPath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func canonicalDirectory(t *testing.T, value string) string {
	t.Helper()
	if value == "" || !filepath.IsAbs(value) {
		t.Fatal("fixture directory must be absolute")
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(value))
	if err != nil {
		t.Fatal("fixture directory cannot be canonicalized")
	}
	info, err := os.Lstat(canonical)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("fixture directory is unsafe")
	}
	return canonical
}

type fileFactory struct {
	api         eventAPI
	system      string
	application string
}

func (f *fileFactory) OpenInitial(source logobs.Source, _ uint64) (eventreader.Query, error) {
	return f.open(source, queryForwardDirection)
}
func (f *fileFactory) OpenContinuation(source logobs.Source) (eventreader.Query, error) {
	return f.open(source, queryForwardDirection)
}
func (f *fileFactory) OpenTail(source logobs.Source) (eventreader.Query, error) {
	return f.open(source, queryReverseDirection)
}
func (f *fileFactory) open(source logobs.Source, direction uint32) (eventreader.Query, error) {
	filename := f.system
	if source == logobs.SourceApplication {
		filename = f.application
	} else if source != logobs.SourceSystem {
		return nil, eventreader.ErrInvalidRequest
	}
	queryHandle, err := f.api.query(filename, "*", queryFilePath|direction)
	if err != nil || queryHandle == 0 {
		if queryHandle != 0 {
			f.api.close(queryHandle)
		}
		if err == nil {
			err = errNativeFailed
		}
		return nil, mapNativeError(err)
	}
	contextHandle, err := f.api.renderContext(selectedPaths[:])
	if err != nil || contextHandle == 0 {
		if contextHandle != 0 {
			f.api.close(contextHandle)
		}
		f.api.close(queryHandle)
		if err == nil {
			err = errNativeFailed
		}
		return nil, mapNativeError(err)
	}
	return &query{api: f.api, source: source, query: queryHandle, context: contextHandle}, nil
}

type checkedAPI struct {
	t            *testing.T
	inner        eventAPI
	thread       uint32
	open         map[handle]bool
	allowedFiles map[string]string
	queryStages  map[handle]string
	eventStages  map[handle]string
	calls        int
}

func (a *checkedAPI) check() {
	a.t.Helper()
	runtime.Gosched()
	current := windows.GetCurrentThreadId()
	if a.thread == 0 {
		a.thread = current
	} else if current != a.thread {
		a.t.Fatal("native WEVTAPI handle lifetime changed OS threads")
	}
	a.calls++
}
func (a *checkedAPI) own(value handle, err error) (handle, error) {
	if value != 0 {
		a.open[value] = true
	}
	return value, err
}
func (a *checkedAPI) query(path, expression string, flags uint32) (handle, error) {
	a.check()
	stage, allowed := a.allowedFiles[path]
	if !allowed || expression != "*" || (flags != queryFilePath|queryForwardDirection && flags != queryFilePath|queryReverseDirection) {
		a.t.Fatal("native fixture query escaped its fixed private EVTX boundary")
	}
	value, err := a.own(a.inner.query(path, expression, flags))
	if value != 0 {
		a.queryStages[value] = stage
	}
	return value, err
}
func (a *checkedAPI) renderContext(paths []string) (handle, error) {
	a.check()
	if !reflect.DeepEqual(paths, selectedPaths[:]) {
		a.t.Fatal("native fixture did not use the exact selected-property context")
	}
	return a.own(a.inner.renderContext(paths))
}
func (a *checkedAPI) next(query handle) (handle, bool, error) {
	a.check()
	stage := a.queryStages[query]
	value, exhausted, err := a.inner.next(query)
	switch {
	case err != nil:
		markFixtureStage(a.t, stage+"-native-next-error")
	case exhausted:
		markFixtureStage(a.t, stage+"-native-next-eof")
	case value == 0:
		markFixtureStage(a.t, stage+"-native-next-invalid")
	default:
		markFixtureStage(a.t, stage+"-native-next-record")
	}
	if value != 0 {
		a.open[value] = true
		a.eventStages[value] = stage
	}
	return value, exhausted, err
}
func (a *checkedAPI) seek(query, bookmark handle, offset int64, timeout, flags uint32) error {
	a.check()
	return a.inner.seek(query, bookmark, offset, timeout, flags)
}
func (a *checkedAPI) createBookmark(xml *string) (handle, error) {
	a.check()
	return a.own(a.inner.createBookmark(xml))
}
func (a *checkedAPI) updateBookmark(bookmark, event handle) error {
	a.check()
	return a.inner.updateBookmark(bookmark, event)
}
func (a *checkedAPI) renderValues(context, event handle, maximum uint32) (nativeValues, error) {
	a.check()
	stage := a.eventStages[event]
	markFixtureStage(a.t, stage+"-render-start")
	values, err := a.inner.renderValues(context, event, maximum)
	if err != nil {
		markFixtureStage(a.t, stage+"-render-error")
	} else {
		markFixtureStage(a.t, stage+"-render-ok")
	}
	return values, err
}
func (a *checkedAPI) renderBookmark(bookmark handle, maximum uint32) (string, error) {
	a.check()
	return a.inner.renderBookmark(bookmark, maximum)
}
func (a *checkedAPI) close(value handle) {
	a.check()
	if value == 0 || !a.open[value] {
		a.t.Fatal("native fixture closed an unknown or already closed handle")
	}
	delete(a.open, value)
	delete(a.queryStages, value)
	delete(a.eventStages, value)
	a.inner.close(value)
}

func TestOwnedWindowsNativeFixture(t *testing.T) {
	files := ownedFixtureFiles(t)
	native, err := newSystemAPI()
	if err != nil {
		t.Fatal("WEVTAPI is unavailable on a supported fixture runner")
	}
	api := &checkedAPI{
		t: t, inner: native, open: make(map[handle]bool), queryStages: make(map[handle]string), eventStages: make(map[handle]string),
		allowedFiles: map[string]string{
			files.systemBefore: "system-before", files.systemAfter: "system-after",
			files.applicationBefore: "application-before", files.applicationAfter: "application-after",
		},
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	before := &fileFactory{api: api, system: files.systemBefore, application: files.applicationBefore}
	systemBookmark := assertFixtureSequence(t, before, logobs.SourceSystem, "system-before", []uint16{101, 102}, []uint8{4, 3})
	applicationBookmark := assertFixtureSequence(t, before, logobs.SourceApplication, "application-before", []uint16{201, 202}, []uint8{2, 1})
	markFixtureStage(t, "system-reverse-tail")
	assertReverseTail(t, before, logobs.SourceSystem, 102)
	markFixtureStage(t, "system-bookmark-roundtrip")
	assertBookmarkRoundTrip(t, before, logobs.SourceSystem, systemBookmark, 101)

	after := &fileFactory{api: api, system: files.systemAfter, application: files.applicationAfter}
	markFixtureStage(t, "system-reset")
	assertReset(t, after, logobs.SourceSystem, systemBookmark, 103)
	assertFixtureSequence(t, after, logobs.SourceApplication, "application-after", []uint16{203}, []uint8{4})
	markFixtureStage(t, "application-reset")
	assertReset(t, after, logobs.SourceApplication, applicationBookmark, 203)
	markFixtureStage(t, "handle-accounting")
	if len(api.open) != 0 || api.calls < 20 {
		t.Fatalf("native handle evidence incomplete: open=%d calls=%d", len(api.open), api.calls)
	}
}

func markFixtureStage(t *testing.T, stage string) {
	t.Helper()
	t.Log("owned-fixture-stage: " + stage)
}

func assertFixtureSequence(t *testing.T, factory *fileFactory, source logobs.Source, stagePrefix string, wantIDs []uint16, wantLevels []uint8) []byte {
	t.Helper()
	markFixtureStage(t, stagePrefix+"-query-open")
	queryValue, err := factory.OpenContinuation(source)
	if err != nil {
		t.Fatal(err)
	}
	defer queryValue.Close()
	var firstBookmark []byte
	for index, wantID := range wantIDs {
		markFixtureStage(t, stagePrefix+"-next")
		recordValue, err := queryValue.Next()
		if err != nil || recordValue == nil {
			if recordValue != nil {
				recordValue.Close()
			}
			t.Fatalf("fixture record %d unavailable: %v", index, err)
		}
		failSelected := func(message string) {
			recordValue.Close()
			t.Fatal(message)
		}
		markFixtureStage(t, stagePrefix+"-time-read")
		at, err := recordValue.TimeCreated()
		if err != nil {
			failSelected("fixture TimeCreated read failed")
		}
		markFixtureStage(t, stagePrefix+"-time-value")
		if at == 0 {
			failSelected("fixture TimeCreated value is invalid")
		}
		markFixtureStage(t, stagePrefix+"-level-read")
		level, err := recordValue.Level()
		if err != nil {
			failSelected("fixture Level read failed")
		}
		markFixtureStage(t, stagePrefix+"-level-value")
		if level != wantLevels[index] {
			failSelected("fixture Level value does not match")
		}
		markFixtureStage(t, stagePrefix+"-event-id-read")
		id, err := recordValue.EventID()
		if err != nil {
			failSelected("fixture EventID read failed")
		}
		markFixtureStage(t, stagePrefix+"-event-id-value")
		if id != wantID {
			failSelected("fixture EventID value does not match")
		}
		markFixtureStage(t, stagePrefix+"-provider-guid-read")
		guid, err := recordValue.ProviderGUID()
		if err != nil {
			failSelected("fixture ProviderGUID read failed")
		}
		markFixtureStage(t, stagePrefix+"-provider-guid-value")
		if guid != fixtureProviderGUID {
			failSelected("fixture ProviderGUID value does not match")
		}
		markFixtureStage(t, stagePrefix+"-bookmark-read")
		bookmark, err := recordValue.Bookmark()
		if err != nil {
			failSelected("fixture bookmark read failed")
		}
		markFixtureStage(t, stagePrefix+"-bookmark-decode")
		decoded, err := decodeAnchor(bookmark)
		if err != nil {
			failSelected("fixture bookmark decode failed")
		}
		markFixtureStage(t, stagePrefix+"-anchor-record-id")
		if decoded.recordID == 0 {
			failSelected("fixture bookmark record identifier is invalid")
		}
		markFixtureStage(t, stagePrefix+"-anchor-time")
		if decoded.fileTime != at {
			failSelected("fixture bookmark time does not match")
		}
		markFixtureStage(t, stagePrefix+"-anchor-event-id")
		if decoded.eventID != id {
			failSelected("fixture bookmark event identifier does not match")
		}
		markFixtureStage(t, stagePrefix+"-anchor-provider-guid")
		if decoded.guid != guid {
			failSelected("fixture bookmark provider identifier does not match")
		}
		markFixtureStage(t, stagePrefix+"-anchor-flags")
		if decoded.flags != anchorTimePresent|anchorEventPresent|anchorGUIDPresent {
			failSelected("fixture bookmark presence flags do not match")
		}
		if index == 0 {
			firstBookmark = append([]byte(nil), bookmark...)
		}
		recordValue.Close()
	}
	markFixtureStage(t, stagePrefix+"-forward-eof")
	extra, err := queryValue.Next()
	if err != nil || extra != nil {
		if extra != nil {
			extra.Close()
		}
		t.Fatalf("fixture query did not end exactly: record=%v err=%v", extra, err)
	}
	return firstBookmark
}

func assertReverseTail(t *testing.T, factory *fileFactory, source logobs.Source, wantID uint16) {
	t.Helper()
	queryValue, err := factory.OpenTail(source)
	if err != nil {
		t.Fatal(err)
	}
	recordValue, err := queryValue.Next()
	if err != nil || recordValue == nil {
		if recordValue != nil {
			recordValue.Close()
		}
		queryValue.Close()
		t.Fatalf("fixture tail unavailable: %v", err)
	}
	id, idErr := recordValue.EventID()
	recordValue.Close()
	queryValue.Close()
	if idErr != nil || id != wantID {
		t.Fatalf("fixture reverse tail=%d err=%v", id, idErr)
	}
}

func assertBookmarkRoundTrip(t *testing.T, factory *fileFactory, source logobs.Source, saved []byte, wantID uint16) {
	t.Helper()
	queryValue, err := factory.OpenContinuation(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := queryValue.SeekBookmark(saved); err != nil {
		queryValue.Close()
		t.Fatalf("strict bookmark seek failed: %v", err)
	}
	recordValue, err := queryValue.Next()
	if err != nil || recordValue == nil {
		if recordValue != nil {
			recordValue.Close()
		}
		queryValue.Close()
		t.Fatalf("bookmarked record unavailable: %v", err)
	}
	id, idErr := recordValue.EventID()
	_, bookmarkErr := recordValue.Bookmark()
	exact, matchErr := recordValue.BookmarkMatches(saved)
	recordValue.Close()
	queryValue.Close()
	if idErr != nil || id != wantID || bookmarkErr != nil || matchErr != nil || !exact {
		t.Fatal("strict bookmark did not restore the exact selected record")
	}
}

func assertReset(t *testing.T, factory *fileFactory, source logobs.Source, saved []byte, wantID uint16) {
	t.Helper()
	reader, err := eventreader.New(eventreader.Config{Factory: factory, Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Second)
	batch, err := reader.Read(context.Background(), logobs.ReadRequest{
		Source: source, QueryStartedAt: started,
		Checkpoint: logobs.Checkpoint{Revision: 1, Opaque: append([]byte(nil), saved...)},
	})
	if err != nil {
		t.Fatalf("native reset read failed: %v", err)
	}
	wantReason := logobs.ReasonCheckpointReset
	if batch.Kind != logobs.BatchResetEstablished || batch.SupportState != logobs.SupportSupported ||
		batch.CollectionState != logobs.CollectionPartial || !reflect.DeepEqual(batch.ReasonCode, &wantReason) ||
		batch.ExaminedCount != 1 || batch.ProbeCount != 1 || len(batch.NextOpaque) == 0 || len(batch.Events) != 0 ||
		batch.DiscardedCount != 0 || batch.CaughtUp {
		t.Fatalf("unexpected reset batch: kind=%s support=%s collection=%s examined=%d", batch.Kind, batch.SupportState, batch.CollectionState, batch.ExaminedCount)
	}
	decoded, decodeErr := decodeAnchor(batch.NextOpaque)
	if decodeErr != nil || decoded.eventID != wantID || decoded.source != source {
		t.Fatal("reset did not establish the owned post-clear tail")
	}
}

var _ eventreader.Factory = (*fileFactory)(nil)
var _ eventAPI = (*checkedAPI)(nil)
