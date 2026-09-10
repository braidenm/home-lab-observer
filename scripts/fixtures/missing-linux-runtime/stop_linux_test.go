package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

func TestStopSyntheticChild(t *testing.T) {
	if os.Getenv("OBSERVER_SYNTHETIC_STOP_CHILD") == "1" {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGTERM)
		fmt.Println("READY")
		<-stop
		os.Exit(0)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestStopSyntheticChild$")
	command.Env = append(os.Environ(), "OBSERVER_SYNTHETIC_STOP_CHILD=1")
	output, err := command.StdoutPipe()
	if err != nil || command.Start() != nil {
		t.Fatal("synthetic child failed")
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	defer command.Process.Kill()
	ready := make(chan bool, 1)
	go func() { scanner := bufio.NewScanner(output); ready <- scanner.Scan() && scanner.Text() == "READY" }()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("child not ready")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("child readiness timeout")
	}
	if stop(command, done) != nil {
		t.Fatal("graceful join failed")
	}
	if command.ProcessState == nil || !command.ProcessState.Success() {
		t.Fatal("child not reaped")
	}
}
