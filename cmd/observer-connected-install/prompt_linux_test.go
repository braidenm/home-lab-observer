//go:build linux

package main

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type promptReady struct {
	once  sync.Once
	ready chan struct{}
}

func (p *promptReady) Write(b []byte) (int, error) {
	p.once.Do(func() { close(p.ready) })
	return len(b), nil
}

func TestPromptFlushesSuffixAndRestoresTerminal(t *testing.T) {
	for _, kind := range []string{"valid", "overflow", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			master := os.NewFile(uintptr(fd), "owned-pty-master")
			defer master.Close()
			if unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0) != nil {
				t.Fatal("unlock pty")
			}
			number, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
			if err != nil {
				t.Fatal(err)
			}
			slave, err := os.OpenFile("/dev/pts/"+strconv.Itoa(number), os.O_RDWR|unix.O_NOCTTY, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer slave.Close()
			original, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			prompt := &promptReady{ready: make(chan struct{})}
			type answer struct {
				b   []byte
				err error
			}
			done := make(chan answer, 1)
			go func() { b, err := readGrantFrom(ctx, slave, prompt); done <- answer{b, err} }()
			select {
			case <-prompt.ready:
			case <-ctx.Done():
				t.Fatal("prompt deadline")
			}
			payload := "hle_" + strings.Repeat("a", 43) + "\nunused-pasted-command\n"
			if kind == "overflow" {
				payload = strings.Repeat("x", 128) + "\nunused-pasted-command\n"
			}
			if kind == "cancel" {
				payload = "hle_partial_synthetic"
			}
			if _, err := master.Write([]byte(payload)); err != nil {
				t.Fatal(err)
			}
			if kind == "cancel" {
				cancel()
			}
			var result answer
			select {
			case result = <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("prompt failed to join")
			}
			if kind == "valid" {
				if result.err != nil || len(result.b) != 47 {
					t.Fatal("valid grant refused")
				}
			} else if result.err == nil {
				t.Fatal("bad grant accepted")
			}
			clear(result.b)
			restored, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
			if err != nil || restored.Lflag != original.Lflag {
				t.Fatal("terminal settings not restored")
			}
			queued, err := unix.IoctlGetInt(int(slave.Fd()), unix.TIOCINQ)
			if err != nil || queued != 0 {
				t.Fatal("pasted suffix left for shell")
			}
		})
	}
}
