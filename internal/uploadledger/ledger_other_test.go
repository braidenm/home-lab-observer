//go:build !linux

package uploadledger

import (
	"context"
	"os"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

func TestUnsupportedDoesNotTouchDirectory(t *testing.T) {
	dir := t.TempDir()
	if _, err := Provision(context.Background(), dir, uploadstate.Binding{}); err != ErrUnsupported {
		t.Fatal("unsupported provision")
	}
	if _, err := OpenExisting(context.Background(), dir, uploadstate.Binding{}); err != ErrUnsupported {
		t.Fatal("unsupported open")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("unsupported platform touched storage")
	}
	l := &Ledger{}
	if _, err := l.Load(context.Background()); err != ErrUnsupported {
		t.Fatal("unsupported load")
	}
	if err := l.Commit(context.Background(), uploadstate.Record{}, uploadstate.Record{}); err != ErrUnsupported {
		t.Fatal("unsupported commit")
	}
	if l.Close() != nil {
		t.Fatal("unsupported close")
	}
}
