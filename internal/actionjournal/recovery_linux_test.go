//go:build linux

package actionjournal

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

func TestCommitFailurePoisonsBeforeRelease(t *testing.T) {
	for _, after := range []bool{false, true} {
		for _, finish := range []bool{false, true} {
			t.Run(map[bool]string{false: "before", true: "after"}[after]+map[bool]string{false: "-intent", true: "-terminal"}[finish], func(t *testing.T) {
				j, dir := provision(t)
				k := key(t, 1, 1)
				a := acquire(t, j, k)
				if finish && a.Intent(ctx) != nil {
					t.Fatal("intent")
				}
				fault := func() error { return errors.New("private-fault-canary") }
				if after {
					j.afterCommit = fault
				} else {
					j.beforeCommit = fault
				}
				var e error
				if finish {
					e = a.Finish(ctx, actioncore.Started)
				} else {
					e = a.Intent(ctx)
				}
				if !errors.Is(e, ErrRecovery) || !j.failed {
					t.Fatal("uncertainty not poisoned")
				}
				if _, e = j.Acquire(ctx, key(t, 2, 2)); !errors.Is(e, ErrRecovery) {
					t.Fatal("poison released admission")
				}
				a.Close()
				j.Close()
				re, e := OpenExisting(ctx, dir, owner)
				if e != nil {
					t.Fatal("reopen after uncertainty")
				}
				defer re.Close()
				re.now = func() time.Time { return epoch }
				stored, e := re.read(ctx, k.ActionID)
				if e != nil {
					t.Fatal("recovered logical record")
				}
				expected := actioncore.Code("")
				if finish || after {
					expected = actioncore.OutcomeUnknown
				}
				if finish && after {
					expected = actioncore.Started
				}
				again := acquire(t, re, k)
				if again.Existing() != expected {
					t.Fatal("old-or-new evidence mismatch")
				}
				again.Close()
				_ = stored
			})
		}
	}
}

func TestCrashBoundaries(t *testing.T) {
	if phase := os.Getenv("HLO_ACTION_CRASH"); phase != "" {
		j, e := OpenExisting(ctx, os.Getenv("HLO_ACTION_DIR"), owner)
		if e != nil {
			os.Exit(90)
		}
		j.now = func() time.Time { return epoch }
		a, e := j.Acquire(ctx, key(t, 1, 1))
		if e != nil {
			os.Exit(91)
		}
		if phase == "before-intent" {
			j.beforeCommit = func() error { os.Exit(23); return nil }
			a.Intent(ctx)
		}
		if phase == "after-intent" {
			j.afterCommit = func() error { os.Exit(23); return nil }
			a.Intent(ctx)
		}
		if a.Intent(ctx) != nil {
			os.Exit(92)
		}
		if os.WriteFile(filepath.Join(filepath.Dir(j.dir), "mutation-count"), []byte{1}, 0600) != nil {
			os.Exit(93)
		}
		if phase == "before-terminal" {
			j.beforeCommit = func() error { os.Exit(23); return nil }
		} else {
			j.afterCommit = func() error { os.Exit(23); return nil }
		}
		a.Finish(ctx, actioncore.Started)
		os.Exit(94)
	}
	for _, phase := range []string{"before-intent", "after-intent", "before-terminal", "after-terminal"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "journal")
			os.Mkdir(dir, 0700)
			j, e := Provision(ctx, dir, owner)
			if e != nil {
				t.Fatal("provision")
			}
			j.Close()
			binary, e := os.Executable()
			if e != nil {
				t.Fatal("executable")
			}
			bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(bounded, binary, "-test.run=^TestCrashBoundaries$", "-test.timeout=8s")
			cmd.Env = []string{"TMPDIR=" + os.TempDir(), "GOMAXPROCS=2", "HLO_ACTION_CRASH=" + phase, "HLO_ACTION_DIR=" + dir}
			e = cmd.Run()
			var exit *exec.ExitError
			if !errors.As(e, &exit) || exit.ExitCode() != 23 {
				t.Fatal("child did not reach exact crash boundary")
			}
			re, e := OpenExisting(ctx, dir, owner)
			if e != nil {
				t.Fatal("recover crash")
			}
			defer re.Close()
			re.now = func() time.Time { return epoch }
			a := acquire(t, re, key(t, 1, 1))
			expected := actioncore.OutcomeUnknown
			if phase == "before-intent" {
				expected = ""
			}
			if phase == "after-terminal" {
				expected = actioncore.Started
			}
			if a.Existing() != expected {
				t.Fatal("crash recovered wrong phase")
			}
			if expected != "" && a.Intent(ctx) == nil {
				t.Fatal("recovered evidence redispatched")
			}
			a.Close()
			if phase == "before-terminal" || phase == "after-terminal" {
				raw, e := os.ReadFile(filepath.Join(root, "mutation-count"))
				if e != nil || len(raw) != 1 || raw[0] != 1 {
					t.Fatal("synthetic mutation count")
				}
			}
		})
	}
}

func TestUnsafeAndForeignStateRefusal(t *testing.T) {
	for _, kind := range []string{"mode", "hardlink", "symlink", "extra", "wal", "header", "binding", "schema", "corrupt-digest", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			j, dir := provision(t)
			a := acquire(t, j, key(t, 1, 1))
			a.Close()
			if kind == "schema" {
				j.conn.ExecContext(ctx, `CREATE TABLE foreign_state(value TEXT)`)
			}
			if kind == "corrupt-digest" {
				j.conn.ExecContext(ctx, `UPDATE actions SET digest=zeroblob(32)`)
			}
			j.Close()
			path := filepath.Join(dir, databaseName)
			switch kind {
			case "mode":
				os.Chmod(path, 0640)
			case "hardlink":
				os.Link(path, filepath.Join(filepath.Dir(dir), "linked-db"))
			case "symlink":
				os.Rename(path, path+".saved")
				os.Symlink(path+".saved", path)
			case "extra":
				os.WriteFile(filepath.Join(dir, "foreign"), nil, 0600)
			case "wal":
				os.WriteFile(filepath.Join(dir, "action.sqlite-wal"), nil, 0600)
			case "header":
				f, _ := os.OpenFile(path, os.O_WRONLY, 0)
				f.WriteAt([]byte{0, 0, 0, 0}, 68)
				f.Close()
			case "oversized":
				os.Truncate(path, databaseBytes+4096)
			}
			b := owner
			if kind == "binding" {
				b.OwnerID = "usr_00000000000000000000000000000000"
			}
			before, _ := os.ReadFile(path)
			if other, e := OpenExisting(ctx, dir, b); e == nil {
				other.Close()
				t.Fatal("unsafe/foreign store accepted")
			}
			after, _ := os.ReadFile(path)
			if sha256.Sum256(before) != sha256.Sum256(after) {
				t.Fatal("refusal rewrote fixture database")
			}
		})
	}
}

func TestExclusiveStoreAndConcurrentAttemptGuards(t *testing.T) {
	j, dir := provision(t)
	if other, e := OpenExisting(ctx, dir, owner); !errors.Is(e, ErrBusy) {
		if other != nil {
			other.Close()
		}
		t.Fatal("second owner admitted")
	}
	a := acquire(t, j, key(t, 1, 1))
	if _, e := j.Acquire(ctx, key(t, 1, 1)); !errors.Is(e, ErrBusy) {
		t.Fatal("duplicate live handle")
	}
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { done <- a.Intent(ctx) }()
	}
	success := 0
	for i := 0; i < 2; i++ {
		if <-done == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("concurrent intent count")
	}
	a.Close()
	b := acquire(t, j, key(t, 2, 2))
	if b.Intent(ctx) != nil {
		t.Fatal("second intent")
	}
	b.Close()
	if _, e := j.Acquire(ctx, key(t, 3, 3)); !errors.Is(e, ErrBusy) {
		t.Fatal("unknown released global permit")
	}
	j.Close()
	re, e := OpenExisting(ctx, dir, owner)
	if e != nil {
		t.Fatal("two-unknown reopen")
	}
	defer re.Close()
	re.now = func() time.Time { return epoch }
	if _, e := re.Acquire(ctx, key(t, 3, 3)); !errors.Is(e, ErrBusy) {
		t.Fatal("restart freed unknown global permits")
	}
}

func TestExclusiveProcessLock(t *testing.T) {
	if dir := os.Getenv("HLO_ACTION_LOCK_DIR"); dir != "" {
		j, e := OpenExisting(ctx, dir, owner)
		if errors.Is(e, ErrBusy) {
			os.Exit(24)
		}
		if e != nil {
			os.Exit(90)
		}
		j.Close()
		return
	}
	j, dir := provision(t)
	binary, e := os.Executable()
	if e != nil {
		t.Fatal("executable")
	}
	run := func() error {
		bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(bounded, binary, "-test.run=^TestExclusiveProcessLock$", "-test.timeout=8s")
		cmd.Env = []string{"TMPDIR=" + os.TempDir(), "GOMAXPROCS=2", "HLO_ACTION_LOCK_DIR=" + dir}
		return cmd.Run()
	}
	var exit *exec.ExitError
	if e := run(); !errors.As(e, &exit) || exit.ExitCode() != 24 {
		t.Fatal("another process acquired live store")
	}
	j.Close()
	if run() != nil {
		t.Fatal("exclusive ownership not released after SQL close")
	}
}

func TestFullExpectedStateAndCancellation(t *testing.T) {
	j, _ := provision(t)
	a := acquire(t, j, key(t, 1, 1))
	j.now = func() time.Time { return epoch.Add(2 * time.Second) }
	if _, e := j.conn.ExecContext(ctx, `UPDATE actions SET admitted_at=?`, stamp(epoch.Add(time.Second))); e != nil {
		t.Fatal("fixture update")
	}
	if a.Intent(ctx) == nil || !j.failed {
		t.Fatal("stale expected admission authorized intent")
	}
	a.Close()
	j, _ = provision(t)
	a = acquire(t, j, key(t, 1, 1))
	c, cancel := context.WithCancel(ctx)
	cancel()
	if a.Intent(c) == nil || !j.failed {
		t.Fatal("canceled intent not refused")
	}
	a.Close()
}

func TestJournalHeaderEnvelope(t *testing.T) {
	hot := func() []byte {
		b := make([]byte, 512+4104)
		copy(b, []byte{0xd9, 0xd5, 0x05, 0xf9, 0x20, 0xa1, 0x63, 0xd7})
		binary.BigEndian.PutUint32(b[8:12], 1)
		binary.BigEndian.PutUint32(b[16:20], 7)
		binary.BigEndian.PutUint32(b[20:24], 512)
		binary.BigEndian.PutUint32(b[24:28], 4096)
		return b
	}
	for _, kind := range []string{"empty", "cold-zero", "cold-initialized", "hot", "magic", "page", "sector", "count", "short", "truncated", "cold-corrupt"} {
		t.Run(kind, func(t *testing.T) {
			j, dir := provision(t)
			j.Close()
			b := hot()
			good := false
			switch kind {
			case "empty":
				b = nil
				good = true
			case "cold-zero":
				b = make([]byte, 512)
				good = true
			case "cold-initialized":
				clear(b[:8])
				binary.BigEndian.PutUint32(b[8:12], 0)
				good = true
			case "hot":
				good = true
			case "magic":
				b[0] ^= 1
			case "page":
				binary.BigEndian.PutUint32(b[24:28], 8192)
			case "sector":
				binary.BigEndian.PutUint32(b[20:24], 513)
			case "count":
				binary.BigEndian.PutUint32(b[8:12], 3073)
			case "short":
				b = b[:12]
			case "truncated":
				b = b[:512]
			case "cold-corrupt":
				clear(b[:8])
				binary.BigEndian.PutUint32(b[20:24], 0)
			}
			path := filepath.Join(dir, journalName)
			if os.WriteFile(path, b, 0600) != nil {
				t.Fatal("journal fixture")
			}
			before := sha256.Sum256(b)
			e := checkJournalHeader(dir)
			if (e == nil) != good {
				t.Fatal("journal envelope classification")
			}
			after, _ := os.ReadFile(path)
			if sha256.Sum256(after) != before {
				t.Fatal("header check changed fixture")
			}
			if !good {
				if other, e := OpenExisting(ctx, dir, owner); e == nil {
					other.Close()
					t.Fatal("corrupt journal reached SQLite")
				}
				after, _ = os.ReadFile(path)
				if sha256.Sum256(after) != before {
					t.Fatal("corrupt journal altered")
				}
			}
		})
	}
}

func TestImpossibleDuplicateUnknownFencesRefuseOpen(t *testing.T) {
	for _, physical := range []bool{false, true} {
		j, dir := provision(t)
		a := acquire(t, j, key(t, 1, 1))
		if a.Intent(ctx) != nil {
			t.Fatal("intent")
		}
		a.Close()
		k := key(t, 2, 2)
		if physical {
			k.Metadata.ContainerID = key(t, 1, 1).Metadata.ContainerID
		} else {
			k.TargetID = key(t, 1, 1).TargetID
			k.Metadata.Request.TargetID = k.TargetID
		}
		k = rebuild(t, k)
		raw, _ := actioncore.CanonicalJournalMetadata(k)
		if _, e := j.conn.ExecContext(ctx, `INSERT INTO actions VALUES(?,?,?,?,?,?,?)`, k.ActionID, k.TargetID, k.Metadata.EngineGeneration, k.Metadata.ContainerID, raw, k.Fingerprint[:], stamp(epoch)); e != nil {
			t.Fatal("corrupt fixture action")
		}
		if _, e := j.conn.ExecContext(ctx, `INSERT INTO events VALUES(?,1,?,'INTENT','')`, k.ActionID, stamp(epoch)); e != nil {
			t.Fatal("corrupt fixture intent")
		}
		j.Close()
		if other, e := OpenExisting(ctx, dir, owner); e == nil {
			other.Close()
			t.Fatal("impossible duplicate fence accepted")
		}
	}
}
