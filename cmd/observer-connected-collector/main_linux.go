//go:build linux

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
	"github.com/braidenm/home-lab-observer/internal/numerichost"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/sharedhandoff"
)

var releaseIdentity string

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	os.Exit(run(ctx))
}

func run(ctx context.Context) (code int) {
	if connectedprofile.CheckEnvironment(os.Environ()) != nil {
		return 22
	}
	identity, err := connectedidentity.Resolve(releaseIdentity, "collector")
	if err != nil || len(os.Args) != 1 {
		return 22
	}
	c, err := connectedprofile.Load()
	if err != nil || connectedprofile.CheckIdentity(c, true) != nil {
		return 22
	}
	w, err := sharedhandoff.OpenWriter(connectedprofile.StateDirectory+"/handoff", sharedhandoff.Policy{CollectorUID: c.CollectorUID, UploaderUID: c.UploaderUID, SharedGID: c.SharedGID, ServerID: c.ServerID})
	if err != nil {
		return 22
	}
	defer func() {
		if w.Close() != nil {
			code = 22
		}
	}()
	status, err := connectedstatus.Open(connectedprofile.StateDirectory + "/collector-status")
	if err != nil {
		return 22
	}
	defer func() {
		if status.Close() != nil {
			code = 22
		}
	}()
	collector := numerichost.New(nil, numerichost.GopsutilProvider{}, numerichost.Config{CollectorVersion: identity.Version, CPUSampleDuration: time.Second, MaxFilesystems: 16})
	for ctx.Err() == nil {
		snapshot := collector.Collect(ctx)
		if ctx.Err() != nil {
			break
		}
		if w.Publish(snapshot, remoteprojection.Identity{SourceID: c.ServerID, Version: identity.Version, OS: "linux"}) != nil {
			return 22
		}
		if status.Write(connectedstatus.Record{Version: "observer-connected-status/v1", State: "COLLECTING", UpdatedAt: time.Now().UTC()}) != nil {
			return 22
		}
		timer := time.NewTimer(15 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	return 0
}
