// Package logprocess contains the private fixed-helper process boundary. It is
// not an arbitrary command runner and has no HTTP, storage or upload role.
package logprocess

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logprotocol"
)

const sourceTimeout = 2 * time.Second
const cleanupGrace = 100 * time.Millisecond

var (
	errInputInvalid   = errors.New("LOG_HELPER_INPUT_INVALID")
	errOutputTooLarge = errors.New("LOG_HELPER_OUTPUT_TOO_LARGE")
	errProcessFailed  = errors.New("LOG_HELPER_PROCESS_FAILED")
	errProcessBusy    = errors.New("LOG_HELPER_PROCESS_BUSY")
)

// Only the code-owned platform resolver can supply this in production. No
// exported constructor accepts a path, argv, environment or command callback.
type commandSpec struct {
	path string
	args []string
	env  []string
}

type transport struct {
	slot chan struct{}
	run  func(context.Context, []byte) ([]byte, error)
}

func newTransport(resolve func() (commandSpec, error)) *transport {
	return &transport{slot: make(chan struct{}, 1), run: func(ctx context.Context, input []byte) ([]byte, error) {
		spec, err := resolve()
		if err != nil {
			if errors.Is(err, ErrHelperUnavailable) {
				return nil, ErrHelperUnavailable
			}
			if errors.Is(err, ErrHelperMismatch) {
				return nil, ErrHelperMismatch
			}
			return nil, errProcessFailed
		}
		return runCommand(ctx, spec, input)
	}}
}

type result struct {
	output []byte
	err    error
}

func (t *transport) exchange(parent context.Context, input []byte) ([]byte, error) {
	if parent == nil || len(input) > logprotocol.MaxRequestBytes {
		return nil, errInputInvalid
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	select {
	case t.slot <- struct{}{}:
	default:
		return nil, errProcessBusy
	}
	ctx, cancel := context.WithTimeout(parent, sourceTimeout)
	defer cancel()
	done := make(chan result, 1)
	privateInput := bytes.Clone(input)
	go func() {
		defer func() { <-t.slot }()
		output, err := t.run(ctx, privateInput)
		if err != nil {
			output = nil
		}
		done <- result{output, err}
	}()
	select {
	case completed := <-done:
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return completed.output, completed.err
	case <-ctx.Done():
		// The slot is released only by the actual reaper. An uninterruptible OS
		// wait cannot create a chain of abandoned child processes or goroutines.
		timer := time.NewTimer(cleanupGrace)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
		}
		return nil, ctx.Err()
	}
}

func runCommand(parent context.Context, spec commandSpec, input []byte) ([]byte, error) {
	if !filepath.IsAbs(spec.path) || len(spec.env) == 0 {
		return nil, errProcessFailed
	}
	if parent.Err() != nil {
		return nil, parent.Err()
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	output := &boundedOutput{cancel: cancel}
	cmd := exec.CommandContext(ctx, spec.path, spec.args...)
	cmd.Dir = filepath.Dir(spec.path)
	cmd.Env = append([]string(nil), spec.env...)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	cmd.WaitDelay = cleanupGrace
	configureChild(cmd)
	// CommandContext kills only this verified helper; helpers never spawn
	// descendants. Run has exactly one Wait owner and releases OS resources.
	err := cmd.Run()
	if output.overflow.Load() {
		return nil, errOutputTooLarge
	}
	if parent.Err() != nil {
		return nil, parent.Err()
	}
	if err != nil {
		return nil, errProcessFailed
	}
	return output.data, nil
}

type boundedOutput struct {
	data     []byte
	overflow atomic.Bool
	cancel   context.CancelFunc
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > logprotocol.MaxResponseBytes-len(b.data) {
		b.overflow.Store(true)
		b.cancel()
		return 0, errOutputTooLarge
	}
	// Exact capacity prevents append growth retaining more than the wire cap.
	if b.data == nil {
		b.data = make([]byte, 0, logprotocol.MaxResponseBytes)
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
