//go:build !linux

package enrollmentstore

import (
	"context"
	"os"
	"testing"
)

func TestUnsupportedDoesNotMutate(t *testing.T) {
	path := t.TempDir()
	if _, err := OpenNew(context.Background(), path); err != ErrUnsupported {
		t.Fatal(err)
	}
	if _, err := OpenReady(context.Background(), path, testBinding()); err != ErrUnsupported {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 0 {
		t.Fatal("unsupported mutated directory")
	}
}
