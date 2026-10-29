//go:build linux

package connectedruntime

import (
	"runtime"

	"golang.org/x/sys/unix"
)

// CheckRichProcessesDockerReaderPrimitives proves the reader's distinct local
// capabilities without dialing the Engine. Every successful test FD is closed
// before startup publishes its response for root's independent FD audit.
func CheckRichProcessesDockerReaderPrimitives() error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return ErrUnsafe
	}
	return checkDockerReaderPrimitives(primitiveCalls{socket: unix.Socket, socketpair: unix.Socketpair, close: unix.Close, shmget: func() (int, error) { return unix.SysvShmGet(unix.IPC_PRIVATE, 4096, unix.IPC_CREAT|0600) }, removeShm: func(id int) error { _, err := unix.SysvShmCtl(id, unix.IPC_RMID, nil); return err }, ring: ringSetup})
}

func checkDockerReaderPrimitives(p primitiveCalls) error {
	if checkDeniedPrimitives([]int{unix.AF_INET, unix.AF_INET6}, p) != nil {
		return ErrUnsafe
	}
	fd, err := p.socket(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return ErrUnsafe
	}
	if p.close(fd) != nil {
		return ErrUnsafe
	}
	return nil
}
