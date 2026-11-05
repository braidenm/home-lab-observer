//go:build linux

package actionjournal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

func TestV1SnapshotIsReadOnlyAndShowsJoinedIntentAcrossRestart(t *testing.T) {
	j, dir := provision(t)
	empty, err := j.AdmissionSnapshot(ctx)
	if err != nil || empty.Binding != owner || len(empty.Records) != 0 || empty.ObservedAt != epoch {
		t.Fatal("empty snapshot")
	}
	k := key(t, 1, 1)
	a := acquire(t, j, k)
	s, err := j.AdmissionSnapshot(ctx)
	if err != nil || len(s.Records) != 1 || s.Records[0].Key != k || !s.Records[0].Active || s.Records[0].HasIntent {
		t.Fatal("active admission snapshot")
	}
	if a.Intent(ctx) != nil {
		t.Fatal("intent fixture")
	}
	a.Close()
	if j.Close() != nil {
		t.Fatal("close fixture")
	}
	reopened, err := OpenExisting(ctx, dir, owner)
	if err != nil {
		t.Fatal("reopen fixture")
	}
	defer reopened.Close()
	reopened.now = func() time.Time { return epoch.Add(100 * 24 * time.Hour) }
	for range 3 {
		s, err = reopened.AdmissionSnapshot(ctx)
		if err != nil || len(s.Records) != 1 || s.Records[0].Key != k || s.Records[0].Active || !s.Records[0].HasIntent ||
			s.Records[0].Code != actioncore.OutcomeUnknown || !s.Records[0].TerminalAt.IsZero() {
			t.Fatal("retained unknown snapshot")
		}
	}
	var actions, events int
	if reopened.conn.QueryRowContext(ctx, "SELECT count(*) FROM actions").Scan(&actions) != nil ||
		reopened.conn.QueryRowContext(ctx, "SELECT count(*) FROM events").Scan(&events) != nil || actions != 1 || events != 1 {
		t.Fatal("snapshot created or expired evidence")
	}
}

func TestV1SnapshotLoadsExactlyBoundedMaximumAndRejectsExtraRow(t *testing.T) {
	j, _ := provision(t)
	// Fixture-only bulk seed avoids 1000 repeated broker journeys. Actual v1
	// metadata and two validated events per action still drive the real loader.
	seed := func(n int) error {
		k := key(t, n, n)
		raw, err := actioncore.CanonicalJournalMetadata(k)
		if err != nil {
			return err
		}
		if _, err = j.conn.ExecContext(ctx, `INSERT INTO actions VALUES(?,?,?,?,?,?,?)`, k.ActionID, k.TargetID,
			k.Metadata.EngineGeneration, k.Metadata.ContainerID, raw, k.Fingerprint[:], stamp(epoch)); err != nil {
			return err
		}
		for ordinal, code := range []string{"INTENT", "STARTED"} {
			if _, err = j.conn.ExecContext(ctx, `INSERT INTO events VALUES(?,?,?,?, '')`, k.ActionID, ordinal+1, stamp(epoch), code); err != nil {
				return err
			}
		}
		return nil
	}
	if err := j.transaction(ctx, func() error {
		for n := 1; n <= maxRecords; n++ {
			if err := seed(n); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal("bounded fixture seed")
	}
	s, err := j.AdmissionSnapshot(ctx)
	if err != nil || len(s.Records) != maxRecords {
		t.Fatal("maximum snapshot")
	}
	for index, record := range s.Records {
		if record.Key.ActionID != fmt.Sprintf("hla_%032x", index+1) || record.Code != actioncore.Started || record.Active {
			t.Fatal("maximum actual evidence")
		}
	}
	if err := j.transaction(ctx, func() error { return seed(maxRecords + 1) }); err != nil {
		t.Fatal("extra-row corruption fixture")
	}
	if _, err := j.AdmissionSnapshot(ctx); err == nil {
		t.Fatal("snapshot truncated extra evidence")
	}
}

func TestV1SnapshotReportsOnlyActualTerminalAndRefusesInvalidContext(t *testing.T) {
	j, _ := provision(t)
	a := acquire(t, j, key(t, 1, 1))
	if a.Intent(ctx) != nil || a.Finish(ctx, actioncore.Started) != nil {
		t.Fatal("terminal fixture")
	}
	a.Close()
	s, err := j.AdmissionSnapshot(ctx)
	if err != nil || len(s.Records) != 1 || s.Records[0].Code != actioncore.Started || s.Records[0].TerminalAt != epoch || s.Records[0].Active {
		t.Fatal("terminal evidence")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := j.AdmissionSnapshot(canceled); err == nil {
		t.Fatal("canceled snapshot")
	}
	if _, err := j.AdmissionSnapshot(nil); err == nil {
		t.Fatal("nil snapshot context")
	}
	if _, err := (*Journal)(nil).AdmissionSnapshot(ctx); err == nil {
		t.Fatal("nil store snapshot")
	}
}
