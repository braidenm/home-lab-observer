//go:build linux

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/braidenm/home-lab-observer/internal/actionbroker"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	code := run(ctx, os.Args, os.Environ(), actionbroker.RunBroker)
	cancel()
	os.Exit(code)
}

// Only the identical manager-environment policy is shared with the worker.
// Metadata is inert; rejected values and runtime failures are never printed.
func run(ctx context.Context, args, environ []string, broker func(context.Context) error) int {
	if ctx == nil || len(args) != 1 || args[0] == "" || connectedprofile.CheckEnvironment(environ) != nil || broker == nil {
		return 22
	}
	if broker(ctx) != nil {
		return 22
	}
	return 0
}
