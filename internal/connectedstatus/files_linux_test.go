//go:build linux

package connectedstatus

import (
	"golang.org/x/sys/unix"
	"os"
	"testing"
	"time"
)

func TestPrivateBoundedStatusLifecycle(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	w, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err != ErrUnsafe {
		t.Fatal("concurrent writer accepted")
	}
	r := Record{Version: "observer-connected-status/v1", State: "COLLECTING", UpdatedAt: time.Now().UTC()}
	for range 50 {
		if err := w.Write(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatal("status storage grew")
	}
	if err := os.WriteFile(dir+"/.status-next", []byte("interrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	w, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	if _, err := os.Stat(dir + "/.status-next"); !os.IsNotExist(err) {
		t.Fatal("disposable staging not cleared")
	}
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err != ErrUnsafe {
		t.Fatal("shared directory accepted")
	}
}

func TestUnsafeStatusEntriesFailWithoutFollowing(t *testing.T) {
	for _, kind := range []string{"symlink-lock", "fifo-lock", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			if os.Chmod(dir, 0700) != nil {
				t.Fatal("chmod")
			}
			switch kind {
			case "symlink-lock":
				target := t.TempDir() + "/unrelated"
				if os.WriteFile(target, nil, 0600) != nil || os.Symlink(target, dir+"/.status-lock") != nil {
					t.Fatal("fixture")
				}
			case "fifo-lock":
				if unix.Mkfifo(dir+"/.status-lock", 0600) != nil {
					t.Fatal("fixture")
				}
			case "unknown":
				if os.WriteFile(dir+"/unknown", nil, 0600) != nil {
					t.Fatal("fixture")
				}
			}
			if w, err := Open(dir); err != ErrUnsafe {
				if w != nil {
					w.Close()
				}
				t.Fatal("unsafe entry accepted")
			}
		})
	}
}
