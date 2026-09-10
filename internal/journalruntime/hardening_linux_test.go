package journalruntime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Native policy changes are confined to this fresh child. Never crash the child,
// create a dump, read private input or query the journal to verify the policy.
func TestHardenNativeChild(t *testing.T) {
	const marker = "OBSERVER_TEST_JOURNAL_HARDENING_CHILD"
	if os.Getenv(marker) == "1" {
		if Harden() != nil || Harden() != nil {
			t.Fatal("native hardening unavailable")
		}
		var limit unix.Rlimit
		if unix.Getrlimit(unix.RLIMIT_CORE, &limit) != nil || limit.Cur != 0 || limit.Max != 0 {
			t.Fatal("native core limits not disabled")
		}
		dumpable, err := unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0)
		if err != nil || dumpable != 0 {
			t.Fatal("native process remains dumpable")
		}
		fmt.Println("HELPER_HARDENING_OK")
		return
	}
	var before, after unix.Rlimit
	if unix.Getrlimit(unix.RLIMIT_CORE, &before) != nil {
		t.Fatal("cannot read parent core policy")
	}
	beforeDumpable, err := unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0)
	if err != nil {
		t.Fatal("cannot read parent dumpability")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHardenNativeChild$", "-test.count=1")
	child.Env = append(os.Environ(), marker+"=1")
	child.WaitDelay = time.Second
	output, err := child.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "HELPER_HARDENING_OK\n") {
		t.Fatal("isolated native hardening test failed")
	}
	if unix.Getrlimit(unix.RLIMIT_CORE, &after) != nil || before != after {
		t.Fatal("parent core policy changed")
	}
	afterDumpable, err := unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0)
	if err != nil || beforeDumpable != afterDumpable {
		t.Fatal("parent dumpability changed")
	}
}
