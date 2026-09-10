package journalreader

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

func TestCumulativeNativeBudgetAdvancesHeavyBacklog(t *testing.T) {
	for _, validID := range []bool{false, true} {
		t.Run(fmt.Sprintf("captured-%t", validID), func(t *testing.T) { testHeavyBacklog(t, validID) })
	}
}

func testHeavyBacklog(t *testing.T, validID bool) {
	t.Helper()
	const totalRows = 200
	rows := make([]fakeRow, totalRows)
	for i := range rows {
		rows[i] = row(queryTime.Add(-time.Minute), i)
		prefix := []byte(fmt.Sprintf("synthetic-heavy-cursor-%d-", i))
		rows[i].cursor = append(prefix, bytes.Repeat([]byte("x"), logobs.MaxCheckpointBytes-len(prefix))...)
		rows[i].priority = bytes.Repeat([]byte("x"), logobs.MaxNativeFieldBytes)
		rows[i].id = bytes.Repeat([]byte("x"), logobs.MaxNativeFieldBytes)
		if validID {
			rows[i].id = []byte("0123456789abcdef0123456789abcdef")
		}
	}
	request := initialRequest()
	processed := 0
	for cycle := 0; cycle < 4; cycle++ {
		j := newJournal(rows...)
		reader, _ := New(Config{Factory: &fakeFactory{journal: j}, Now: func() time.Time { return request.QueryStartedAt }})
		batch, err := reader.Read(context.Background(), request)
		if err != nil || batch.Validate() != nil || j.closes != 1 {
			t.Fatal("heavy backlog read failed")
		}
		if j.nativeBytes > logobs.MaxSourceBytes {
			t.Fatal("cumulative native metadata budget exceeded")
		}
		known := len(batch.Events) + int(batch.DiscardedCount)
		if known == 0 || len(batch.NextOpaque) == 0 || (validID && batch.DiscardedCount != 0) || (!validID && len(batch.Events) != 0) {
			t.Fatal("heavy rows lost correctly classified resumable progress")
		}
		processed += known
		if processed > totalRows || !bytes.Equal(batch.NextOpaque, rows[processed-1].cursor) {
			t.Fatal("budget stop replayed/skipped records")
		}
		if batch.CaughtUp {
			if processed != totalRows || cycle == 0 {
				t.Fatal("heavy backlog falsely exhausted")
			}
			return
		}
		if reason(batch) != logobs.ReasonResponseTooLarge || batch.CollectionState != logobs.CollectionPartial || batch.Deferred {
			t.Fatal("budget stop claimed sentinel or wrong partial state")
		}
		if batch.ExaminedCount != uint32(known)+batch.ProbeCount || calls(j, "next") != int(batch.ExaminedCount) {
			t.Fatal("budget stop visited an unreserved row")
		}
		if j.nativeBytes+maxNativeVisitBytes <= logobs.MaxSourceBytes {
			t.Fatal("budget stop occurred despite full next-visit headroom")
		}
		previous := request.QueryStartedAt
		request.Checkpoint = logobs.Checkpoint{Revision: request.Checkpoint.Revision + 1, Opaque: batch.NextOpaque, PreviousAttemptAt: &previous}
		request.QueryStartedAt = previous.Add(time.Minute)
	}
	t.Fatal("heavy backlog made no eventual progress")
}

func TestNativeBudgetCountsProbesAndStopsWithoutPrefix(t *testing.T) {
	old := row(queryTime.Add(-time.Hour), 0)
	old.cursor = bytes.Repeat([]byte("x"), logobs.MaxCheckpointBytes)
	for _, request := range []logobs.ReadRequest{initialRequest(), resumeRequest(old.cursor, false), resumeRequest(nil, true)} {
		j := newJournal(old)
		batch := run(t, j, request)
		if batch.ProbeCount != 1 || j.nativeBytes != logobs.MaxCheckpointBytes {
			t.Fatal("probe cursor escaped byte accounting")
		}
	}
	a := attempt{nativeBytes: logobs.MaxSourceBytes - 1, supported: true, request: initialRequest(),
		batch: logobs.Batch{Kind: logobs.BatchNormal, StartedAt: queryTime}}
	if a.chargeNative(1) != nil || a.nativeBytes != logobs.MaxSourceBytes {
		t.Fatal("exact budget boundary rejected")
	}
	if a.chargeNative(1) != errNativeBudget || a.nativeBytes != logobs.MaxSourceBytes {
		t.Fatal("budget overflow mutated accounting")
	}
	a.fail(errNativeBudget)
	a.batch.FinishedAt = queryTime
	if a.batch.Validate() != nil || a.batch.CollectionState != logobs.CollectionFailed || reason(a.batch) != logobs.ReasonResponseTooLarge || len(a.batch.NextOpaque) != 0 {
		t.Fatal("no-prefix exhaustion advanced progress")
	}
}

func TestOversizedFieldSentinelDiscardsInsteadOfFallback(t *testing.T) {
	for _, field := range []string{"priority", "message-id"} {
		r := row(queryTime, 0)
		r.id = []byte("0123456789abcdef0123456789abcdef")
		r.errors = map[string]error{field: ErrFieldTooLarge}
		j := newJournal(r)
		batch := run(t, j, initialRequest())
		if !batch.CaughtUp || batch.DiscardedCount != 1 || len(batch.Events) != 0 || reason(batch) != logobs.ReasonInvalidResponse {
			t.Fatal("oversized field accepted fallback metadata")
		}
		if field == "priority" && calls(j, "message-id") != 0 {
			t.Fatal("oversized priority unnecessarily requested message ID")
		}
		if j.nativeBytes > logobs.MaxCheckpointBytes+8+1+32 {
			t.Fatal("oversized field sentinel returned a payload")
		}
	}
}

func TestInitialSeekCeilsSubmicrosecondLowerBound(t *testing.T) {
	for _, fraction := range []time.Duration{time.Nanosecond, 999 * time.Nanosecond, time.Microsecond, 999999999 * time.Nanosecond} {
		request := initialRequest()
		request.QueryStartedAt = queryTime.Add(fraction)
		lower := request.QueryStartedAt.Add(-5 * time.Minute)
		want := uint64(lower.UnixMicro())
		if lower.Nanosecond()%1000 != 0 {
			want++
		}
		before := row(time.UnixMicro(int64(want)-1).UTC(), 0)
		after := row(time.UnixMicro(int64(want)).UTC(), 1)
		j := newJournal(before, after)
		reader, _ := New(Config{Factory: &fakeFactory{journal: j}, Now: func() time.Time { return request.QueryStartedAt }})
		batch, err := reader.Read(context.Background(), request)
		if err != nil || batch.Validate() != nil || j.seekMicros != want || len(batch.Events) != 1 || batch.Events[0].ObservedAt.Before(lower) || batch.DiscardedCount != 0 {
			t.Fatal("initial lower bound rounded down or invented discard")
		}
		if !batch.CaughtUp || !bytes.Equal(batch.NextOpaque, after.cursor) || j.closes != 1 {
			t.Fatal("ceil seek lost valid in-window row")
		}
	}
}
