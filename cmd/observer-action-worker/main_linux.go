//go:build linux

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/braidenm/home-lab-observer/internal/actionworker"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	code := run(ctx, os.Args, os.Environ(), actionworker.RunWorker)
	cancel()
	os.Exit(code)
}

// No rejected values or underlying errors are printed. The fixed environment
// admission is reused only for its identical manager-metadata/override policy;
// allowed CREDENTIALS_DIRECTORY metadata is never a credential source here.
func run(ctx context.Context, args, environ []string, worker func(context.Context) error) int {
	if ctx == nil || len(args) != 1 || args[0] == "" || connectedprofile.CheckEnvironment(environ) != nil || worker == nil {
		return 22
	}
	if err := worker(ctx); err != nil {
		return 22
	}
	return 0
}
