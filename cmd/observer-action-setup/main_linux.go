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
	code := run(ctx, os.Args, os.Environ(), actionworker.RunExchange)
	cancel()
	os.Exit(code)
}
func run(ctx context.Context, args, environ []string, exchange func(context.Context) error) int {
	if ctx == nil || len(args) != 2 || args[0] == "" || args[1] != "exchange" || connectedprofile.CheckEnvironment(environ) != nil || exchange == nil {
		return 22
	}
	if exchange(ctx) != nil {
		return 22
	}
	return 0
}
