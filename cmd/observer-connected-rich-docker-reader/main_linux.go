//go:build linux

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/dockerreader"
)

var releaseIdentity string

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	code := run(ctx)
	cancel()
	os.Exit(code)
}

func validateLaunch(environ []string, identity string, args []string) error {
	if len(args) != 1 || connectedprofile.CheckRichProcessesDockerReaderEnvironment(environ) != nil {
		return connectedprofile.ErrUnsafe
	}
	if _, err := connectedidentity.Resolve(identity, "rich-docker-reader"); err != nil {
		return err
	}
	return nil
}

func run(ctx context.Context) int {
	if validateLaunch(os.Environ(), releaseIdentity, os.Args) != nil || dockerreader.RunInstalled(ctx) != nil {
		return 22
	}
	return 0
}
