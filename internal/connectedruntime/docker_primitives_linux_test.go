//go:build linux

package connectedruntime

import (
	"reflect"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDockerReaderPrimitiveProofClosesLocalSocketAndNeverAllowsNetwork(t *testing.T) {
	for _, change := range []string{"valid", "inet-success", "inet6-success", "unix-denied", "unix-close-failure", "socketpair", "shm", "ring"} {
		var closed []int
		p := primitiveCalls{socket: func(domain, kind, protocol int) (int, error) {
			if domain == unix.AF_UNIX {
				if change == "unix-denied" {
					return -1, unix.EPERM
				}
				return 17, nil
			}
			if change == "inet-success" && domain == unix.AF_INET || change == "inet6-success" && domain == unix.AF_INET6 {
				return 18, nil
			}
			return -1, unix.EAFNOSUPPORT
		}, socketpair: func(int, int, int) ([2]int, error) {
			if change == "socketpair" {
				return [2]int{19, 20}, nil
			}
			return [2]int{}, unix.EPERM
		}, close: func(fd int) error {
			closed = append(closed, fd)
			if change == "unix-close-failure" {
				return unix.EIO
			}
			return nil
		}, shmget: func() (int, error) {
			if change == "shm" {
				return 21, nil
			}
			return -1, unix.EPERM
		}, removeShm: func(int) error { return nil }, ring: func() (int, error) {
			if change == "ring" {
				return 22, nil
			}
			return -1, unix.EPERM
		}}
		err := checkDockerReaderPrimitives(p)
		if (err == nil) != (change == "valid") {
			t.Fatal("reader primitive boundary mismatch")
		}
		if change == "valid" && !reflect.DeepEqual(closed, []int{17}) {
			t.Fatal("local test socket remained open")
		}
	}
}
