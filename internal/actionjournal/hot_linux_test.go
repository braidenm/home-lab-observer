//go:build linux

package actionjournal

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

func TestRealHotJournalRecovery(t *testing.T) {
	if dir := os.Getenv("HLO_ACTION_HOT_DIR"); dir != "" {
		j, e := OpenExisting(ctx, dir, owner)
		if e != nil {
			os.Exit(90)
		}
		j.now = func() time.Time { return epoch.Add(31 * 24 * time.Hour) }
		// Recovery-format fixture only: force spill so the production no-spill
		// adapter's real OpenExisting path must recover a genuine hot journal.
		if _, e = j.conn.ExecContext(ctx, `PRAGMA cache_size=1; PRAGMA cache_spill=ON`); e != nil {
			os.Exit(91)
		}
		j.beforeCommit = func() error {
			f, e := os.Open(filepath.Join(dir, journalName))
			if e != nil {
				os.Exit(92)
			}
			var magic [8]byte
			_, e = io.ReadFull(f, magic[:])
			f.Close()
			if e != nil || !bytes.Equal(magic[:], []byte{0xd9, 0xd5, 0x05, 0xf9, 0x20, 0xa1, 0x63, 0xd7}) {
				os.Exit(93)
			}
			os.Exit(23)
			return nil
		}
		j.Prune(ctx)
		os.Exit(94)
	}
	j, dir := provision(t)
	for n := 1; n <= 100; n++ {
		a := acquire(t, j, key(t, n, n))
		if a.Finish(ctx, actioncore.AlreadyRunning) != nil {
			t.Fatal("terminal fixture")
		}
		a.Close()
	}
	j.Close()
	binary, e := os.Executable()
	if e != nil {
		t.Fatal("executable")
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(bounded, binary, "-test.run=^TestRealHotJournalRecovery$", "-test.timeout=8s")
	cmd.Env = []string{"TMPDIR=" + os.TempDir(), "GOMAXPROCS=2", "HLO_ACTION_HOT_DIR=" + dir}
	var exit *exec.ExitError
	if e = cmd.Run(); !errors.As(e, &exit) || exit.ExitCode() != 23 {
		t.Fatal("real hot journal was not established")
	}
	if checkJournalHeader(dir) != nil {
		t.Fatal("legitimate hot envelope refused")
	}
	re, e := OpenExisting(ctx, dir, owner)
	if e != nil {
		t.Fatal("SQLite hot recovery failed")
	}
	defer re.Close()
	floor, e := re.floor(ctx)
	if e != nil || rowCount(t, re) != 100 || !floor.IsZero() {
		t.Fatal("hot rollback changed original evidence/floor")
	}
}
