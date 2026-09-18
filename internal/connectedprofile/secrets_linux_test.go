//go:build linux

package connectedprofile

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSecretHardeningOnlyInChild(t *testing.T) {
	if len(os.Args) > 1 && os.Args[len(os.Args)-1] == "--secret-boundary-child" {
		if HardenSecretProcess() != nil {
			os.Exit(1)
		}
		var limit unix.Rlimit
		dumpable, err := unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0)
		if err != nil || dumpable != 0 || unix.Getrlimit(unix.RLIMIT_CORE, &limit) != nil || limit.Cur != 0 || limit.Max != 0 {
			os.Exit(2)
		}
		if HardenSecretProcess() != nil {
			os.Exit(3)
		}
		os.Exit(0)
	}
	var before, after unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &before); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSecretHardeningOnlyInChild$", "--", "--secret-boundary-child")
	c.WaitDelay = time.Second
	if err := c.Run(); err != nil {
		t.Fatal("child boundary failed", err)
	}
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &after); err != nil || before != after {
		t.Fatal("parent limits changed")
	}
}
