//go:build linux

package uploadledger

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

var binding = uploadstate.Binding{ServerID: "srv_0123456789abcdef0123456789abcdef", ConnectorID: "agent_0123456789abcdef0123456789abcdef"}
var at = time.Date(2026, 9, 11, 12, 0, 0, 1, time.UTC)
var syntheticError = errors.New("synthetic-private-error")

func directory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}
func provision(t *testing.T) (*Ledger, string) {
	t.Helper()
	dir := directory(t)
	l, err := Provision(context.Background(), dir, binding)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l, dir
}
func pending(t *testing.T, seq int64, offset time.Duration) uploadstate.Record {
	t.Helper()
	observed := at.Add(offset)
	body, err := remoteprojection.Encode(observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: observed}, remoteprojection.Identity{SourceID: binding.ServerID, Version: "0.1.0-preview.3", OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	return uploadstate.Record{Binding: binding, Watermark: seq, Pending: &uploadstate.Pending{Sequence: seq, Body: body, Digest: sha256.Sum256(body), CollectedAt: observed}}
}
func load(t *testing.T, l *Ledger) uploadstate.Record {
	t.Helper()
	r, err := l.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestProvisionRoundTripAndSettings(t *testing.T) {
	l, dir := provision(t)
	initial := load(t, l)
	if initial.Watermark != 0 {
		t.Fatal("initial watermark")
	}
	next := pending(t, math.MaxInt64, 0)
	if err := l.Commit(context.Background(), initial, next); err != nil {
		t.Fatal(err)
	}
	if err := l.configure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	l, err := OpenExisting(context.Background(), dir, binding)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	got := load(t, l)
	if !sameRecord(got, next) {
		t.Fatal("roundtrip mismatch")
	}
	got.Pending.Body[0] = '!'
	if !sameRecord(load(t, l), next) {
		t.Fatal("load aliases state")
	}
	ack := uploadstate.Record{Binding: binding, Watermark: math.MaxInt64, HasAck: true, LastAck: next.Pending.Digest, Stopped: uploadstate.Exhausted}
	if err := l.Commit(context.Background(), next, ack); err != nil {
		t.Fatal(err)
	}
	if !sameRecord(load(t, l), ack) {
		t.Fatal("terminal ack mismatch")
	}
}

func TestCASAndSequenceSafety(t *testing.T) {
	for _, name := range []string{"stale", "decrease", "replace", "reintroduce", "binding"} {
		t.Run(name, func(t *testing.T) {
			l, _ := provision(t)
			initial := load(t, l)
			first := pending(t, 1, 0)
			if err := l.Commit(context.Background(), initial, first); err != nil {
				t.Fatal(err)
			}
			expected, next := first, pending(t, 2, time.Second)
			switch name {
			case "stale":
				expected = initial
			case "decrease":
				next = initial
			case "replace":
				next = pending(t, 1, time.Second)
			case "reintroduce":
				retired := uploadstate.Record{Binding: binding, Watermark: 1}
				if err := l.Commit(context.Background(), first, retired); err != nil {
					t.Fatal(err)
				}
				expected, next = retired, first
			case "binding":
				next.Binding.ConnectorID = "agent_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			}
			if err := l.Commit(context.Background(), expected, next); err != ErrRecovery {
				t.Fatal("unsafe transition accepted")
			}
			if _, err := l.Load(context.Background()); err != ErrRecovery {
				t.Fatal("failure did not latch")
			}
		})
	}
}

func TestExactPendingMayBeRetained(t *testing.T) {
	l, _ := provision(t)
	initial := load(t, l)
	first := pending(t, 1, 0)
	if err := l.Commit(context.Background(), initial, first); err != nil {
		t.Fatal(err)
	}
	next := uploadstate.CloneRecord(first)
	next.Stopped = uploadstate.CredentialRejected
	if err := l.Commit(context.Background(), first, next); err != nil {
		t.Fatal(err)
	}
	if !sameRecord(load(t, l), next) {
		t.Fatal("terminal pending changed")
	}
}

func TestUncertainCommitLatchesAndReopenUsesActualState(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "before", true: "after"}[after], func(t *testing.T) {
			l, dir := provision(t)
			initial := load(t, l)
			next := pending(t, 1, 0)
			if after {
				l.afterCommit = func() error { return syntheticError }
			} else {
				l.beforeCommit = func() error { return syntheticError }
			}
			if err := l.Commit(context.Background(), initial, next); err != ErrRecovery {
				t.Fatal("uncertainty not fixed")
			}
			if err := l.Commit(context.Background(), initial, next); err != ErrRecovery {
				t.Fatal("failure not latched")
			}
			l.Close()
			reopened, err := OpenExisting(context.Background(), dir, binding)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			want := initial
			if after {
				want = next
			}
			if !sameRecord(load(t, reopened), want) {
				t.Fatal("wrong authoritative state")
			}
		})
	}
}

func TestMissingAndForeignStateNeverRecreated(t *testing.T) {
	for _, name := range []string{"missing", "empty", "corrupt", "no-row", "version", "binding", "extra-schema"} {
		t.Run(name, func(t *testing.T) {
			l, dir := provision(t)
			l.Close()
			path := filepath.Join(dir, databaseName)
			switch name {
			case "missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "empty":
				if err := os.Truncate(path, 0); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if err := os.WriteFile(path, []byte("synthetic-corruption"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "no-row", "version", "extra-schema":
				db, err := sql.Open("sqlite", dataSource(dir))
				if err != nil {
					t.Fatal(err)
				}
				statement := map[string]string{"no-row": "DELETE FROM upload_record", "version": "PRAGMA user_version=2", "extra-schema": "CREATE TABLE unexpected(x INTEGER)"}[name]
				if _, err := db.Exec(statement); err != nil {
					t.Fatal(err)
				}
				db.Close()
			}
			before, _ := os.ReadFile(path)
			b := binding
			if name == "binding" {
				b.ServerID = "srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			}
			if got, err := OpenExisting(context.Background(), dir, b); err == nil {
				got.Close()
				t.Fatal("invalid state accepted")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("invalid state repaired")
			}
			if name == "missing" {
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("missing database recreated")
				}
			}
		})
	}
}

func TestPrivateArtifactBounds(t *testing.T) {
	for _, name := range []string{"mode", "hardlink", "symlink", "oversize", "journal", "unknown", "directory-mode"} {
		t.Run(name, func(t *testing.T) {
			l, dir := provision(t)
			l.Close()
			path := filepath.Join(dir, databaseName)
			switch name {
			case "mode":
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, filepath.Join(directory(t), "linked")); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(directory(t), "absent"), path); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				if err := os.Truncate(path, maxDatabaseBytes+1); err != nil {
					t.Fatal(err)
				}
			case "journal":
				if err := os.WriteFile(filepath.Join(dir, journalName), make([]byte, maxTotalBytes), 0o600); err != nil {
					t.Fatal(err)
				}
			case "unknown":
				if err := os.WriteFile(filepath.Join(dir, "unexpected"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			case "directory-mode":
				if err := os.Chmod(dir, 0o750); err != nil {
					t.Fatal(err)
				}
			}
			if got, err := OpenExisting(context.Background(), dir, binding); err == nil {
				got.Close()
				t.Fatal("unsafe state accepted")
			}
		})
	}
}

func TestCanceledAndClosedOperations(t *testing.T) {
	l, _ := provision(t)
	initial := load(t, l)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.Commit(ctx, initial, pending(t, 1, 0)); err != ErrRecovery {
		t.Fatal("cancellation accepted")
	}
	if _, err := l.Load(context.Background()); err != ErrRecovery {
		t.Fatal("cancellation did not latch")
	}
	l.Close()
	if l.Close() != nil {
		t.Fatal("close not idempotent")
	}
	if _, err := l.Load(context.Background()); err != ErrRecovery {
		t.Fatal("closed ledger readable")
	}
}

func TestProcessLease(t *testing.T) {
	if os.Getenv("OBSERVER_LEDGER_CHILD") == "lease" {
		l, err := OpenExisting(context.Background(), os.Getenv("OBSERVER_LEDGER_DIR"), binding)
		if err == ErrBusy {
			os.Exit(0)
		}
		if l != nil {
			l.Close()
		}
		os.Exit(4)
	}
	l, dir := provision(t)
	defer l.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProcessLease$")
	command.Env = []string{"OBSERVER_LEDGER_CHILD=lease", "OBSERVER_LEDGER_DIR=" + dir}
	if err := command.Run(); err != nil {
		t.Fatal("second process obtained ownership")
	}
}

func TestDSNDoesNotInterpretPathOptions(t *testing.T) {
	dir := filepath.Join(directory(t), "state ?mode=memory&x=#%")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	l, err := Provision(context.Background(), dir, binding)
	if err != nil {
		t.Fatal(err)
	}
	l.Close()
	if info, err := os.Stat(filepath.Join(dir, databaseName)); err != nil || info.Size() == 0 {
		t.Fatal("not a real fixed database")
	}
}
