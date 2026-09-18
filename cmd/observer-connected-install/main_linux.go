//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedinstall"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

var releaseIdentity string

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	code := run(ctx)
	cancel()
	os.Exit(code)
}

func run(ctx context.Context) int {
	if connectedprofile.HardenSecretProcess() != nil {
		return 22
	}
	if _, err := connectedidentity.Resolve(releaseIdentity, "install"); err != nil {
		return 22
	}
	if len(os.Args) != 5 || os.Args[1] != "install" {
		fmt.Fprintln(os.Stderr, "Usage: observer-connected-install install <verified-bundle-directory> <manifest-sha256> <server-id>")
		return 22
	}
	if connectedinstall.CheckRequest(ctx, os.Args[2], os.Args[3], os.Args[4]) != nil {
		fmt.Fprintln(os.Stderr, "PREFLIGHT_REFUSED: verify privileges, supported host and bundle before enrollment.")
		return 22
	}
	grant, err := readGrant(ctx)
	if err != nil {
		return 22
	}
	defer clear(grant)
	err = connectedinstall.Install(ctx, connectedinstall.Request{BundleDirectory: os.Args[2], ManifestSHA256: os.Args[3], ServerID: os.Args[4], Grant: grant})
	if err != nil {
		var phase *connectedinstall.PhaseError
		if errors.As(err, &phase) && phase.Phase == "PREFLIGHT_REFUSED" {
			fmt.Fprintln(os.Stderr, "PREFLIGHT_REFUSED: no enrollment attempted; verify bundle, supported host and network prerequisites.")
		} else if errors.As(err, &phase) && phase.Phase == "LOCAL_SETUP_INCOMPLETE" {
			fmt.Fprintln(os.Stderr, "LOCAL_SETUP_INCOMPLETE: enrollment was not attempted; preserve partial local setup for explicit recovery.")
		} else {
			fmt.Fprintln(os.Stderr, "ENROLLMENT_RECOVERY_REQUIRED: exchange may have occurred; preserve setup and revoke registration before new enrollment.")
		}
		return 22
	}
	fmt.Fprintln(os.Stdout, "INSTALLED_PENDING_ACCEPTANCE: services remain stopped; packaged isolation acceptance is required before activation.")
	return 0
}

func readGrant(ctx context.Context) ([]byte, error) { return readGrantFrom(ctx, os.Stdin, os.Stderr) }

func readGrantFrom(ctx context.Context, terminal *os.File, prompt io.Writer) (result []byte, resultErr error) {
	if ctx == nil || terminal == nil || prompt == nil {
		return nil, connectedinstall.ErrUnsafe
	}
	// A controlling-terminal prompt avoids argv/environment/history and refuses
	// redirected streams that might accidentally echo or persist a grant.
	fd := int(terminal.Fd())
	original, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, connectedinstall.ErrUnsafe
	}
	quiet := *original
	quiet.Lflag &^= unix.ECHO | unix.ECHONL
	if unix.IoctlSetTermios(fd, unix.TCSETS, &quiet) != nil {
		return nil, connectedinstall.ErrUnsafe
	}
	defer func() {
		// TCSAFLUSH restores settings while discarding unread pasted suffixes,
		// including commands/newlines after an overlong or canceled grant.
		if unix.IoctlSetTermios(fd, unix.TCSETSF, original) != nil {
			clear(result)
			result = nil
			resultErr = connectedinstall.ErrUnsafe
		}
	}()
	fmt.Fprint(prompt, "One-use enrollment grant (paste only hle_ value): ")
	buf := make([]byte, 0, 48)
	one := make([]byte, 1)
	for len(buf) <= 47 {
		if ctx.Err() != nil {
			clear(buf)
			return nil, connectedinstall.ErrUnsafe
		}
		poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(poll, 100)
		if err == unix.EINTR || n == 0 {
			continue
		}
		if err != nil || poll[0].Revents&unix.POLLIN == 0 {
			clear(buf)
			return nil, connectedinstall.ErrUnsafe
		}
		n, err = terminal.Read(one)
		if err != nil || n != 1 {
			clear(buf)
			return nil, connectedinstall.ErrUnsafe
		}
		if one[0] == '\n' {
			break
		}
		buf = append(buf, one[0])
	}
	fmt.Fprintln(prompt)
	if len(buf) != 47 {
		clear(buf)
		return nil, connectedinstall.ErrUnsafe
	}
	return buf, nil
}
