//go:build !linux

package sharedhandoff

import (
	"context"
	"os"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
)

func TestUnsupportedHasNoFilesystemSideEffects(t *testing.T) {
	dir := t.TempDir()
	if _, err := OpenWriter(dir, Policy{}); err != ErrUnsupported {
		t.Fatal("unsupported writer")
	}
	if _, err := OpenReader(dir, Policy{}); err != ErrUnsupported {
		t.Fatal("unsupported reader")
	}
	w, r := &Writer{}, &Reader{}
	if w.Publish(observation.Snapshot{}, remoteprojection.Identity{}) != ErrUnsupported {
		t.Fatal("unsupported publish")
	}
	if _, err := r.Read(context.Background(), ""); err != ErrUnsupported {
		t.Fatal("unsupported read")
	}
	if w.Close() != nil || r.Close() != nil {
		t.Fatal("unsupported close")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("unsupported touched storage")
	}
}
