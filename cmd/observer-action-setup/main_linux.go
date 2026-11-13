//go:build linux

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/braidenm/home-lab-observer/internal/actionbroker"
	"github.com/braidenm/home-lab-observer/internal/actionworker"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	code := run(ctx, os.Args, os.Environ(), actionworker.RunExchange, actionbroker.ProvisionJournal)
	cancel()
	os.Exit(code)
}
func run(ctx context.Context, args, environ []string, exchange, provision func(context.Context) error) int {
	if ctx == nil || ctx.Err() != nil || len(args) != 2 || args[0] == "" || connectedprofile.CheckEnvironment(environ) != nil {
		return 22
	}
	var operation func(context.Context) error
	switch args[1] {
	case "exchange":
		operation = exchange
	case "provision-journal":
		operation = provision
	}
	if operation == nil || operation(ctx) != nil {
		return 22
	}
	return 0
}
