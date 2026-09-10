package eventreader

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

func TestSelectedFieldsNormalizeWithoutPrivateLeak(t *testing.T) {
	for _, tc := range []struct {
		level    uint8
		severity logobs.Severity
	}{{0, logobs.SeverityUnknown}, {1, logobs.SeverityCritical}, {2, logobs.SeverityError}, {3, logobs.SeverityWarn}, {4, logobs.SeverityInfo}, {5, logobs.SeverityTrace}, {255, logobs.SeverityUnknown}} {
		row := fixtureRecord(1)
		row.level = tc.level
		row.id = 65535
		row.guid = [16]byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}
		delete(row.fieldErrors, "guid")
		row.cursor = []byte("private-bookmark secret@example.invalid https://user:password@example.invalid")
		b, err := readFixture(t, &fakeFactory{initial: &fakeQuery{records: []*fakeRecord{row}}}, logobs.Checkpoint{})
		if err != nil || len(b.Events) != 1 || b.Events[0].Severity != tc.severity || b.Events[0].EventCode != "WIN_123456789abcdef01122334455667788_65535" {
			t.Fatalf("%+v %v", b, err)
		}
		encoded, _ := json.Marshal(b)
		if strings.Contains(string(encoded), "private-bookmark") || strings.Contains(string(encoded), "password") {
			t.Fatal("private canary serialized")
		}
	}
}

func TestSelectedFieldFailuresAreNotFabricatedSuccess(t *testing.T) {
	for _, field := range []string{"time", "level", "id", "guid"} {
		for _, fieldErr := range []error{ErrFieldMissing, ErrFieldInvalid, ErrFieldTooLarge, errors.New("private-native-error")} {
			row := fixtureRecord(1)
			row.fieldErrors[field] = fieldErr
			b, err := readFixture(t, &fakeFactory{initial: &fakeQuery{records: []*fakeRecord{row}}}, logobs.Checkpoint{})
			if err != nil || b.Validate() != nil {
				t.Fatalf("%s: %+v %v", field, b, err)
			}
			if fieldErr.Error() == "private-native-error" {
				if b.CollectionState != logobs.CollectionFailed || len(b.Events) != 0 || b.CaughtUp || b.ReasonCode == nil || *b.ReasonCode != logobs.ReasonReaderFailed {
					t.Fatalf("native error leaked/progress: %+v", b)
				}
				continue
			}
			discard := field == "time" || field == "id" || errors.Is(fieldErr, ErrFieldTooLarge)
			if discard {
				if b.DiscardedCount != 1 || len(b.Events) != 0 || !b.CaughtUp || b.CollectionState != logobs.CollectionPartial {
					t.Fatalf("discard %+v", b)
				}
			} else if len(b.Events) != 1 {
				t.Fatalf("fallback %+v", b)
			}
		}
	}
}

func TestFileTimeConversionAndFutureHorizon(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ticks uint64
		valid bool
	}{
		{"epoch", 0, true}, {"now", uint64(testNow.Unix()+fileTimeEpochOffset) * 10000000, true},
		{"horizon", uint64(testNow.Unix()+fileTimeEpochOffset+2) * 10000000, true},
		{"beyond-horizon", uint64(testNow.Unix()+fileTimeEpochOffset+2)*10000000 + 1, false},
		{"overflow", ^uint64(0), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := fixtureRecord(1)
			row.at = tc.ticks
			b, err := readFixture(t, &fakeFactory{initial: &fakeQuery{records: []*fakeRecord{row}}}, logobs.Checkpoint{})
			if err != nil {
				t.Fatal(err)
			}
			if tc.valid {
				if len(b.Events) != 1 {
					t.Fatalf("rejected time %+v", b)
				}
			} else if b.DiscardedCount != 1 || !b.Discards[0].At.Equal(testNow) {
				t.Fatalf("invalid time attribution %+v", b)
			}
		})
	}
}

func TestInitialLowerBoundCeilsFileTimeAndRejectsUnderflow(t *testing.T) {
	for _, ns := range []int{0, 1, 99, 100, 999999999} {
		now := testNow.Add(time.Duration(ns))
		f := &fakeFactory{initial: &fakeQuery{}, tail: &fakeQuery{}}
		r, _ := New(Config{Factory: f, Now: func() time.Time { return now }})
		if _, err := r.Read(context.Background(), logobs.ReadRequest{Source: logobs.SourceSystem, QueryStartedAt: now}); err != nil {
			t.Fatal(err)
		}
		lower := now.Add(-5 * time.Minute)
		rounded := time.Unix(int64(f.lower/10000000)-fileTimeEpochOffset, int64(f.lower%10000000)*100).UTC()
		if rounded.Before(lower) || rounded.Sub(lower) >= 100*time.Nanosecond {
			t.Fatalf("rounded %s below %s", rounded, lower)
		}
	}
	for _, source := range []logobs.Source{"Security", "system; command", ""} {
		f := &fakeFactory{}
		r, _ := New(Config{Factory: f})
		if _, err := r.Read(context.Background(), logobs.ReadRequest{Source: source, QueryStartedAt: testNow}); !errors.Is(err, ErrInvalidRequest) || len(f.calls) != 0 {
			t.Fatal("invalid source reached native")
		}
	}
	before := time.Date(1601, 1, 1, 0, 4, 59, 0, time.UTC)
	f := &fakeFactory{}
	r, _ := New(Config{Factory: f, Now: func() time.Time { return before }})
	if _, err := r.Read(context.Background(), logobs.ReadRequest{Source: logobs.SourceSystem, QueryStartedAt: before}); !errors.Is(err, ErrInvalidRequest) || len(f.calls) != 0 {
		t.Fatal("unsigned lower-bound wrap")
	}
}

func TestRowQuotaReservesProbeAndSentinel(t *testing.T) {
	for _, continuation := range []bool{false, true} {
		q := &fakeQuery{}
		for i := 0; i < 514; i++ {
			q.records = append(q.records, fixtureRecord(i))
		}
		f := &fakeFactory{initial: q, continuation: q}
		cp := logobs.Checkpoint{}
		if continuation {
			cp = logobs.Checkpoint{Revision: 1, Opaque: []byte("saved")}
		}
		b, err := readFixture(t, f, cp)
		maximum := 512
		if continuation {
			maximum = 511
		}
		if err != nil || b.Validate() != nil || b.ExaminedCount != 513 || !b.Deferred || b.CaughtUp || len(b.Events) != maximum {
			t.Fatalf("quota %+v %v", b, err)
		}
		if q.records[512].closed != 1 || len(q.records[512].reads) != 1 || q.records[513].closed != 0 {
			t.Fatal("sentinel read fields or exceeded bound")
		}
		if string(b.NextOpaque) != string(q.records[511].cursor) {
			t.Fatal("sentinel advanced cursor")
		}
	}
}

func TestNativeByteBudgetAdvancesHeavyBacklog(t *testing.T) {
	processed := 0
	for processed < 300 {
		q := &fakeQuery{}
		cp := logobs.Checkpoint{}
		if processed > 0 {
			probe := fixtureRecord(processed - 1)
			probe.cursor = []byte(strings.Repeat("p", logobs.MaxCheckpointBytes))
			q.records = append(q.records, probe)
			cp = logobs.Checkpoint{Revision: 1, Opaque: []byte("saved")}
		}
		for i := processed; i < 300; i++ {
			row := fixtureRecord(i)
			row.cursor = append(row.cursor, []byte(strings.Repeat("c", logobs.MaxCheckpointBytes-len(row.cursor)))...)
			q.records = append(q.records, row)
		}
		b, err := readFixture(t, &fakeFactory{initial: q, continuation: q}, cp)
		if err != nil || b.Validate() != nil || len(b.Events) == 0 {
			t.Fatalf("no progress %+v %v", b, err)
		}
		actual := int(b.ExaminedCount)*logobs.MaxCheckpointBytes + len(b.Events)*(8+1+2)
		if actual > logobs.MaxSourceBytes {
			t.Fatal("native byte bound exceeded")
		}
		processed += len(b.Events)
		if processed < 300 && (*b.ReasonCode != logobs.ReasonResponseTooLarge || b.Deferred || b.CaughtUp) {
			t.Fatal("wrong byte-bound state")
		}
		if b.CaughtUp && processed != 300 {
			t.Fatal("premature EOF")
		}
	}
	if processed != 300 {
		t.Fatal("replayed records")
	}
}

func TestBookmarkOversizeAndNativeFailureCloseEverything(t *testing.T) {
	for _, bad := range []func(*fakeRecord){func(r *fakeRecord) { r.cursor = make([]byte, logobs.MaxCheckpointBytes+1) }, func(r *fakeRecord) { r.errorBookmark = errors.New("private-bookmark-error") }} {
		row := fixtureRecord(1)
		bad(row)
		q := &fakeQuery{records: []*fakeRecord{row}}
		b, err := readFixture(t, &fakeFactory{initial: q}, logobs.Checkpoint{})
		if !errors.Is(err, ErrReadFailed) || len(b.Events) != 0 || q.closed != 1 || row.closed != 1 {
			t.Fatalf("%+v %v", b, err)
		}
	}
	row := fixtureRecord(1)
	q := &fakeQuery{records: []*fakeRecord{row}, nextErr: errors.New("private-next-error")}
	b, err := readFixture(t, &fakeFactory{initial: q}, logobs.Checkpoint{})
	if err != nil || b.CollectionState != logobs.CollectionFailed || q.closed != 1 || row.closed != 1 {
		t.Fatal("returned handle with error leaked")
	}
}

func TestQueryCloseCancellationDoesNotReturnSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	q := &fakeQuery{records: []*fakeRecord{fixtureRecord(1)}, onClose: cancel}
	r, _ := New(Config{Factory: &fakeFactory{initial: q}, Now: func() time.Time { return testNow }})
	if b, err := r.Read(ctx, logobs.ReadRequest{Source: logobs.SourceSystem, QueryStartedAt: testNow}); !errors.Is(err, context.Canceled) || len(b.Events) != 0 || q.closed != 1 {
		t.Fatalf("close cancellation escaped: %+v %v", b, err)
	}
}

func TestInitialTailMustBeOlderThanExactWindow(t *testing.T) {
	for _, tc := range []struct {
		name     string
		delta    int64
		fieldErr error
		success  bool
	}{
		{"older", -5*60*10000000 - 1, nil, true},
		{"exact-bound", -5 * 60 * 10000000, nil, false},
		{"new-arrival", 0, nil, false},
		{"missing", -6 * 60 * 10000000, ErrFieldMissing, false},
		{"oversized", -6 * 60 * 10000000, ErrFieldTooLarge, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := fixtureRecord(1)
			row.at = uint64(int64(row.at) + tc.delta)
			row.fieldErrors["time"] = tc.fieldErr
			b, err := readFixture(t, &fakeFactory{initial: &fakeQuery{}, tail: &fakeQuery{records: []*fakeRecord{row}}}, logobs.Checkpoint{})
			if err != nil || b.Validate() != nil {
				t.Fatalf("%+v %v", b, err)
			}
			if b.CaughtUp != tc.success || !tc.success && (len(b.NextOpaque) != 0 || b.DiscardedCount != 0 || b.CollectionState != logobs.CollectionFailed) {
				t.Fatalf("unproved initial tail: %+v", b)
			}
			if row.closed != 1 {
				t.Fatal("tail record leaked")
			}
		})
	}
}

func TestFailureMatrixUsesOnlyFixedReasons(t *testing.T) {
	for _, tc := range []struct {
		name       string
		open       bool
		cause      error
		support    logobs.SupportState
		collection logobs.CollectionState
		reason     logobs.ReasonCode
	}{
		{"permission", true, ErrPermissionDenied, logobs.SupportPermissionDenied, logobs.CollectionNotRun, logobs.ReasonPermissionDenied},
		{"unavailable", true, ErrUnavailable, logobs.SupportUnavailable, logobs.CollectionNotRun, logobs.ReasonLogHelperUnavailable},
		{"early-timeout", true, context.DeadlineExceeded, logobs.SupportUnavailable, logobs.CollectionFailed, logobs.ReasonReaderFailed},
		{"native-timeout", false, context.DeadlineExceeded, logobs.SupportSupported, logobs.CollectionFailed, logobs.ReasonDeadlineExceeded},
		{"native-private", false, errors.New("token-secret-user@example.invalid"), logobs.SupportSupported, logobs.CollectionFailed, logobs.ReasonReaderFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := &fakeQuery{}
			f := &fakeFactory{initial: q}
			if tc.open {
				f.openErr = tc.cause
			} else {
				q.nextErr = tc.cause
			}
			b, err := readFixture(t, f, logobs.Checkpoint{})
			if err != nil || b.Validate() != nil || b.SupportState != tc.support || b.CollectionState != tc.collection || b.ReasonCode == nil || *b.ReasonCode != tc.reason || len(b.NextOpaque) != 0 || q.closed != 1 {
				t.Fatalf("%+v %v", b, err)
			}
		})
	}
}

func TestMixedDiscardsAndCapturedRowsShareQuota(t *testing.T) {
	q := &fakeQuery{}
	for i := 0; i < 513; i++ {
		row := fixtureRecord(i)
		if i%2 == 0 {
			row.fieldErrors["id"] = ErrFieldMissing
		}
		q.records = append(q.records, row)
	}
	b, err := readFixture(t, &fakeFactory{initial: q}, logobs.Checkpoint{})
	if err != nil || b.Validate() != nil || len(b.Events) != 256 || b.DiscardedCount != 256 || b.ExaminedCount != 513 || !b.Deferred {
		t.Fatalf("%+v %v", b, err)
	}
}

func TestBudgetAndCancelledCallsDoNotVisitNative(t *testing.T) {
	q := &fakeQuery{}
	a := attempt{ctx: context.Background(), query: q, nativeBytes: logobs.MaxSourceBytes - maxNativeVisitBytes + 1}
	if err := a.ingest(); !errors.Is(err, errNativeBudget) || q.index != 0 {
		t.Fatal("visited outside reservation")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.ctx = ctx
	called := false
	if _, err := callValue(&a, func() (bool, error) { called = true; return true, nil }); !errors.Is(err, context.Canceled) || called {
		t.Fatal("called native after cancellation")
	}
}
