//go:build linux

package uploadledger

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

func largePending(t *testing.T, sequence int64) uploadstate.Record {
	t.Helper()
	r := pending(t, sequence, 0)
	cpu := observation.CPU{LogicalCPUs: 4096, UsagePercent: 99.99999999999999}
	memory := observation.Memory{TotalBytes: remoteprojection.MaxExactInteger, UsedBytes: remoteprojection.MaxExactInteger, SwapTotalBytes: remoteprojection.MaxExactInteger, SwapUsedBytes: remoteprojection.MaxExactInteger}
	uptime := observation.Uptime{Seconds: remoteprojection.MaxExactInteger}
	fs := make([]observation.Filesystem, remoteprojection.MaxFilesystems)
	for i := range fs {
		fs[i] = observation.Filesystem{TotalBytes: remoteprojection.MaxExactInteger, UsedBytes: remoteprojection.MaxExactInteger}
	}
	body, err := remoteprojection.Encode(observation.Snapshot{
		SchemaVersion: observation.SchemaVersion, ObservedAt: at,
		CPU:         observation.Section[observation.CPU]{State: observation.Available, Data: &cpu},
		Memory:      observation.Section[observation.Memory]{State: observation.Available, Data: &memory},
		Uptime:      observation.Section[observation.Uptime]{State: observation.Available, Data: &uptime},
		Filesystems: observation.Section[[]observation.Filesystem]{State: observation.Available, Data: &fs, Quality: observation.SectionQuality{Samples: len(fs), Total: len(fs)}},
	}, remoteprojection.Identity{SourceID: binding.ServerID, Version: "1234567890.1234567890.1234567890-preview", OS: "windows"})
	if err != nil {
		t.Fatal(err)
	}
	r.Pending.Body = body
	r.Pending.Digest = sha256.Sum256(body)
	return r
}

func TestRepeatedLargestProfileHasBoundedTransactionalFiles(t *testing.T) {
	l, dir := provision(t)
	var peak int64
	var sawJournal bool
	l.beforeCommit = func() error {
		if err := checkFiles(dir, true); err != nil {
			return err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		var total int64
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			total += info.Size()
			if entry.Name() == journalName && info.Size() > 0 {
				sawJournal = true
			}
		}
		if total > peak {
			peak = total
		}
		return nil
	}
	previous := load(t, l)
	for i := int64(1); i <= 100; i++ {
		next := largePending(t, i)
		if err := l.Commit(context.Background(), previous, next); err != nil {
			t.Fatal(err)
		}
		retired := uploadstate.Record{Binding: binding, Watermark: i}
		if i%2 == 0 {
			retired.HasAck = true
			retired.LastAck = next.Pending.Digest
		}
		if err := l.Commit(context.Background(), next, retired); err != nil {
			t.Fatal(err)
		}
		previous = retired
	}
	if !sawJournal || peak <= 0 || peak > maxTotalBytes {
		t.Fatal("transactional storage proof missing")
	}
	if l.configure(context.Background()) != nil {
		t.Fatal("configuration changed")
	}
}

func TestProcessDeathAndHotJournalRecovery(t *testing.T) {
	if phase := os.Getenv("OBSERVER_LEDGER_CRASH"); phase != "" {
		dir := os.Getenv("OBSERVER_LEDGER_DIR")
		l, err := OpenExisting(context.Background(), dir, binding)
		if err != nil {
			os.Exit(4)
		}
		initial := load(t, l)
		if phase == "hot" {
			l.Close()
			// Test-only hot-journal fixture: enable spill on a raw connection and
			// write a schema-valid oversized-for-profile body. Recovery must restore
			// the previous valid record. Production always disables spill.
			db, err := sql.Open("sqlite", dataSource(dir))
			if err != nil {
				os.Exit(4)
			}
			db.SetMaxOpenConns(1)
			for _, q := range []string{`PRAGMA cache_size=1`, `PRAGMA cache_spill=ON`, `BEGIN IMMEDIATE`} {
				if _, err := db.Exec(q); err != nil {
					os.Exit(4)
				}
			}
			next := pending(t, 1, 0)
			next.Pending.Body = bytes.Repeat([]byte("x"), 16384)
			if _, err := db.Exec(updateRecord, recordArgs(next)...); err != nil {
				os.Exit(4)
			}
			journal, err := os.ReadFile(filepath.Join(dir, journalName))
			magic := []byte{0xd9, 0xd5, 0x05, 0xf9, 0x20, 0xa1, 0x63, 0xd7}
			if err != nil || len(journal) < 8 || !bytes.Equal(journal[:8], magic) {
				os.Exit(5)
			}
			os.Exit(7)
		}
		if phase == "before" {
			l.beforeCommit = func() error { os.Exit(7); return nil }
		} else {
			l.afterCommit = func() error { os.Exit(7); return nil }
		}
		l.Commit(context.Background(), initial, pending(t, 1, 0))
		os.Exit(4)
	}
	for _, phase := range []string{"before", "after", "hot"} {
		t.Run(phase, func(t *testing.T) {
			l, dir := provision(t)
			initial := load(t, l)
			l.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProcessDeathAndHotJournalRecovery$")
			command.Env = []string{"OBSERVER_LEDGER_CRASH=" + phase, "OBSERVER_LEDGER_DIR=" + dir}
			var exit *exec.ExitError
			if err := command.Run(); !errors.As(err, &exit) || exit.ExitCode() != 7 {
				t.Fatal("owned crash fixture did not reach exact phase")
			}
			reopened, err := OpenExisting(context.Background(), dir, binding)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			want := initial
			if phase == "after" {
				want = pending(t, 1, 0)
			}
			if !sameRecord(load(t, reopened), want) {
				t.Fatal("crash recovery lost logical atomicity")
			}
		})
	}
}
