//go:build linux

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/braidenm/home-lab-observer/internal/actionbroker"
	"github.com/braidenm/home-lab-observer/internal/actionworker"
	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedinstall"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

var releaseIdentity string

func main() {
	identity, err := connectedidentity.Resolve(releaseIdentity, "action-setup")
	if err != nil {
		os.Exit(22)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	code := 22
	if len(os.Args) > 1 && os.Args[1] == "stage-release" {
		code = runStage(ctx, os.Args, os.Environ(), identity, connectedinstall.StageActionRelease)
	} else {
		code = run(ctx, os.Args, os.Environ(), actionworker.RunExchange, actionbroker.ProvisionJournal, actionworker.SeedEnrollment)
	}
	cancel()
	os.Exit(code)
}

func runStage(ctx context.Context, args, environ []string, identity connectedidentity.Identity,
	stage func(context.Context, string, string, connectedidentity.Identity) error) int {
	if ctx == nil || ctx.Err() != nil || len(args) != 4 || args[0] == "" || args[1] != "stage-release" ||
		args[2] == "" || connectedprofile.CheckEnvironment(environ) != nil || stage == nil {
		return 22
	}
	if stage(ctx, args[2], args[3], identity) != nil {
		return 22
	}
	return 0
}
func run(ctx context.Context, args, environ []string, exchange, provision, seed func(context.Context) error) int {
	if ctx == nil || ctx.Err() != nil || len(args) != 2 || args[0] == "" || connectedprofile.CheckEnvironment(environ) != nil {
		return 22
	}
	var operation func(context.Context) error
	switch args[1] {
	case "exchange":
		operation = exchange
	case "provision-journal":
		operation = provision
	case "seed-enrollment":
		operation = seed
	}
	if operation == nil || operation(ctx) != nil {
		return 22
	}
	return 0
}
