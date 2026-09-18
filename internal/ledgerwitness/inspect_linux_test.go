//go:build linux

package ledgerwitness

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/uploadledger"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

func fixture(t *testing.T) (string, *uploadledger.Ledger) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	l, err := uploadledger.Provision(context.Background(), dir, binding)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return dir, l
}

func TestInspectPreservesUsedStateAndFileIdentity(t *testing.T) {
	for _, state := range []string{"initial", "pending", "ack", "terminal", "exhausted"} {
		t.Run(state, func(t *testing.T) {
			dir, l := fixture(t)
			ctx := context.Background()
			want, err := l.Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if state != "initial" {
				sequence := int64(7)
				if state == "exhausted" {
					sequence = math.MaxInt64
				}
				p := pending(t, sequence, time.Date(2001, 1, 2, 3, 4, 5, 1, time.UTC))
				if err := l.Commit(ctx, want, p); err != nil {
					t.Fatal(err)
				}
				want = uploadstate.CloneRecord(p)
				switch state {
				case "ack", "exhausted":
					want.Pending = nil
					want.HasAck = true
					want.LastAck = p.Pending.Digest
					if state == "exhausted" {
						want.Stopped = uploadstate.Exhausted
					}
				case "terminal":
					want.Stopped = uploadstate.CredentialRejected
				}
				if state != "pending" {
					if err := l.Commit(ctx, p, want); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := l.Close(); err != nil {
				t.Fatal(err)
			}
			db := filepath.Join(dir, "upload.sqlite")
			beforeDir, err := os.Stat(dir)
			if err != nil {
				t.Fatal(err)
			}
			beforeDB, err := os.Stat(db)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := Fingerprint(want, binding)
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				got, err := inspect(ctx, dir, binding)
				if err != nil || got != expected {
					t.Fatalf("inspection failed: %v", err)
				}
			}
			afterDir, err := os.Stat(dir)
			if err != nil {
				t.Fatal(err)
			}
			afterDB, err := os.Stat(db)
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(beforeDir, afterDir) || !os.SameFile(beforeDB, afterDB) {
				t.Fatal("replaced existing ledger")
			}
			opened, err := uploadledger.OpenExisting(ctx, dir, binding)
			if err != nil {
				t.Fatal(err)
			}
			got, loadErr := opened.Load(ctx)
			closeErr := opened.Close()
			if loadErr != nil || closeErr != nil || !reflect.DeepEqual(got, want) {
				t.Fatal("logical state changed")
			}
		})
	}
}

func TestInspectRefusesMissingBusyForeignCorruptAndCanceled(t *testing.T) {
	ctx := context.Background()
	assertRejected := func(ctx context.Context, dir string, b uploadstate.Binding) {
		t.Helper()
		got, err := inspect(ctx, dir, b)
		if err != ErrUnsafe || got != [32]byte{} {
			t.Fatal("unsafe inspection yielded witness")
		}
	}
	empty := t.TempDir()
	if err := os.Chmod(empty, 0700); err != nil {
		t.Fatal(err)
	}
	assertRejected(ctx, empty, binding)
	entries, err := os.ReadDir(empty)
	if err != nil || len(entries) != 0 {
		t.Fatal("missing state created files")
	}
	dir, l := fixture(t)
	assertRejected(ctx, dir, binding)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	foreign := binding
	foreign.ConnectorID = "agent_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	assertRejected(ctx, dir, foreign)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	assertRejected(canceled, dir, binding)
	assertRejected(nil, dir, binding)
	if _, err := inspect(ctx, dir, binding); err != nil {
		t.Fatal("refusal damaged valid state")
	}
	// SQLite's persisted user_version is the big-endian header value at 60.
	databasePath := filepath.Join(dir, "upload.sqlite")
	database, err := os.ReadFile(databasePath)
	if err != nil || len(database) < 64 {
		t.Fatal("fixture header unavailable")
	}
	binary.BigEndian.PutUint32(database[60:64], 99)
	if err := os.WriteFile(databasePath, database, 0600); err != nil {
		t.Fatal(err)
	}
	assertRejected(ctx, dir, binding)
	if err := os.WriteFile(filepath.Join(dir, "upload.sqlite"), []byte("synthetic corrupt database"), 0600); err != nil {
		t.Fatal(err)
	}
	assertRejected(ctx, dir, binding)
}

type faultLedger struct {
	record            uploadstate.Record
	loadErr, closeErr error
	closed            bool
	cancel            context.CancelFunc
}

func (l *faultLedger) Load(context.Context) (uploadstate.Record, error) { return l.record, l.loadErr }
func (l *faultLedger) Close() error {
	l.closed = true
	if l.cancel != nil {
		l.cancel()
	}
	return l.closeErr
}

func TestInspectionClosesClearsAndRefusesLateFailures(t *testing.T) {
	for _, scenario := range []string{"load", "close", "cancel", "invalid"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			l := &faultLedger{record: pending(t, 1, time.Date(2001, 1, 2, 3, 4, 5, 0, time.UTC))}
			switch scenario {
			case "load":
				l.loadErr = errors.New("synthetic private load error")
			case "close":
				l.closeErr = errors.New("synthetic private close error")
			case "cancel":
				l.cancel = cancel
			case "invalid":
				l.record.Watermark = -1
			}
			got, err := inspectOpened(ctx, l, binding)
			if err != ErrUnsafe || got != [32]byte{} || !l.closed {
				t.Fatal("failure returned witness or leaked ledger")
			}
			for _, b := range l.record.Pending.Body {
				if b != 0 {
					t.Fatal("private snapshot not cleared")
				}
			}
		})
	}
}
