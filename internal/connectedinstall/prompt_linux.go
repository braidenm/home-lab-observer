//go:build linux

package connectedinstall

import (
	"context"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// ReadEnrollmentGrantFrom reads one bounded grant from a real terminal without
// echo. Both fixed installers share the cancellation and terminal-restore path;
// callers never put the grant on argv, environment or command history.
func ReadEnrollmentGrantFrom(ctx context.Context, terminal *os.File, prompt io.Writer) (result []byte, resultErr error) {
	if ctx == nil || terminal == nil || prompt == nil {
		return nil, ErrUnsafe
	}
	fd := int(terminal.Fd())
	original, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, ErrUnsafe
	}
	quiet := *original
	quiet.Lflag &^= unix.ECHO | unix.ECHONL
	if unix.IoctlSetTermios(fd, unix.TCSETS, &quiet) != nil {
		return nil, ErrUnsafe
	}
	defer func() {
		if unix.IoctlSetTermios(fd, unix.TCSETSF, original) != nil {
			clear(result)
			result = nil
			resultErr = ErrUnsafe
		}
	}()
	fmt.Fprint(prompt, "One-use enrollment grant (paste only hle_ value): ")
	buf := make([]byte, 0, 48)
	one := make([]byte, 1)
	defer clear(one)
	for len(buf) <= 47 {
		if ctx.Err() != nil {
			clear(buf)
			return nil, ErrUnsafe
		}
		poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(poll, 100)
		if err == unix.EINTR || n == 0 {
			continue
		}
		if err != nil || poll[0].Revents&unix.POLLIN == 0 {
			clear(buf)
			return nil, ErrUnsafe
		}
		n, err = terminal.Read(one)
		if err != nil || n != 1 {
			clear(buf)
			return nil, ErrUnsafe
		}
		if one[0] == '\n' {
			break
		}
		buf = append(buf, one[0])
	}
	fmt.Fprintln(prompt)
	if len(buf) != 47 {
		clear(buf)
		return nil, ErrUnsafe
	}
	return buf, nil
}
