//go:build !linux

package actionjournal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

func TestUnsupportedNeverTouchesStorage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "must-not-exist")
	if _, e := Provision(context.Background(), dir, Binding{}); !errors.Is(e, ErrUnsupported) {
		t.Fatal("provision")
	}
	if _, e := OpenExisting(context.Background(), dir, Binding{}); !errors.Is(e, ErrUnsupported) {
		t.Fatal("open")
	}
	if _, e := os.Lstat(dir); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("unsupported touched disk")
	}
	j := new(Journal)
	if _, e := j.Acquire(context.Background(), actioncore.Key{}); !errors.Is(e, ErrUnsupported) {
		t.Fatal("acquire")
	}
	if !errors.Is(j.Prune(context.Background()), ErrUnsupported) || j.Close() != nil {
		t.Fatal("unsupported lifecycle")
	}
}
