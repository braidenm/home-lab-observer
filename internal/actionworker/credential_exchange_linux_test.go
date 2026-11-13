//go:build linux

package actionworker

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestCredentialInputEOFGrammarAndCallerFlags(t *testing.T) {
	for _, suffix := range []string{"", "\n", "\n\n", "x", "\r\n"} {
		t.Run(strconv.Itoa(len(suffix))+suffix, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal("owned pipe")
			}
			defer r.Close()
			flags, _ := unix.FcntlInt(r.Fd(), unix.F_GETFL, 0)
			fd, err := unix.Open("/proc/self/fd/"+strconv.Itoa(int(r.Fd())), unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
			if err != nil {
				t.Fatal("private pipe descriptor")
			}
			defer unix.Close(fd)
			if _, err := w.Write([]byte("hlae_" + strings.Repeat("1", 32) + "." + strings.Repeat("A", 43) + suffix)); err != nil {
				t.Fatal("synthetic input")
			}
			w.Close()
			op, _ := newCredentialOperation(context.Background())
			defer op.close()
			_, err = consumeEnrollment(op, fd)
			if (suffix == "" || suffix == "\n") != (err == nil) {
				t.Fatal("closed input grammar")
			}
			after, _ := unix.FcntlInt(r.Fd(), unix.F_GETFL, 0)
			if flags != after {
				t.Fatal("caller flags changed")
			}
		})
	}
}
func TestCredentialInputNativeDeadlineAndCancelDuringCapture(t *testing.T) {
	r, w, _ := os.Pipe()
	defer r.Close()
	defer w.Close()
	fd, err := unix.Open("/proc/self/fd/"+strconv.Itoa(int(r.Fd())), unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal("private pipe")
	}
	defer unix.Close(fd)
	for _, mode := range []string{"5s", "30s", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			op, _ := newCredentialOperation(ctx)
			defer op.close()
			op.capture = func() (nativeStamp, error) {
				now := op.start
				switch mode {
				case "cancel":
					cancel()
				case "5s":
					now.at = now.at.Add(5 * time.Second)
					now.boot += 5 * time.Second
				case "30s":
					now.at = now.at.Add(30 * time.Second)
					now.boot += 30 * time.Second
				}
				return now, nil
			}
			if _, err := consumeEnrollment(op, fd); err == nil {
				t.Fatal("withheld EOF/native/cancel admitted")
			}
		})
	}
}
