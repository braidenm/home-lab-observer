//go:build windows && (amd64 || arm64)

package main

import (
	"context"
	"io"
	"runtime"

	"github.com/braidenm/home-lab-observer/internal/buildidentity"
	"github.com/braidenm/home-lab-observer/internal/eventnative"
	"github.com/braidenm/home-lab-observer/internal/eventreader"
	"github.com/braidenm/home-lab-observer/internal/logobs"
	"github.com/braidenm/home-lab-observer/internal/logprocess"
	"github.com/braidenm/home-lab-observer/internal/logprotocol"
)

const privateLogHelperCommand = "__log-helper"

// dispatchPrivateLogHelper recognizes one private, code-owned child mode before
// ordinary CLI parsing or diagnostics. The verified parent supplies this exact
// argument; it is not a public command surface.
func dispatchPrivateLogHelper(args []string, input io.Reader, output io.Writer) (bool, int) {
	if len(args) == 0 || args[0] != privateLogHelperCommand {
		return false, 0
	}
	return true, runWindowsLogHelper(args[1:], releaseIdentity, input, output, func() error { return nil }, openWindowsEventReader)
}

func runWindowsLogHelper(args []string, rawIdentity string, input io.Reader, output io.Writer, harden func() error, open func() (logobs.Reader, func() error, error)) int {
	if len(args) != 0 || rawIdentity == "" {
		return 1
	}
	identity, err := buildidentity.Resolve(rawIdentity, "", "", "observer", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return 1
	}
	return logprocess.Serve(context.Background(), logprocess.HelperConfig{
		Build:  logprotocol.Build{Version: identity.Version, Commit: identity.Commit, OS: identity.OS, Arch: identity.Arch},
		Harden: harden,
		Open:   open,
	}, input, output)
}

func openWindowsEventReader() (logobs.Reader, func() error, error) {
	factory, err := eventnative.NewFactory()
	if err != nil {
		return nil, nil, err
	}
	reader, err := eventreader.New(eventreader.Config{Factory: factory})
	if err != nil {
		return nil, nil, err
	}
	return reader, nil, nil
}
