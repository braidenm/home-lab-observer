//go:build linux && (amd64 || arm64)

// The optional Linux helper has no command surface: one private pipe request,
// one bounded response, and no diagnostic output containing native data.
package main

import (
	"context"
	"io"
	"os"
	"runtime"

	"github.com/braidenm/home-lab-observer/internal/buildidentity"
	"github.com/braidenm/home-lab-observer/internal/journalnative"
	"github.com/braidenm/home-lab-observer/internal/journalreader"
	"github.com/braidenm/home-lab-observer/internal/journalruntime"
	"github.com/braidenm/home-lab-observer/internal/logobs"
	"github.com/braidenm/home-lab-observer/internal/logprocess"
	"github.com/braidenm/home-lab-observer/internal/logprotocol"
)

var releaseIdentity = ""

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, journalruntime.Harden, openReader))
}

func run(args []string, input io.Reader, output io.Writer, harden func() error, open func() (logobs.Reader, func() error, error)) int {
	if len(args) != 0 || releaseIdentity == "" {
		return 1
	}
	identity, err := buildidentity.Resolve(releaseIdentity, "", "", "journal-helper", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return 1
	}
	return logprocess.Serve(context.Background(), logprocess.HelperConfig{
		Build:  logprotocol.Build{Version: identity.Version, Commit: identity.Commit, OS: identity.OS, Arch: identity.Arch},
		Harden: harden, Open: open,
	}, input, output)
}

func openReader() (logobs.Reader, func() error, error) {
	factory, err := journalnative.NewFactory()
	if err != nil {
		return nil, nil, err
	}
	reader, err := journalreader.New(journalreader.Config{Factory: factory})
	if err != nil {
		_ = factory.Close()
		return nil, nil, err
	}
	return reader, factory.Close, nil
}
