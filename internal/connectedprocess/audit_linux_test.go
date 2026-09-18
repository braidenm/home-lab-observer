//go:build linux

package connectedprocess

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestAuditOwnedChild(t *testing.T) {
	for _, mode := range []string{"ordinary", "high-file", "socket", "high-socket", "unix-socket", "many-files", "missing-stdin"} {
		t.Run(mode, func(t *testing.T) {
			cmd, input := child(t, mode)
			executable, err := os.Executable()
			if err != nil {
				t.Fatal("test executable unavailable")
			}
			identity, err := Audit(context.Background(), cmd.Process.Pid, executable)
			wantOK := mode == "ordinary" || mode == "high-file"
			if wantOK {
				if err != nil || identity.PID != cmd.Process.Pid || identity.StartTimeTicks == 0 {
					t.Fatal("safe child refused")
				}
				again, err := Audit(context.Background(), cmd.Process.Pid, executable)
				if err != nil || again != identity {
					t.Fatal("identity changed")
				}
			} else if err != ErrUnsafe || identity != (Identity{}) {
				t.Fatal("unsafe child accepted")
			}
			if _, err := Audit(context.Background(), cmd.Process.Pid, "/dev/null"); err != ErrUnsafe {
				t.Fatal("wrong executable accepted")
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := Audit(ctx, cmd.Process.Pid, executable); err != ErrUnsafe {
				t.Fatal("canceled audit accepted")
			}
			input.Close()
			if cmd.Wait() != nil {
				t.Fatal("child exit failed")
			}
			if _, err := Audit(context.Background(), cmd.Process.Pid, executable); err != ErrUnsafe {
				t.Fatal("exited child accepted")
			}
		})
	}
}

func child(t *testing.T, mode string) (*exec.Cmd, io.WriteCloser) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProcessFixture$")
	cmd.Env = []string{"OBSERVER_PROCESS_FIXTURE=" + mode}
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal("fixture input unavailable")
	}
	output, err := cmd.StdoutPipe()
	if err != nil || cmd.Start() != nil {
		t.Fatal("fixture start failed")
	}
	t.Cleanup(func() { input.Close(); cmd.Process.Kill(); cmd.Wait() })
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || line != "READY\n" {
		t.Fatal("fixture not ready")
	}
	return cmd, input
}

func TestProcessFixture(t *testing.T) {
	mode := os.Getenv("OBSERVER_PROCESS_FIXTURE")
	if mode == "" {
		return
	}
	fail := func() { os.Exit(2) }
	var fd int
	var err error
	control := os.Stdin
	switch mode {
	case "socket", "high-socket":
		fd, err = unix.Socket(unix.AF_INET, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	case "unix-socket":
		fd, err = unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	case "high-file":
		fd, err = unix.Open("/dev/null", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	case "many-files":
		for i := 0; i < 257; i++ {
			if _, err := unix.Open("/dev/null", unix.O_RDONLY|unix.O_CLOEXEC, 0); err != nil {
				fail()
			}
		}
	case "ordinary":
	case "missing-stdin":
		if unix.Dup3(0, 8, unix.O_CLOEXEC) != nil {
			fail()
		}
		control = os.NewFile(8, "fixture-control")
		if unix.Close(0) != nil {
			fail()
		}
	default:
		fail()
	}
	if err != nil {
		fail()
	}
	if mode == "high-socket" || mode == "high-file" {
		if unix.Dup3(fd, 4096, unix.O_CLOEXEC) != nil {
			fail()
		}
		unix.Close(fd)
		if unix.Setrlimit(unix.RLIMIT_NOFILE, &unix.Rlimit{Cur: 256, Max: 256}) != nil {
			fail()
		}
	}
	// Every socket is synthetic and unconnected. High-number fixtures lower only
	// this child process's limits AFTER creating the descriptor under test.
	if _, err := io.WriteString(os.Stdout, "READY\n"); err != nil {
		fail()
	}
	var b [1]byte
	control.Read(b[:])
	os.Exit(0)
}

func TestEmptyAndMissingStandardDescriptorInventory(t *testing.T) {
	// Synthetic fd directories exercise the inaccessible/empty inventory shape
	// without terminating the native test runner's main thread.
	for _, present := range [][]string{nil, {"0", "1"}, {"0", "2"}, {"1", "2"}} {
		dir := t.TempDir()
		if os.Mkdir(filepath.Join(dir, "fd"), 0700) != nil {
			t.Fatal("fixture directory failed")
		}
		for _, name := range present {
			f, err := os.Create(filepath.Join(dir, "fd", name))
			if err != nil {
				t.Fatal("fixture entry failed")
			}
			f.Close()
		}
		f, err := os.Open(dir)
		if err != nil {
			t.Fatal("fixture handle failed")
		}
		err = auditDescriptors(context.Background(), int(f.Fd()))
		f.Close()
		if err != ErrUnsafe {
			t.Fatal("incomplete inventory accepted")
		}
	}
}

func TestParseStartRejectsMalformedIdentity(t *testing.T) {
	fields := append([]string{"S"}, strings.Fields(strings.Repeat("0 ", 18))...)
	fields = append(fields, "1234", "0")
	b := []byte("42 (synthetic ) comm) " + strings.Join(fields, " ") + "\n")
	if got, err := parseStart(b, 42); err != nil || got != 1234 {
		t.Fatal("valid stat refused")
	}
	for _, bad := range [][]byte{nil, []byte("42 broken"), []byte("42 () S"), []byte(strings.Replace(string(b), "1234", "-1", 1)), []byte(strings.Replace(string(b), "1234", "0", 1)), []byte(strings.Replace(string(b), "1234", "18446744073709551616", 1))} {
		if _, err := parseStart(bad, 42); err != ErrUnsafe {
			t.Fatal("malformed identity accepted")
		}
	}
	if _, err := parseStart(b, 43); err != ErrUnsafe {
		t.Fatal("wrong PID accepted")
	}
}

func TestAuditRejectsInvalidArguments(t *testing.T) {
	for _, pid := range []int{-1, 0} {
		if _, err := Audit(context.Background(), pid, "/fixed/observer"); err != ErrUnsafe {
			t.Fatal("invalid PID accepted")
		}
	}
	for _, path := range []string{"", "relative", "/fixed/../observer", strings.Repeat("/", 4097)} {
		if _, err := Audit(context.Background(), os.Getpid(), path); err != ErrUnsafe {
			t.Fatal("invalid path accepted")
		}
	}
	if _, err := Audit(nil, os.Getpid(), "/fixed/observer"); err != ErrUnsafe {
		t.Fatal("nil context accepted")
	}
}
