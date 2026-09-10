package eventreader

import (
	"runtime"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/logobs"
	"golang.org/x/sys/windows"
)

// This reads only the test thread ID, never a Windows event channel.
func TestNativeLifetimeRemainsOnCreatingWindowsThread(t *testing.T) {
	var owner uint32
	calls := 0
	check := func() {
		runtime.Gosched()
		current := windows.GetCurrentThreadId()
		if owner == 0 {
			owner = current
		} else if current != owner {
			t.Fatal("query lifetime moved OS threads")
		}
		calls++
	}
	row := fixtureRecord(1)
	row.onClose = check
	q := &fakeQuery{records: []*fakeRecord{row}, nextHook: check, onClose: check}
	if _, err := readFixture(t, &fakeFactory{initial: q, openHook: check}, logobs.Checkpoint{}); err != nil {
		t.Fatal(err)
	}
	if calls != 5 {
		t.Fatalf("missing lifetime callbacks: %d", calls)
	}
}
