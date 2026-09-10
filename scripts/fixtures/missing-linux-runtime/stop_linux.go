package main

import (
	"os/exec"
	"syscall"
	"time"
)

func stop(command *exec.Cmd, done <-chan error) error {
	if command.Process.Signal(syscall.SIGTERM) != nil {
		return errProof
	}
	select {
	case err := <-done:
		if err != nil {
			return errProof
		}
		return nil
	case <-time.After(10 * time.Second):
		_ = command.Process.Kill()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		return errProof
	}
}
