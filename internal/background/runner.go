package background

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"
)

const maxCommandOutput = 256 << 10

type command struct {
	name    string
	args    []string
	capture bool
}

type commandResult struct {
	output   string
	exitCode int
}

type commandRunner interface {
	run(context.Context, command) (commandResult, error)
}

type execRunner struct{ timeout time.Duration }

func (r execRunner) run(ctx context.Context, request command) (commandResult, error) {
	if request.name == "" {
		return commandResult{}, errors.New("empty fixed command")
	}
	timeout := r.timeout
	if timeout <= 0 || timeout > GracefulWait {
		timeout = 10 * time.Second
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	executable, err := trustedManagerExecutable(request.name)
	if err != nil {
		return commandResult{}, err
	}
	cmd := exec.CommandContext(commandCtx, executable, request.args...)
	cmd.WaitDelay = 2 * time.Second
	prepareManagerCommand(cmd)
	var output bytes.Buffer
	limited := &limitedWriter{writer: &output, remaining: maxCommandOutput}
	if request.capture {
		cmd.Stdout = limited
		cmd.Stderr = io.Discard
	} else {
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
	}
	err = cmd.Run()
	result := commandResult{output: output.String()}
	if limited.overflow {
		return result, errors.New("manager command output exceeded its limit")
	}
	if err == nil {
		return result, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		result.exitCode = exit.ExitCode()
		return result, err
	}
	if commandCtx.Err() != nil {
		return result, commandCtx.Err()
	}
	return result, fmt.Errorf("run fixed manager command: %w", err)
}

type limitedWriter struct {
	writer    io.Writer
	remaining int
	overflow  bool
}

func (w *limitedWriter) Write(value []byte) (int, error) {
	original := len(value)
	if len(value) > w.remaining {
		value = value[:w.remaining]
		w.overflow = true
	}
	if len(value) > 0 {
		_, _ = w.writer.Write(value)
		w.remaining -= len(value)
	}
	return original, nil
}
