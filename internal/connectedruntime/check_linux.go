//go:build linux

// Package connectedruntime performs fixed same-process local checks. It has no
// network client, listener, credential reader or collector initialization.
package connectedruntime

import (
	"errors"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

var ErrUnsafe = errors.New("connected_runtime_refused")

// CheckPrimitives does not prove inherited-descriptor closure. Root must audit
// every descriptor of the paused invocation before publishing a probe request.
func CheckPrimitives(collector bool) error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return ErrUnsafe
	}
	return checkPrimitives(collector, primitiveCalls{socket: unix.Socket, socketpair: unix.Socketpair, close: unix.Close, shmget: func() (int, error) { return unix.SysvShmGet(unix.IPC_PRIVATE, 4096, unix.IPC_CREAT|0600) }, removeShm: func(id int) error { _, err := unix.SysvShmCtl(id, unix.IPC_RMID, nil); return err }, ring: ringSetup})
}

type primitiveCalls struct {
	socket     func(int, int, int) (int, error)
	socketpair func(int, int, int) ([2]int, error)
	close      func(int) error
	shmget     func() (int, error)
	removeShm  func(int) error
	ring       func() (int, error)
}

func checkPrimitives(collector bool, p primitiveCalls) error {
	for _, domain := range []int{unix.AF_UNIX, unix.AF_INET, unix.AF_INET6} {
		if !collector && domain != unix.AF_UNIX {
			continue
		}
		fd, err := p.socket(domain, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
		if err == nil {
			p.close(fd)
			return ErrUnsafe
		}
		if !deniedSocket(err) {
			return ErrUnsafe
		}
	}
	pair, err := p.socketpair(unix.AF_UNIX, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err == nil {
		p.close(pair[0])
		p.close(pair[1])
		return ErrUnsafe
	}
	if err != unix.EPERM {
		return ErrUnsafe
	}
	id, err := p.shmget()
	if err == nil {
		// A drifted profile can deny IPC_RMID. Always refuse; the root gate's
		// verified private IPC namespace and joined teardown then own cleanup.
		p.removeShm(id)
		return ErrUnsafe
	}
	if err != unix.EPERM {
		return ErrUnsafe
	}
	fd, err := p.ring()
	if err == nil {
		p.close(fd)
		return ErrUnsafe
	}
	if err != unix.EPERM {
		return ErrUnsafe
	}
	return nil
}

func ringSetup() (int, error) {
	// Linux's fixed io_uring_params ABI is 120 bytes; uint64 storage supplies
	// native alignment. No ring is submitted or mapped. Unexpected success is
	// closed immediately and is a terminal failed check, never accepted.
	var parameters [15]uint64
	fd, _, errno := unix.Syscall(unix.SYS_IO_URING_SETUP, 1, uintptr(unsafe.Pointer(&parameters[0])), 0)
	runtime.KeepAlive(parameters)
	if errno != 0 {
		return -1, errno
	}
	return int(fd), nil
}

func deniedSocket(err error) bool { return err == unix.EPERM || err == unix.EAFNOSUPPORT }

// CheckView checks only fixed visibility boundaries, never collection coverage.
// Collection stays explicitly incomplete until independent installation proof.
func CheckView(collector bool) error {
	if collector {
		for _, path := range []string{"/etc/home-lab-observer-connected/credentials", "/var/lib/home-lab-observer-connected/ledger", "/var/lib/home-lab-observer-connected/enrollment", "/var/lib/home-lab-observer-connected/uploader-root", "/var/lib/home-lab-observer-connected/uploader-status"} {
			f, err := os.Open(path)
			if err == nil {
				f.Close()
				return ErrUnsafe
			}
			if !os.IsPermission(err) && !os.IsNotExist(err) {
				return ErrUnsafe
			}
		}
		for _, path := range []string{"/proc/stat", "/proc/meminfo"} {
			f, err := os.Open(path)
			if err != nil {
				return ErrUnsafe
			}
			if f.Close() != nil {
				return ErrUnsafe
			}
		}
		return nil
	}
	for _, path := range []string{"/proc", "/sys", "/var", "/home", "/root"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return ErrUnsafe
		}
	}
	for _, path := range []string{"/", "/bin", "/etc", "/handoff", "/activation"} {
		i, err := os.Lstat(path)
		if err != nil || !i.IsDir() {
			return ErrUnsafe
		}
		err = unix.Access(path, unix.W_OK)
		if err != unix.EACCES && err != unix.EROFS {
			return ErrUnsafe
		}
	}
	return nil
}
