//go:build linux

package connectedinstall

import (
	"context"
	"strings"
	"testing"
)

func TestActivationStartCaptureOnTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	owned := map[string]workerInstance{}
	want := workerInstance{PID: 123, Invocation: strings.Repeat("a", 32), Active: "active"}
	err := startActivationWorkerWith(ctx, uploaderUnit, owned,
		func(context.Context, string, string) error {
			if _, ok := owned[uploaderUnit]; !ok {
				t.Fatal("attempt not recorded before command")
			}
			cancel()
			return ErrRecovery
		},
		func(check context.Context, unit string) (workerInstance, error) {
			if check.Err() != nil {
				t.Fatal("cleanup inherited canceled context")
			}
			if _, ok := check.Deadline(); !ok {
				t.Fatal("unbounded recovery inspection")
			}
			return want, nil
		})
	if err != ErrRecovery || owned[uploaderUnit] != want {
		t.Fatal("uncertain launched worker lost")
	}
}

func TestActivationStartTransitionAndUnknown(t *testing.T) {
	for _, state := range []string{"activating", "deactivating", "unknown"} {
		t.Run(state, func(t *testing.T) {
			owned := map[string]workerInstance{}
			want := workerInstance{PID: 123, Invocation: strings.Repeat("a", 32), Active: state}
			err := startActivationWorkerWith(context.Background(), uploaderUnit, owned, func(context.Context, string, string) error { return nil }, func(context.Context, string) (workerInstance, error) {
				if state == "unknown" {
					return workerInstance{}, ErrUnsafe
				}
				return want, nil
			})
			if err != ErrRecovery {
				t.Fatal("non-ready start accepted")
			}
			got, present := owned[uploaderUnit]
			if !present {
				t.Fatal("attempt disappeared")
			}
			if state == "unknown" {
				if got.Invocation != "" {
					t.Fatal("invented invocation")
				}
			} else if got != want {
				t.Fatal("transition identity lost")
			}
		})
	}
}

func TestActivationWaitCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if activationWait(ctx) == nil {
		t.Fatal("canceled wait accepted")
	}
}

func TestActivationStartCancellationDuringCapture(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	owned := map[string]workerInstance{}
	want := workerInstance{PID: 123, Invocation: strings.Repeat("a", 32), Active: "active"}
	err := startActivationWorkerWith(ctx, uploaderUnit, owned,
		func(context.Context, string, string) error { return nil },
		func(check context.Context, _ string) (workerInstance, error) {
			cancel()
			if check.Err() != nil {
				t.Fatal("capture lost identity to concurrent cancellation")
			}
			return want, nil
		})
	if err != ErrRecovery || owned[uploaderUnit] != want {
		t.Fatal("canceled start not retained for cleanup")
	}
}
