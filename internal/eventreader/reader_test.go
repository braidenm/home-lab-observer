package eventreader

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

var testNow = time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)

type fakeRecord struct {
	at                      uint64
	level                   uint8
	id                      uint16
	guid                    [16]byte
	cursor                  []byte
	exact                   bool
	fieldErrors             map[string]error
	matchErr, errorBookmark error
	closed                  int
	reads                   []string
	onClose                 func()
}

func (r *fakeRecord) TimeCreated() (uint64, error) {
	r.reads = append(r.reads, "time")
	return r.at, r.fieldErrors["time"]
}
func (r *fakeRecord) Level() (uint8, error) {
	r.reads = append(r.reads, "level")
	return r.level, r.fieldErrors["level"]
}
func (r *fakeRecord) EventID() (uint16, error) {
	r.reads = append(r.reads, "id")
	return r.id, r.fieldErrors["id"]
}
func (r *fakeRecord) ProviderGUID() ([16]byte, error) {
	r.reads = append(r.reads, "guid")
	return r.guid, r.fieldErrors["guid"]
}
func (r *fakeRecord) Bookmark() ([]byte, error) {
	r.reads = append(r.reads, "bookmark")
	return r.cursor, r.errorBookmark
}
func (r *fakeRecord) BookmarkMatches([]byte) (bool, error) {
	r.reads = append(r.reads, "match")
	return r.exact, r.matchErr
}
func (r *fakeRecord) Close() {
	r.closed++
	if r.onClose != nil {
		r.onClose()
	}
}

type fakeQuery struct {
	records                  []*fakeRecord
	index, closed, seekCalls int
	seekErr, nextErr         error
	nextHook                 func()
	saved                    []byte
	onClose                  func()
}

func (q *fakeQuery) SeekBookmark(saved []byte) error {
	q.seekCalls++
	q.saved = append([]byte(nil), saved...)
	return q.seekErr
}
func (q *fakeQuery) Next() (Record, error) {
	if q.nextHook != nil {
		q.nextHook()
	}
	if q.index >= len(q.records) {
		return nil, q.nextErr
	}
	r := q.records[q.index]
	q.index++
	return r, q.nextErr
}
func (q *fakeQuery) Close() {
	q.closed++
	if q.onClose != nil {
		q.onClose()
	}
}

type fakeFactory struct {
	initial, continuation, tail *fakeQuery
	calls                       []string
	sources                     []logobs.Source
	lower                       uint64
	openErr                     error
	openHook                    func()
}

func (f *fakeFactory) open(mode string, source logobs.Source, q *fakeQuery) (Query, error) {
	f.calls = append(f.calls, mode)
	f.sources = append(f.sources, source)
	if f.openHook != nil {
		f.openHook()
	}
	if q == nil {
		return nil, f.openErr
	}
	return q, f.openErr
}
func (f *fakeFactory) OpenInitial(s logobs.Source, lower uint64) (Query, error) {
	f.lower = lower
	return f.open("initial", s, f.initial)
}
func (f *fakeFactory) OpenContinuation(s logobs.Source) (Query, error) {
	return f.open("continuation", s, f.continuation)
}
func (f *fakeFactory) OpenTail(s logobs.Source) (Query, error) { return f.open("tail", s, f.tail) }

func fixtureRecord(index int) *fakeRecord {
	return &fakeRecord{at: uint64(testNow.Unix()+11644473600) * 10000000, level: 2, id: 9, cursor: []byte(fmt.Sprintf("synthetic-bookmark-%d", index)), exact: true, fieldErrors: map[string]error{"guid": ErrFieldMissing}}
}
func readFixture(t *testing.T, f *fakeFactory, cp logobs.Checkpoint) (logobs.Batch, error) {
	t.Helper()
	if cp.Revision > 0 && cp.PreviousAttemptAt == nil {
		prior := testNow.Add(-time.Minute)
		cp.PreviousAttemptAt = &prior
	}
	r, err := New(Config{Factory: f, Now: func() time.Time { return testNow }})
	if err != nil {
		t.Fatal(err)
	}
	return r.Read(context.Background(), logobs.ReadRequest{Source: logobs.SourceSystem, Checkpoint: cp, QueryStartedAt: testNow})
}

func TestInitialReadOwnsRowsAndUsesFixedSources(t *testing.T) {
	for _, source := range []logobs.Source{logobs.SourceSystem, logobs.SourceApplication} {
		row := fixtureRecord(1)
		q := &fakeQuery{records: []*fakeRecord{row}}
		f := &fakeFactory{initial: q}
		r, _ := New(Config{Factory: f, Now: func() time.Time { return testNow }})
		b, err := r.Read(context.Background(), logobs.ReadRequest{Source: source, QueryStartedAt: testNow})
		if err != nil || b.Validate() != nil || !b.CaughtUp || len(b.Events) != 1 || b.Events[0].EventCode != "WIN_9" || b.Events[0].Source != source {
			t.Fatalf("%+v %v", b, err)
		}
		if row.closed != 1 || q.closed != 1 || len(f.calls) != 1 || f.sources[0] != source {
			t.Fatal("wrong ownership or source")
		}
		row.cursor[0] = 'x'
		if b.NextOpaque[0] == 'x' {
			t.Fatal("aliased bookmark")
		}
	}
}

func TestContinuationExactnessAndResetProof(t *testing.T) {
	for _, tc := range []struct {
		name        string
		seek, match error
		exact       bool
		empty       bool
		reset, fail bool
	}{
		{"exact", nil, nil, true, false, false, false},
		{"nearest", nil, nil, false, false, true, false},
		{"reused-anchor", nil, nil, false, false, true, false},
		{"strict-stale", ErrBookmarkStale, nil, false, false, true, false},
		{"generic-stale", errors.New("ERROR_EVT_QUERY_RESULT_STALE private-canary"), nil, false, false, false, true},
		{"missing-probe", nil, nil, false, true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probe := fixtureRecord(1)
			probe.exact = tc.exact
			probe.matchErr = tc.match
			q := &fakeQuery{records: []*fakeRecord{probe}, seekErr: tc.seek}
			if tc.empty {
				q.records = nil
			}
			tailRow := fixtureRecord(9)
			tail := &fakeQuery{records: []*fakeRecord{tailRow}}
			f := &fakeFactory{continuation: q, tail: tail}
			b, err := readFixture(t, f, logobs.Checkpoint{Revision: 1, Opaque: []byte("saved-private-anchor")})
			if err != nil || b.Validate() != nil {
				t.Fatalf("%+v %v", b, err)
			}
			if tc.fail {
				if b.Kind != logobs.BatchNormal || b.CollectionState != logobs.CollectionFailed || len(f.calls) != 1 {
					t.Fatalf("generic failure invented reset: %+v", b)
				}
				return
			}
			if tc.reset {
				if b.Kind != logobs.BatchResetEstablished || b.CaughtUp || len(b.Events) != 0 || tail.index != 1 {
					t.Fatalf("reset %+v", b)
				}
			} else if !b.CaughtUp || b.ProbeCount != 1 || len(b.Events) != 0 {
				t.Fatalf("exact tail %+v", b)
			}
		})
	}
}

func TestEmptyWindowAndPendingResetNeedTailEvidence(t *testing.T) {
	for _, pending := range []bool{false, true} {
		for _, visible := range []bool{false, true} {
			tail := &fakeQuery{}
			if visible {
				tail.records = []*fakeRecord{fixtureRecord(1)}
				if !pending {
					tail.records[0].at -= 6 * 60 * 10000000
				}
			}
			f := &fakeFactory{initial: &fakeQuery{}, tail: tail}
			b, err := readFixture(t, f, logobs.Checkpoint{Revision: 1, ResetPending: pending})
			if err != nil || b.Validate() != nil {
				t.Fatalf("%+v %v", b, err)
			}
			if pending {
				if b.CaughtUp || len(f.calls) != 1 || f.calls[0] != "tail" {
					t.Fatal("pending attempted ingestion")
				}
				if visible != (b.Kind == logobs.BatchResetEstablished) {
					t.Fatal("wrong reset")
				}
			} else if visible != b.CaughtUp {
				t.Fatal("unproved initial zero")
			}
		}
	}
}

func TestVisitedBookmarkFailureRejectsPrefix(t *testing.T) {
	good, bad := fixtureRecord(1), fixtureRecord(2)
	bad.cursor = nil
	f := &fakeFactory{initial: &fakeQuery{records: []*fakeRecord{good, bad}}}
	b, err := readFixture(t, f, logobs.Checkpoint{})
	if !errors.Is(err, ErrReadFailed) || len(b.Events) != 0 || good.closed != 1 || bad.closed != 1 {
		t.Fatalf("%+v %v", b, err)
	}
}

func TestCancellationClosesReturnedHandles(t *testing.T) {
	for _, stage := range []string{"open", "next"} {
		ctx, cancel := context.WithCancel(context.Background())
		row := fixtureRecord(1)
		q := &fakeQuery{records: []*fakeRecord{row}}
		f := &fakeFactory{initial: q}
		if stage == "open" {
			f.openHook = cancel
		} else {
			q.nextHook = cancel
		}
		r, _ := New(Config{Factory: f, Now: func() time.Time { return testNow }})
		_, err := r.Read(ctx, logobs.ReadRequest{Source: logobs.SourceSystem, QueryStartedAt: testNow})
		if !errors.Is(err, context.Canceled) || q.closed != 1 || stage == "next" && row.closed != 1 {
			t.Fatalf("%s: %v close %d/%d", stage, err, q.closed, row.closed)
		}
	}
}
