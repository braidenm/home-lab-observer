package logprocess

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logprotocol"
)

func TestSyntheticChild(t *testing.T) {
	if len(os.Args) < 2 || !strings.HasPrefix(os.Args[len(os.Args)-1], "observer-child=") {
		return
	}
	mode := strings.TrimPrefix(os.Args[len(os.Args)-1], "observer-child=")
	switch mode {
	case "echo":
		_, _ = io.Copy(os.Stdout, os.Stdin)
	case "maximum", "overflow":
		n := logprotocol.MaxResponseBytes
		if mode == "overflow" {
			n++
		}
		_, _ = os.Stdout.Write(bytes.Repeat([]byte("x"), n))
	case "fail":
		_, _ = io.Copy(io.Discard, os.Stdin)
		_, _ = os.Stderr.WriteString("synthetic-private-native-error")
		_, _ = os.Stdout.WriteString("partial-private-output")
		os.Exit(7)
	case "block":
		time.Sleep(time.Minute)
	default:
		os.Exit(9)
	}
	os.Exit(0)
}

func childTransport(t *testing.T, mode string) *transport {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return newTransport(func() (commandSpec, error) {
		return commandSpec{path: executable, args: []string{"-test.run=^TestSyntheticChild$", "observer-child=" + mode}, env: []string{"LANG=C"}}, nil
	})
}

func TestPrivateProcessBoundsAndFailures(t *testing.T) {
	for _, tc := range []struct {
		mode string
		size int
		want error
	}{
		{"echo", logprotocol.MaxRequestBytes, nil},
		{"maximum", 0, nil},
		{"overflow", 0, errOutputTooLarge},
		{"fail", 12, errProcessFailed},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			output, err := childTransport(t, tc.mode).exchange(context.Background(), bytes.Repeat([]byte("z"), tc.size))
			if !errors.Is(err, tc.want) {
				t.Fatalf("fixed result: %v", err)
			}
			if tc.want != nil && len(output) != 0 {
				t.Fatal("failure leaked partial packet")
			}
			if tc.mode == "echo" && !bytes.Equal(output, bytes.Repeat([]byte("z"), tc.size)) {
				t.Fatal("private pipe mismatch")
			}
			if tc.mode == "maximum" && len(output) != logprotocol.MaxResponseBytes {
				t.Fatal("exact output cap rejected")
			}
		})
	}
}

func TestNoLaunchForOversizeOrCanceledInput(t *testing.T) {
	var calls atomic.Int32
	tr := newTransport(func() (commandSpec, error) { calls.Add(1); return commandSpec{}, nil })
	if output, err := tr.exchange(context.Background(), make([]byte, logprotocol.MaxRequestBytes+1)); !errors.Is(err, errInputInvalid) || output != nil {
		t.Fatal("oversize accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if output, err := tr.exchange(ctx, nil); !errors.Is(err, context.Canceled) || output != nil {
		t.Fatal("cancellation ignored")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid request launched child")
	}
}

func TestCancelKillsAndReapsBeforeReuse(t *testing.T) {
	tr := childTransport(t, "block")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	output, err := tr.exchange(ctx, nil)
	if !errors.Is(err, context.DeadlineExceeded) || output != nil {
		t.Fatalf("deadline: %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("caller deadline exceeded cleanup bound")
	}
	// Completion can lag the bounded caller grace on slow CI. Wait only for the
	// actual single slot, not an arbitrary sleep or a new launch.
	select {
	case tr.slot <- struct{}{}:
		<-tr.slot
	case <-time.After(time.Second):
		t.Fatal("direct child was not reaped")
	}
}

func TestUnreapedChildOccupiesSingleSlot(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	tr := &transport{slot: make(chan struct{}, 1), run: func(context.Context, []byte) ([]byte, error) { close(entered); <-release; return nil, errProcessFailed }}
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := tr.exchange(ctx, nil); done <- err }()
	<-entered
	if _, err := tr.exchange(context.Background(), nil); !errors.Is(err, errProcessBusy) {
		t.Fatal("concurrent child allowed")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("unbounded cleanup wait")
	}
	if _, err := tr.exchange(context.Background(), nil); !errors.Is(err, errProcessBusy) {
		t.Fatal("unreaped child slot released")
	}
}

func TestLaunchFailureAndUnspecifiedEnvironmentArePrivate(t *testing.T) {
	for _, spec := range []commandSpec{{path: "/synthetic/not-an-observer", env: []string{"LANG=C"}}, {path: "/synthetic/private-path"}} {
		tr := newTransport(func() (commandSpec, error) { return spec, nil })
		out, err := tr.exchange(context.Background(), []byte("synthetic-private-cursor"))
		if !errors.Is(err, errProcessFailed) || len(out) != 0 || strings.Contains(err.Error(), "synthetic") {
			t.Fatal("launch boundary leaked or inherited environment")
		}
	}
}
