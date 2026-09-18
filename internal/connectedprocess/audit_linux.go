//go:build linux

package connectedprocess

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const maxDescriptors = 256

// Audit examines every descriptor, including numbers above RLIMIT_NOFILE.
// expectedExecutable is the caller's verified, fixed artifact path, never user
// input. The caller must independently check the manager's exact InvocationID,
// MainPID, unit restrictions and absence of socket activation/FD store before
// and after Audit, then publish the fresh request without replacing the worker.
// No /proc mount is needed in the worker. Nondumpable cross-UID workers require
// sufficient parent ptrace authority; inaccessible state fails closed.
// The deadline is checked between fixed local kernel operations; it cannot
// interrupt an individual blocked kernel filesystem operation.
func Audit(ctx context.Context, pid int, expectedExecutable string) (Identity, error) {
	if ctx == nil || ctx.Err() != nil || pid <= 0 || !filepath.IsAbs(expectedExecutable) || filepath.Clean(expectedExecutable) != expectedExecutable || len(expectedExecutable) > 4096 {
		return Identity{}, ErrUnsafe
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pidfd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return Identity{}, ErrUnsafe
	}
	defer unix.Close(pidfd)
	proc, err := unix.Open("/proc/"+strconv.Itoa(pid), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return Identity{}, ErrUnsafe
	}
	defer unix.Close(proc)
	var fs unix.Statfs_t
	if unix.Fstatfs(proc, &fs) != nil || fs.Type != unix.PROC_SUPER_MAGIC {
		return Identity{}, ErrUnsafe
	}
	artifact, err := unix.Open(expectedExecutable, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return Identity{}, ErrUnsafe
	}
	defer unix.Close(artifact)
	var expected unix.Stat_t
	if unix.Fstat(artifact, &expected) != nil || expected.Mode&unix.S_IFMT != unix.S_IFREG || expected.Mode&0111 == 0 {
		return Identity{}, ErrUnsafe
	}
	start, err := readStart(proc, pid)
	if err != nil || !live(pidfd) || !sameExecutable(proc, &expected) {
		return Identity{}, ErrUnsafe
	}
	// Repeat complete inspection. These passes are not an atomic inventory or a
	// proof against transient churn; the reviewed startup barrier prevents
	// application socket activity throughout this audit.
	for pass := 0; pass < 2; pass++ {
		if ctx.Err() != nil || auditDescriptors(ctx, proc) != nil {
			return Identity{}, ErrUnsafe
		}
	}
	end, err := readStart(proc, pid)
	if err != nil || start != end || !live(pidfd) || !sameExecutable(proc, &expected) || ctx.Err() != nil {
		return Identity{}, ErrUnsafe
	}
	return Identity{PID: pid, StartTimeTicks: start}, nil
}

func live(pidfd int) bool {
	fds := []unix.PollFd{{Fd: int32(pidfd), Events: unix.POLLIN}}
	n, err := unix.Poll(fds, 0)
	return err == nil && n == 0 && fds[0].Revents == 0
}

func sameExecutable(proc int, expected *unix.Stat_t) bool {
	var actual unix.Stat_t
	return unix.Fstatat(proc, "exe", &actual, 0) == nil && actual.Mode&unix.S_IFMT == unix.S_IFREG && actual.Dev == expected.Dev && actual.Ino == expected.Ino && actual.Size == expected.Size
}

func auditDescriptors(ctx context.Context, proc int) error {
	fd, err := unix.Openat(proc, "fd", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrUnsafe
	}
	dir := os.NewFile(uintptr(fd), "process-descriptors")
	defer dir.Close()
	names, err := dir.Readdirnames(maxDescriptors + 1)
	if err != nil && err != io.EOF || len(names) > maxDescriptors {
		return ErrUnsafe
	}
	var standard [3]bool
	for _, name := range names {
		if ctx.Err() != nil {
			return ErrUnsafe
		}
		n, err := strconv.ParseUint(name, 10, 31)
		if err != nil || strconv.FormatUint(n, 10) != name {
			return ErrUnsafe
		}
		if n < 3 {
			standard[n] = true
		}
		var st unix.Stat_t
		// Follow the kernel's fd reference to obtain type only. Never open or
		// read another process's descriptor contents or emit its target path.
		if unix.Fstatat(fd, name, &st, 0) != nil || st.Mode&unix.S_IFMT == unix.S_IFSOCK {
			return ErrUnsafe
		}
	}
	// The fixed service profile requires all three standard descriptors. An
	// empty proc inventory can also mean a terminated main thread rather than
	// an absence of descriptors in a still-live multithreaded process.
	if !standard[0] || !standard[1] || !standard[2] {
		return ErrUnsafe
	}
	return nil
}

func readStart(proc, pid int) (uint64, error) {
	fd, err := unix.Openat(proc, "stat", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return 0, ErrUnsafe
	}
	f := os.NewFile(uintptr(fd), "process-identity")
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(b) > 4096 {
		return 0, ErrUnsafe
	}
	return parseStart(b, pid)
}

func parseStart(b []byte, pid int) (uint64, error) {
	s := string(b)
	if !strings.HasPrefix(s, strconv.Itoa(pid)+" (") {
		return 0, ErrUnsafe
	}
	end := strings.LastIndex(s, ") ")
	if end < 0 {
		return 0, ErrUnsafe
	}
	fields := strings.Fields(s[end+2:])
	if len(fields) < 20 {
		return 0, ErrUnsafe
	}
	n, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != fields[19] {
		return 0, ErrUnsafe
	}
	return n, nil
}
