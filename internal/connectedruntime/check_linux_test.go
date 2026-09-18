//go:build linux

package connectedruntime

import (
	"errors"
	"reflect"
	"testing"

	"golang.org/x/sys/unix"
)

func TestUnexpectedPrimitiveSuccessRejectsAndAttemptsCleanup(t *testing.T) {
	for _, stage := range []string{"socket", "socketpair", "shm", "shm-cleanup-refused", "ring", "all-denied"} {
		t.Run(stage, func(t *testing.T) {
			var cleanup []int
			p := primitiveCalls{socket: func(int, int, int) (int, error) { return -1, unix.EPERM }, socketpair: func(int, int, int) ([2]int, error) { return [2]int{}, unix.EPERM }, shmget: func() (int, error) { return -1, unix.EPERM }, removeShm: func(id int) error { cleanup = append(cleanup, id); return nil }, ring: func() (int, error) { return -1, unix.EPERM }, close: func(fd int) error { cleanup = append(cleanup, fd); return nil }}
			var want []int
			switch stage {
			case "socket":
				p.socket = func(int, int, int) (int, error) { return 10, nil }
				want = []int{10}
			case "socketpair":
				p.socketpair = func(int, int, int) ([2]int, error) { return [2]int{10, 11}, nil }
				want = []int{10, 11}
			case "shm", "shm-cleanup-refused":
				p.shmget = func() (int, error) { return 12, nil }
				want = []int{12}
				p.removeShm = func(id int) error {
					cleanup = append(cleanup, id)
					if stage == "shm-cleanup-refused" {
						return unix.EPERM
					}
					return nil
				}
			case "ring":
				p.ring = func() (int, error) { return 13, nil }
				want = []int{13}
			}
			err := checkPrimitives(false, p)
			if stage == "all-denied" {
				if err != nil {
					t.Fatal("fixed denials refused")
				}
			} else if err != ErrUnsafe {
				t.Fatal("unexpected success authorized startup")
			}
			if !reflect.DeepEqual(cleanup, want) {
				t.Fatalf("cleanup attempt mismatch: got%v want%v", cleanup, want)
			}
		})
	}
}

func TestOnlyExplicitSocketDenialsCount(t *testing.T) {
	for _, err := range []error{unix.EPERM, unix.EAFNOSUPPORT} {
		if !deniedSocket(err) {
			t.Fatal("supported denial refused")
		}
	}
	for _, err := range []error{nil, unix.ENOSYS, unix.EMFILE, unix.EINVAL, errors.New("permission denied")} {
		if deniedSocket(err) {
			t.Fatal("absence or resource failure treated as isolation")
		}
	}
}
