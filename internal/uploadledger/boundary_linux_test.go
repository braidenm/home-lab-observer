//go:build linux

package uploadledger

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

func TestExistingArtifactsAndPartialProvisionRefused(t *testing.T) {
	l, dir := provision(t)
	l.Close()
	before, err := os.ReadFile(filepath.Join(dir, databaseName))
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, err := Provision(context.Background(), dir, binding); err == nil {
		duplicate.Close()
		t.Fatal("reprovisioned existing state")
	}
	after, _ := os.ReadFile(filepath.Join(dir, databaseName))
	if string(before) != string(after) {
		t.Fatal("reprovision changed state")
	}
	partial := directory(t)
	if err := os.WriteFile(filepath.Join(partial, lockName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := OpenExisting(context.Background(), partial, binding); err == nil {
		got.Close()
		t.Fatal("partial state recreated")
	}
	if got, err := Provision(context.Background(), partial, binding); err == nil {
		got.Close()
		t.Fatal("partial provision retried")
	}
}

func TestUntrustedWritableAncestorRejected(t *testing.T) {
	parent := directory(t)
	dir := filepath.Join(parent, "state")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o777); err != nil {
		t.Fatal(err)
	}
	if l, err := Provision(context.Background(), dir, binding); err != ErrUnsafe {
		if l != nil {
			l.Close()
		}
		t.Fatal("writable ancestor accepted")
	}
	if err := os.Chmod(parent, 0o777|os.ModeSticky); err != nil {
		t.Fatal(err)
	}
	l, err := Provision(context.Background(), dir, binding)
	if err != nil {
		t.Fatal("trusted sticky ancestor refused")
	}
	l.Close()
}

func TestForeignHeaderRefusedBeforeSQL(t *testing.T) {
	for _, offset := range []int{16, 18, 19} {
		t.Run(map[int]string{16: "page-size", 18: "write-mode", 19: "read-mode"}[offset], func(t *testing.T) {
			l, dir := provision(t)
			l.Close()
			path := filepath.Join(dir, databaseName)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			data[offset] = 2
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if got, err := OpenExisting(context.Background(), dir, binding); err == nil {
				got.Close()
				t.Fatal("foreign header accepted")
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 2 {
				t.Fatal("foreign header created sidecars")
			}
		})
	}
}

func TestBusyCommitRollsBackBeforeRecovery(t *testing.T) {
	l, dir := provision(t)
	initial := load(t, l)
	other, err := sql.Open("sqlite", dataSource(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	tx, err := other.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := tx.QueryRow(`SELECT count(*) FROM upload_record`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := l.Commit(context.Background(), initial, pending(t, 1, 0)); err != ErrRecovery {
		t.Fatal("blocked commit did not fail closed")
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("busy timeout exceeded")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	other.Close()
	l.Close()
	reopened, err := OpenExisting(context.Background(), dir, binding)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !sameRecord(load(t, reopened), initial) {
		t.Fatal("failed commit changed state")
	}
}

func TestParallelCallFailsPromptlyWithoutChangingRecord(t *testing.T) {
	l, _ := provision(t)
	initial := load(t, l)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	l.beforeCommit = func() error { close(entered); <-release; return nil }
	go func() { done <- l.Commit(context.Background(), initial, pending(t, 1, 0)) }()
	<-entered
	_, err := l.Load(context.Background())
	close(release)
	if err != ErrBusy {
		t.Fatal("parallel call did not return busy")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if load(t, l).Watermark != 1 {
		t.Fatal("parallel call changed state")
	}
}

func TestStoredLogicalCorruptionRefused(t *testing.T) {
	for _, statement := range []string{
		`UPDATE upload_record SET has_ack=1`,
		`UPDATE upload_record SET last_ack=zeroblob(31)`,
		`UPDATE upload_record SET pending_sequence=1,pending_body=x'01',pending_digest=zeroblob(32),pending_at='2026-09-11T12:00:00Z',watermark=1`,
	} {
		l, dir := provision(t)
		l.Close()
		db, err := sql.Open("sqlite", dataSource(dir))
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		if _, err := db.Exec(`PRAGMA ignore_check_constraints=ON`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
		db.Close()
		if got, err := OpenExisting(context.Background(), dir, binding); err == nil {
			got.Close()
			t.Fatal("invalid logical record accepted")
		}
	}
}

func TestCancelledLoadLatches(t *testing.T) {
	l, _ := provision(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.Load(ctx); err != ErrRecovery {
		t.Fatal("cancelled load accepted")
	}
	if _, err := l.Load(context.Background()); err != ErrRecovery {
		t.Fatal("cancelled load did not latch")
	}
	if validChange(uploadstate.Record{Binding: binding}, uploadstate.Record{}, binding) {
		t.Fatal("invalid binding accepted")
	}
}
