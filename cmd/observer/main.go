package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/braidenm/home-lab-observer/internal/collector"
	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/projection"
)

var version = "dev"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, nil)) }

type collectFunc func(context.Context) observation.Snapshot

func run(args []string, stdout, stderr io.Writer, collect collectFunc) int {
	if len(args) > 0 && args[0] == "serve" {
		return runServe(args[1:], stdout, stderr)
	}
	if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		printUsage(stdout)
		return 0
	}
	if len(args) == 0 || args[0] != "collect-once" {
		printUsage(stderr)
		return 2
	}
	if len(args) == 2 && (args[1] == "-h" || args[1] == "--help") {
		printUsage(stdout)
		return 0
	}
	fs := flag.NewFlagSet("collect-once", flag.ContinueOnError)
	fs.SetOutput(stderr)
	maxProcesses := fs.Int("max-processes", 25, "maximum number of process summaries (1-200)")
	processes := fs.Bool("processes", true, "collect safe process summaries")
	timeout := fs.Duration("timeout", 10*time.Second, "collection deadline")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 || *maxProcesses < 1 || *maxProcesses > 200 || *timeout <= 0 || *timeout > time.Minute {
		fmt.Fprintln(stderr, "invalid collect-once options")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	if collect == nil {
		cfg := collector.DefaultConfig()
		cfg.CollectorVersion, cfg.MaxProcesses, cfg.CollectProcesses = version, *maxProcesses, *processes
		c := collector.New(collector.RealClock{}, collector.GopsutilProvider{}, cfg)
		collect = c.Collect
	}
	current := projection.Current(collect(ctx), projection.NativeSystemInfo())
	if err := json.NewEncoder(stdout).Encode(current); err != nil {
		fmt.Fprintln(stderr, "failed to encode observation")
		return 1
	}
	if current.CollectionState == "FAILED" {
		return 1
	}
	return 0
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: observer collect-once [--max-processes N] [--processes=true|false] [--timeout DURATION]")
	fmt.Fprintln(writer, "       observer serve [--listen 127.0.0.1:9847] [--state-dir PATH] [--docker-endpoint LOCAL_SOCKET]")
}
