//go:build linux

package actionjournal

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"golang.org/x/sys/unix"
)

// Requires the explicit scratch-container profile; never fills a host filesystem.
func TestPrivateTmpfsENOSPC(t *testing.T) {
	if os.Getenv("HLO_ACTION_PRESSURE") != "1" {
		t.Skip("explicit isolated fixture required")
	}
	var fs unix.Statfs_t
	if os.TempDir() != "/fixture" || unix.Statfs("/fixture", &fs) != nil || fs.Type != unix.TMPFS_MAGIC || uint64(fs.Blocks)*uint64(fs.Bsize) > 64*1024*1024 {
		t.Fatal("not the fixed bounded private tmpfs")
	}
	for _, phase := range []string{"admission", "intent", "terminal"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "journal")
			os.Mkdir(dir, 0700)
			j, e := Provision(ctx, dir, owner)
			if e != nil {
				t.Fatal("provision")
			}
			defer j.Close()
			j.now = func() time.Time { return epoch }
			var a actioncore.Attempt
			if phase != "admission" {
				a = acquire(t, j, key(t, 1, 1))
				if phase == "terminal" && a.Intent(ctx) != nil {
					t.Fatal("intent")
				}
			}
			path := filepath.Join(root, "owned-capacity-fixture")
			f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				t.Fatal("fill file")
			}
			full := false
			var block [4096]byte
			for n := 0; n < 80*1024*1024; n += len(block) {
				if _, e = f.Write(block[:]); e != nil {
					full = errors.Is(e, unix.ENOSPC)
					break
				}
			}
			f.Close()
			if !full {
				t.Fatal("kernel ENOSPC not established")
			}
			switch phase {
			case "admission":
				_, e = j.Acquire(ctx, key(t, 1, 1))
			case "intent":
				e = a.Intent(ctx)
			case "terminal":
				e = a.Finish(ctx, actioncore.Started)
			}
			if !errors.Is(e, ErrRecovery) || !j.failed {
				t.Fatal("full storage did not fail closed")
			}
			if a != nil {
				a.Close()
			}
			j.Close()
			if os.Remove(path) != nil {
				t.Fatal("owned fixture release")
			}
			re, e := OpenExisting(ctx, dir, owner)
			if e != nil {
				t.Fatal("reopen after space release")
			}
			defer re.Close()
			re.now = func() time.Time { return epoch }
			r, e := re.read(ctx, key(t, 1, 1).ActionID)
			if phase == "admission" {
				if rowCount(t, re) != 0 {
					t.Fatal("failed admission persisted action")
				}
			} else if e != nil {
				t.Fatal("prior evidence lost")
			} else if phase == "intent" && !r.intent.IsZero() {
				t.Fatal("failed intent unexpectedly dispatched")
			} else if phase == "terminal" && (r.intent.IsZero() || !r.terminal.IsZero()) {
				t.Fatal("failed terminal lost unknown intent")
			}
		})
	}
}
