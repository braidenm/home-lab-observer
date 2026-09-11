//go:build linux

package uploadledger

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// This explicitly opted-in test never fills a host filesystem. The subprocess
// owns a private mount namespace and a new bounded tmpfs containing synthetic data.
func TestStoragePressureFixture(t *testing.T) {
	if os.Getenv("HLO_LEDGER_PRESSURE") != "1" {
		t.Skip("explicit privileged fixture opt-in required")
	}
	if os.Geteuid() != 0 {
		t.Fatal("fixture requires root and mount namespace capabilities")
	}
	if os.Getenv("HLO_LEDGER_PRESSURE_CHILD") == "1" {
		runStoragePressure(t)
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("fixture executable unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestStoragePressureFixture$", "-test.count=1")
	cmd.Env = []string{"HLO_LEDGER_PRESSURE=1", "HLO_LEDGER_PRESSURE_CHILD=1", "TMPDIR=/tmp"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: unix.CLONE_NEWNS, Pdeathsig: unix.SIGKILL}
	if cmd.Run() != nil {
		t.Fatal("isolated storage-pressure fixture failed")
	}
}

func runStoragePressure(t *testing.T) {
	t.Helper()
	selfNS, selfErr := os.Readlink("/proc/self/ns/mnt")
	parentNS, parentErr := os.Readlink("/proc/" + strconv.Itoa(os.Getppid()) + "/ns/mnt")
	if selfErr != nil || parentErr != nil || selfNS == parentNS {
		t.Fatal("fixture requires a newly isolated mount namespace")
	}
	// Applied only within the newly created child mount namespace, before mounting.
	if unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, "") != nil {
		t.Fatal("private mount propagation unavailable")
	}
	root := t.TempDir()
	if unix.Mount("tmpfs", root, "tmpfs", unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, "size=1m,mode=0700") != nil {
		t.Fatal("bounded fixture mount unavailable")
	}
	defer func() {
		if unix.Unmount(root, 0) != nil {
			t.Error("fixture unmount failed")
		}
	}()
	dir := filepath.Join(root, "ledger")
	if os.Mkdir(dir, 0o700) != nil {
		t.Fatal("fixture directory creation failed")
	}
	ledger, err := Provision(context.Background(), dir, binding)
	if err != nil {
		t.Fatal("fixture ledger provisioning failed")
	}
	defer ledger.Close()
	first := pending(t, 1, 0)
	if ledger.Commit(context.Background(), load(t, ledger), first) != nil {
		t.Fatal("fixture initial admission failed")
	}
	fillPath := filepath.Join(root, "capacity-fixture")
	fill, err := os.OpenFile(fillPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal("capacity fixture creation failed")
	}
	full := false
	var block [4096]byte
	for written := 0; written < 2*1024*1024; written += len(block) {
		_, writeErr := fill.Write(block[:])
		if writeErr != nil {
			full = errors.Is(writeErr, unix.ENOSPC)
			break
		}
	}
	if fill.Close() != nil || !full {
		t.Fatal("fixture did not establish kernel ENOSPC")
	}
	if !errors.Is(ledger.Commit(context.Background(), first, pending(t, 2, time.Second)), ErrRecovery) {
		t.Fatal("full filesystem commit was not refused")
	}
	if _, err := ledger.Load(context.Background()); !errors.Is(err, ErrRecovery) {
		t.Fatal("failed commit did not latch recovery")
	}
	// Remove only the file this test created on its private bounded mount.
	if os.Remove(fillPath) != nil {
		t.Fatal("capacity release failed")
	}
	if ledger.Close() != nil {
		t.Fatal("fixture ledger close failed")
	}
	reopened, err := OpenExisting(context.Background(), dir, binding)
	if err != nil {
		t.Fatal("fixture ledger recovery failed")
	}
	defer reopened.Close()
	if !sameRecord(load(t, reopened), first) {
		t.Fatal("recovery changed the previously committed pending record")
	}
}
