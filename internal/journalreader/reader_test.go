package journalreader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

var queryTime = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

type fakeFactory struct {
	journal Journal
	err     error
	opens   int
	hook    func()
}

func (f *fakeFactory) OpenSystem() (Journal, error) {
	f.opens++
	if f.hook != nil {
		f.hook()
	}
	return f.journal, f.err
}

type fakeRow struct {
	at                   uint64
	priority, id, cursor []byte
	errors               map[string]error
}
type fakeJournal struct {
	rows        []fakeRow
	pos         int
	closes      int
	calls       []string
	faults      map[string]error
	hook        func(string)
	seekMicros  uint64
	seekCursor  []byte
	nativeBytes int
}

func newJournal(rows ...fakeRow) *fakeJournal { return &fakeJournal{rows: rows, pos: -1} }
func row(at time.Time, index int) fakeRow {
	return fakeRow{at: uint64(at.UnixMicro()), priority: []byte("6"), cursor: []byte(fmt.Sprintf("synthetic-cursor-%d", index))}
}
func (j *fakeJournal) call(name string) error {
	j.calls = append(j.calls, name)
	runtime.Gosched()
	if j.hook != nil {
		j.hook(name)
	}
	if err := j.faults[name]; err != nil {
		return err
	}
	if j.pos >= 0 && j.pos < len(j.rows) {
		return j.rows[j.pos].errors[name]
	}
	return nil
}
func (j *fakeJournal) SeekRealtime(microseconds uint64) error {
	if err := j.call("seek-realtime"); err != nil {
		return err
	}
	j.seekMicros = microseconds
	j.pos = len(j.rows) - 1
	for i, r := range j.rows {
		if r.at >= microseconds {
			j.pos = i - 1
			break
		}
	}
	return nil
}
func (j *fakeJournal) SeekCursor(cursor []byte) error {
	if err := j.call("seek-cursor"); err != nil {
		return err
	}
	j.seekCursor = append([]byte(nil), cursor...)
	j.pos = -1
	for i, r := range j.rows {
		if bytes.Equal(r.cursor, cursor) {
			j.pos = i - 1
			break
		}
	}
	return nil
}
func (j *fakeJournal) SeekTail() error {
	if err := j.call("seek-tail"); err != nil {
		return err
	}
	j.pos = len(j.rows)
	return nil
}
func (j *fakeJournal) Next() (bool, error) {
	if err := j.call("next"); err != nil {
		return false, err
	}
	j.pos++
	return j.pos < len(j.rows), nil
}
func (j *fakeJournal) Previous() (bool, error) {
	if err := j.call("previous"); err != nil {
		return false, err
	}
	j.pos--
	return j.pos >= 0, nil
}
func (j *fakeJournal) TestCursor(cursor []byte) (bool, error) {
	if err := j.call("test-cursor"); err != nil {
		return false, err
	}
	return bytes.Equal(j.rows[j.pos].cursor, cursor), nil
}
func (j *fakeJournal) RealtimeMicros() (uint64, error) {
	err := j.call("realtime")
	if err == nil {
		j.nativeBytes += 8
	}
	return j.rows[j.pos].at, err
}
func (j *fakeJournal) Priority() ([]byte, error) {
	err := j.call("priority")
	if err == nil {
		j.nativeBytes += len(j.rows[j.pos].priority)
	}
	if errors.Is(err, ErrFieldTooLarge) {
		return nil, err
	}
	return j.rows[j.pos].priority, err
}
func (j *fakeJournal) MessageID() ([]byte, error) {
	err := j.call("message-id")
	if err == nil && j.rows[j.pos].id == nil {
		err = ErrFieldMissing
	}
	if err == nil {
		j.nativeBytes += len(j.rows[j.pos].id)
	}
	if errors.Is(err, ErrFieldTooLarge) {
		return nil, err
	}
	return j.rows[j.pos].id, err
}
func (j *fakeJournal) Cursor() ([]byte, error) {
	err := j.call("cursor")
	if err == nil {
		j.nativeBytes += len(j.rows[j.pos].cursor)
	}
	return j.rows[j.pos].cursor, err
}
func (j *fakeJournal) Close() { j.closes++; _ = j.call("close") }

func initialRequest() logobs.ReadRequest {
	return logobs.ReadRequest{Source: logobs.SourceSystem, QueryStartedAt: queryTime}
}
func resumeRequest(cursor []byte, pending bool) logobs.ReadRequest {
	previous := queryTime.Add(-time.Minute)
	return logobs.ReadRequest{Source: logobs.SourceSystem, QueryStartedAt: queryTime,
		Checkpoint: logobs.Checkpoint{Revision: 7, ResetPending: pending, Opaque: cursor, PreviousAttemptAt: &previous}}
}
func run(t *testing.T, j *fakeJournal, request logobs.ReadRequest) logobs.Batch {
	t.Helper()
	factory := &fakeFactory{journal: j}
	reader, err := New(Config{Factory: factory, Now: func() time.Time { return queryTime.Add(time.Millisecond) }})
	if err != nil {
		t.Fatal("reader construction failed")
	}
	batch, err := reader.Read(context.Background(), request)
	if err != nil {
		t.Fatal("synthetic read failed")
	}
	if batch.Validate() != nil {
		t.Fatal("reader returned invalid batch")
	}
	if factory.opens != 1 || j.closes != 1 {
		t.Fatal("handle lifetime mismatch")
	}
	return batch
}
func reason(batch logobs.Batch) logobs.ReasonCode {
	if batch.ReasonCode == nil {
		return ""
	}
	return *batch.ReasonCode
}
func calls(j *fakeJournal, name string) int {
	count := 0
	for _, call := range j.calls {
		if call == name {
			count++
		}
	}
	return count
}

func TestInitialReadAndVisibleTail(t *testing.T) {
	j := newJournal(row(queryTime.Add(-10*time.Minute), 0), row(queryTime.Add(-time.Minute), 1))
	batch := run(t, j, initialRequest())
	if len(batch.Events) != 1 || batch.ExaminedCount != 1 || batch.ProbeCount != 0 || !batch.CaughtUp || batch.CollectionState != logobs.CollectionOK {
		t.Fatal("initial window incorrectly ingested")
	}
	if j.seekMicros != uint64(queryTime.Add(-5*time.Minute).UnixMicro()) {
		t.Fatal("initial seek window changed")
	}
	if calls(j, "seek-tail") != 0 {
		t.Fatal("nonempty initial read used a tail probe")
	}
	old := row(queryTime.Add(-time.Hour), 2)
	j = newJournal(old)
	batch = run(t, j, initialRequest())
	if len(batch.Events) != 0 || batch.ExaminedCount != 1 || batch.ProbeCount != 1 || !batch.CaughtUp || !bytes.Equal(batch.NextOpaque, old.cursor) {
		t.Fatal("visible tail failed to prove empty current window")
	}
	if calls(j, "realtime") != 0 || calls(j, "priority") != 0 || calls(j, "message-id") != 0 {
		t.Fatal("tail probe requested ingestion metadata")
	}
	j = newJournal()
	batch = run(t, j, initialRequest())
	if batch.CaughtUp || batch.SupportState != logobs.SupportUnavailable || reason(batch) != logobs.ReasonNoVisibleJournal || batch.ExaminedCount != 0 {
		t.Fatal("invisible journal reported a healthy zero")
	}
	request := resumeRequest(nil, false)
	j = newJournal(row(queryTime.Add(-time.Minute), 3))
	batch = run(t, j, request)
	if len(batch.Events) != 1 || calls(j, "seek-realtime") != 1 || calls(j, "seek-cursor") != 0 {
		t.Fatal("cursorless durable revision treated as reset")
	}
}

func TestExactContinuationAndStaleReset(t *testing.T) {
	old := row(queryTime.Add(-time.Hour), 0)
	fresh := row(queryTime.Add(-time.Minute), 1)
	j := newJournal(old, fresh)
	batch := run(t, j, resumeRequest(old.cursor, false))
	if len(batch.Events) != 1 || batch.ProbeCount != 1 || batch.ExaminedCount != 2 || !batch.CaughtUp || !bytes.Equal(batch.NextOpaque, fresh.cursor) {
		t.Fatal("exact continuation replayed or skipped a row")
	}
	if calls(j, "test-cursor") != 1 || calls(j, "seek-tail") != 0 {
		t.Fatal("exact cursor proof not used")
	}
	j = newJournal(old)
	batch = run(t, j, resumeRequest(old.cursor, false))
	if len(batch.Events) != 0 || batch.ProbeCount != 1 || !batch.CaughtUp || !bytes.Equal(batch.NextOpaque, old.cursor) {
		t.Fatal("empty continuation lost its cursor proof")
	}
	j = newJournal(old, fresh)
	batch = run(t, j, resumeRequest([]byte("synthetic-missing-cursor"), false))
	if batch.Kind != logobs.BatchResetEstablished || batch.ProbeCount != 2 || batch.ExaminedCount != 2 || batch.CaughtUp || len(batch.Events) != 0 || batch.DiscardedCount != 0 || !bytes.Equal(batch.NextOpaque, fresh.cursor) {
		t.Fatal("nearest cursor incorrectly treated as exact")
	}
	if calls(j, "priority") != 0 || calls(j, "message-id") != 0 {
		t.Fatal("reset read ingestion fields")
	}
	j = newJournal(fresh)
	j.faults = map[string]error{"seek-cursor": ErrInvalidCursor}
	batch = run(t, j, resumeRequest(old.cursor, false))
	if batch.Kind != logobs.BatchResetEstablished || batch.ProbeCount != 1 {
		t.Fatal("invalid cursor did not use same-attempt tail recovery")
	}
	j = newJournal()
	batch = run(t, j, resumeRequest(old.cursor, false))
	if batch.Kind != logobs.BatchResetPending || batch.ProbeCount != 0 || len(batch.NextOpaque) != 0 {
		t.Fatal("empty stale journal did not remain pending")
	}
	j = newJournal(fresh)
	batch = run(t, j, resumeRequest(nil, true))
	if batch.Kind != logobs.BatchResetEstablished || batch.ProbeCount != 1 || calls(j, "seek-cursor") != 0 || calls(j, "next") != 0 {
		t.Fatal("pending reset did more than tail proof")
	}
}

func TestNativeErrorsDoNotInventResetOrLeak(t *testing.T) {
	privateError := errors.New("SYNTHETIC_PRIVATE_NATIVE_ERROR")
	old := row(queryTime.Add(-time.Hour), 0)
	for _, operation := range []string{"seek-cursor", "next", "test-cursor"} {
		j := newJournal(old)
		j.faults = map[string]error{operation: privateError}
		batch := run(t, j, resumeRequest(old.cursor, false))
		if batch.Kind != logobs.BatchNormal || batch.CollectionState != logobs.CollectionFailed || reason(batch) != logobs.ReasonReaderFailed || calls(j, "seek-tail") != 0 {
			t.Fatal("native error invented stale cursor")
		}
		if len(batch.Events) != 0 || len(batch.NextOpaque) != 0 || batch.ExaminedCount != 0 {
			t.Fatal("failed attempt retained progress")
		}
	}
	j := newJournal(old)
	j.faults = map[string]error{"seek-tail": privateError}
	batch := run(t, j, resumeRequest(nil, true))
	if batch.Kind != logobs.BatchResetPending || reason(batch) != logobs.ReasonReaderFailed {
		t.Fatal("pending failure lost reset state")
	}
	for _, tc := range []struct {
		err     error
		support logobs.SupportState
		reason  logobs.ReasonCode
	}{
		{ErrUnavailable, logobs.SupportUnavailable, logobs.ReasonLogHelperUnavailable},
		{ErrPermissionDenied, logobs.SupportPermissionDenied, logobs.ReasonPermissionDenied},
		{privateError, logobs.SupportUnavailable, logobs.ReasonReaderFailed},
	} {
		factory := &fakeFactory{err: tc.err}
		reader, _ := New(Config{Factory: factory, Now: func() time.Time { return queryTime }})
		batch, err := reader.Read(context.Background(), initialRequest())
		if err != nil || batch.Validate() != nil || batch.SupportState != tc.support || reason(batch) != tc.reason {
			t.Fatal("open failure not classified safely")
		}
	}
}

func TestVisitBudgetsAndDeferredCursor(t *testing.T) {
	for _, resume := range []bool{false, true} {
		for _, discardAll := range []bool{false, true} {
			rows := make([]fakeRow, 520)
			for i := range rows {
				rows[i] = row(queryTime.Add(-time.Minute), i)
				if discardAll {
					rows[i].priority = nil
				}
			}
			j := newJournal(rows...)
			request := initialRequest()
			processed := 512
			probe := 0
			if resume {
				request = resumeRequest(rows[0].cursor, false)
				processed = 511
				probe = 1
			}
			batch := run(t, j, request)
			if batch.ExaminedCount != 513 || batch.ProbeCount != uint32(probe) || !batch.Deferred || batch.CaughtUp || reason(batch) != logobs.ReasonBacklogDeferred {
				t.Fatal("native visit budget not enforced")
			}
			if len(batch.Events)+int(batch.DiscardedCount) != processed || !bytes.Equal(batch.NextOpaque, rows[processed-1+probe].cursor) {
				t.Fatal("deferred sentinel counted or cursor advanced past prefix")
			}
			if calls(j, "cursor") != 513 || calls(j, "priority") != processed {
				t.Fatal("sentinel metadata acquired or visit bound exceeded")
			}
		}
	}
}

func TestUncheckpointableVisitRejectsWholeAttempt(t *testing.T) {
	for _, badCursor := range [][]byte{nil, bytes.Repeat([]byte("x"), logobs.MaxCheckpointBytes+1)} {
		rows := []fakeRow{row(queryTime.Add(-time.Minute), 0), row(queryTime.Add(-time.Second), 1)}
		rows[1].cursor = badCursor
		j := newJournal(rows...)
		factory := &fakeFactory{journal: j}
		reader, _ := New(Config{Factory: factory, Now: func() time.Time { return queryTime }})
		batch, err := reader.Read(context.Background(), initialRequest())
		if err != ErrReadFailed || !reflect.DeepEqual(batch, logobs.Batch{}) || j.closes != 1 {
			t.Fatal("uncheckpointable prefix escaped as usable progress")
		}
	}
	// This policy also applies to an otherwise deferred sentinel.
	rows := make([]fakeRow, 513)
	for i := range rows {
		rows[i] = row(queryTime.Add(-time.Minute), i)
	}
	rows[512].cursor = nil
	j := newJournal(rows...)
	reader, _ := New(Config{Factory: &fakeFactory{journal: j}, Now: func() time.Time { return queryTime }})
	batch, err := reader.Read(context.Background(), initialRequest())
	if err != ErrReadFailed || !reflect.DeepEqual(batch, logobs.Batch{}) || j.closes != 1 {
		t.Fatal("uncheckpointable sentinel accepted")
	}
}

func TestSelectedFieldMappingAndPrivacy(t *testing.T) {
	for _, tc := range []struct {
		priority, id string
		severity     logobs.Severity
		code         string
		discard      bool
	}{
		{"0", "", logobs.SeverityCritical, "SYSTEMD_PRIORITY_0", false},
		{"1", "", logobs.SeverityCritical, "SYSTEMD_PRIORITY_1", false},
		{"2", "", logobs.SeverityCritical, "SYSTEMD_PRIORITY_2", false},
		{"3", "", logobs.SeverityError, "SYSTEMD_PRIORITY_3", false},
		{"4", "", logobs.SeverityWarn, "SYSTEMD_PRIORITY_4", false},
		{"5", "", logobs.SeverityInfo, "SYSTEMD_PRIORITY_5", false},
		{"6", "", logobs.SeverityInfo, "SYSTEMD_PRIORITY_6", false},
		{"7", "", logobs.SeverityDebug, "SYSTEMD_PRIORITY_7", false},
		{"invalid", "0123456789abcdef0123456789abcdef", logobs.SeverityUnknown, "SYSTEMD_0123456789abcdef0123456789abcdef", false},
		{"", "", "", "", true},
		{"SYNTHETIC_PRIVATE_FIELD", "PRIVATE_MESSAGE_CANARY", "", "", true},
		{strings.Repeat("x", logobs.MaxNativeFieldBytes+1), "0123456789abcdef0123456789abcdef", "", "", true},
		{"6", strings.Repeat("x", logobs.MaxNativeFieldBytes+1), "", "", true},
	} {
		r := row(queryTime.Add(-time.Second), 0)
		r.priority = []byte(tc.priority)
		r.id = []byte(tc.id)
		batch := run(t, newJournal(r), initialRequest())
		if !batch.CaughtUp {
			t.Fatal("known malformed selected field prevented exhaustion proof")
		}
		if tc.discard {
			if len(batch.Events) != 0 || batch.DiscardedCount != 1 || reason(batch) != logobs.ReasonInvalidResponse {
				t.Fatal("malformed metadata not counted as known discard")
			}
		} else {
			if len(batch.Events) != 1 || batch.Events[0].Severity != tc.severity || batch.Events[0].EventCode != tc.code {
				t.Fatal("selected metadata mapping changed")
			}
		}
	}
}

func TestTimestampBoundsAndAttribution(t *testing.T) {
	old := row(queryTime.Add(-30*24*time.Hour), 0)
	recent := row(queryTime.Add(-8*24*time.Hour), 1)
	batch := run(t, newJournal(old, recent), resumeRequest(old.cursor, false))
	if len(batch.Events) != 1 || !batch.Events[0].ObservedAt.Equal(time.UnixMicro(int64(recent.at)).UTC()) {
		t.Fatal("old backlog timestamp clamped or discarded by reader")
	}
	for _, micros := range []uint64{^uint64(0), uint64(queryTime.Add(2*time.Second + time.Microsecond).UnixMicro())} {
		r := row(queryTime, 0)
		r.at = micros
		j := newJournal(r)
		batch = run(t, j, initialRequest())
		if len(batch.Events) != 0 || batch.DiscardedCount != 1 || !batch.Discards[0].At.Equal(queryTime) || !batch.CaughtUp {
			t.Fatal("invalid future timestamp escaped or wrong discard attribution")
		}
		if calls(j, "priority") != 0 || calls(j, "message-id") != 0 {
			t.Fatal("invalid timestamp acquired unnecessary fields")
		}
	}
	r := row(queryTime.Add(2*time.Second), 0)
	batch = run(t, newJournal(r), initialRequest())
	if len(batch.Events) != 1 || !batch.Events[0].ObservedAt.Equal(queryTime.Add(2*time.Second)) {
		t.Fatal("within-horizon native timestamp changed")
	}
	r = row(queryTime, 0)
	r.errors = map[string]error{"realtime": ErrFieldMissing}
	batch = run(t, newJournal(r), initialRequest())
	if batch.DiscardedCount != 1 || !batch.Discards[0].At.Equal(queryTime) {
		t.Fatal("missing timestamp was not attributed to attempt")
	}
}

func TestInvalidSourceAndCancellationDoNotLeakHandles(t *testing.T) {
	j := newJournal()
	factory := &fakeFactory{journal: j}
	reader, _ := New(Config{Factory: factory, Now: func() time.Time { return queryTime }})
	request := initialRequest()
	request.Source = logobs.SourceApplication
	if _, err := reader.Read(context.Background(), request); err != ErrInvalidRequest || factory.opens != 0 {
		t.Fatal("unsupported source opened journal")
	}
	request = initialRequest()
	request.QueryStartedAt = time.Unix(0, 0).UTC()
	if _, err := reader.Read(context.Background(), request); err != ErrInvalidRequest || factory.opens != 0 {
		t.Fatal("negative initial seek wrapped")
	}
	for _, phase := range []string{"before-open", "during-open", "during-read"} {
		ctx, cancel := context.WithCancel(context.Background())
		j = newJournal(row(queryTime, 0))
		factory = &fakeFactory{journal: j}
		switch phase {
		case "before-open":
			cancel()
		case "during-open":
			factory.hook = cancel
		case "during-read":
			j.hook = func(name string) {
				if name == "next" {
					cancel()
				}
			}
		}
		reader, _ = New(Config{Factory: factory, Now: func() time.Time { return queryTime }})
		batch, err := reader.Read(ctx, initialRequest())
		cancel()
		if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(batch, logobs.Batch{}) {
			t.Fatal("cancelled attempt returned usable progress")
		}
		if phase == "before-open" {
			if factory.opens != 0 || j.closes != 0 {
				t.Fatal("cancelled request called native factory")
			}
		} else if j.closes != 1 {
			t.Fatal("cancelled open/read leaked native handle")
		}
	}
}

func TestMetadataErrorsAndClockRollbackRejectTentativePrefix(t *testing.T) {
	for _, operation := range []string{"realtime", "priority", "message-id"} {
		rows := []fakeRow{row(queryTime.Add(-time.Minute), 0), row(queryTime.Add(-time.Second), 1)}
		rows[1].errors = map[string]error{operation: errors.New("SYNTHETIC_PRIVATE_NATIVE_FAILURE")}
		batch := run(t, newJournal(rows...), initialRequest())
		if batch.CollectionState != logobs.CollectionFailed || reason(batch) != logobs.ReasonReaderFailed || len(batch.Events) != 0 || len(batch.NextOpaque) != 0 || batch.ExaminedCount != 0 {
			t.Fatal("native metadata failure retained tentative prefix")
		}
	}
	j := newJournal(row(queryTime, 0))
	factory := &fakeFactory{journal: j}
	ticks := 0
	reader, _ := New(Config{Factory: factory, Now: func() time.Time {
		ticks++
		if ticks == 1 {
			return queryTime
		}
		return queryTime.Add(-time.Second)
	}})
	batch, err := reader.Read(context.Background(), initialRequest())
	if err != ErrReadFailed || !reflect.DeepEqual(batch, logobs.Batch{}) || j.closes != 1 {
		t.Fatal("clock rollback returned invalid timing or leaked handle")
	}
	j = newJournal(row(queryTime, 0))
	factory = &fakeFactory{journal: j}
	reader, _ = New(Config{Factory: factory, Now: func() time.Time { return queryTime.Add(-time.Second) }})
	if _, err = reader.Read(context.Background(), initialRequest()); err != ErrReadFailed || factory.opens != 0 {
		t.Fatal("pre-query clock opened native handle")
	}
}

func TestMaximumCursorAndReturnedCopies(t *testing.T) {
	r := row(queryTime, 0)
	r.cursor = bytes.Repeat([]byte("x"), logobs.MaxCheckpointBytes)
	r.priority = bytes.Repeat([]byte("x"), logobs.MaxNativeFieldBytes)
	r.id = []byte("0123456789abcdef0123456789abcdef")
	j := newJournal(r)
	batch := run(t, j, initialRequest())
	if len(batch.Events) != 1 || batch.Events[0].Severity != logobs.SeverityUnknown || len(batch.NextOpaque) != logobs.MaxCheckpointBytes {
		t.Fatal("exact field/cursor bounds rejected")
	}
	r.cursor[0] = 'z'
	r.id[0] = 'f'
	if batch.NextOpaque[0] != 'x' || batch.Events[0].EventCode != "SYSTEMD_0123456789abcdef0123456789abcdef" {
		t.Fatal("returned metadata aliases native buffers")
	}
}
