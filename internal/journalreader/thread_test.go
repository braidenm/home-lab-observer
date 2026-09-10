//go:build linux || windows

package journalreader

import (
	"context"
	"testing"
	"time"
)

func TestOpenReadCloseKeepOwningThread(t *testing.T) {
	j := newJournal(row(queryTime, 0))
	var owner uint64
	factory := &fakeFactory{journal: j, hook: func() { owner = nativeThreadID() }}
	j.hook = func(string) {
		if nativeThreadID() != owner {
			t.Fatal("native handle moved away from owning thread")
		}
	}
	reader, _ := New(Config{Factory: factory, Now: func() time.Time { return queryTime }})
	if _, err := reader.Read(context.Background(), initialRequest()); err != nil {
		t.Fatal("thread ownership fixture failed")
	}
	if owner == 0 || j.closes != 1 {
		t.Fatal("thread ownership lifecycle incomplete")
	}
}
