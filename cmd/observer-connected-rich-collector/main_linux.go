//go:build linux

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/braidenm/home-lab-observer/internal/collector"
	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedprocess"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedruntime"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
	"github.com/braidenm/home-lab-observer/internal/containerobs/model"
	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/sharedhandoff"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

var releaseIdentity string

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	code := run(ctx)
	cancel()
	os.Exit(code)
}

func validateLaunch(environ []string, identity string, args []string) (connectedidentity.Identity, error) {
	if connectedprofile.CheckEnvironment(environ) != nil || len(args) != 1 {
		return connectedidentity.Identity{}, connectedprofile.ErrUnsafe
	}
	return connectedidentity.Resolve(identity, "rich-collector")
}

func loadPolicy(c connectedprofile.RichProcessesConfig) ([32]byte, error) {
	p, err := connectedprofile.LoadRichProcessesPolicy()
	if err != nil || connectedprofile.VerifyRichProcessesBinding(c, p) != nil {
		return [32]byte{}, connectedprofile.ErrUnsafe
	}
	return connectedprofile.DigestRichProcessesPolicy(p)
}

func encodeProcesses(raw observation.Snapshot, server, version string) ([]byte, error) {
	host, err := remoteprojection.EncodeNative(raw, remoteprojection.Identity{SourceID: server, Version: version, OS: "linux"})
	if err != nil {
		return nil, err
	}
	workloads, err := workloadprojection.Encode(raw, model.Inventory{SchemaVersion: model.SchemaVersion, SupportState: model.SupportDisabled}, workloadprojection.Identity{SourceID: server, Version: version}, workloadprojection.Consent{Processes: true})
	if err != nil {
		return nil, err
	}
	return combinedprojection.Encode(host, workloads, server)
}

func run(ctx context.Context) (code int) {
	identity, err := validateLaunch(os.Environ(), releaseIdentity, os.Args)
	if err != nil {
		return 22
	}
	c, err := connectedprofile.LoadRichProcessesConfig()
	if err != nil || connectedprofile.CheckRichProcessesIdentity(c, true) != nil {
		return 22
	}
	digest, err := loadPolicy(c)
	if err != nil {
		return 22
	}
	if _, err := connectedprocess.Audit(ctx, os.Getpid(), connectedprofile.RichProcessesReleaseDirectory+"/"+c.ArtifactSHA256+"/observer-connected-rich-collector"); err != nil {
		return 22
	}
	if connectedruntime.CheckPrimitives(true) != nil || connectedruntime.CheckRichProcessesView(true) != nil {
		return 22
	}
	w, err := sharedhandoff.OpenRichProcessesWriter(connectedprofile.RichProcessesStateDirectory+"/handoff", sharedhandoff.RichProcessesPolicy{Policy: sharedhandoff.Policy{CollectorUID: c.CollectorUID, UploaderUID: c.UploaderUID, SharedGID: c.SharedGID, ServerID: c.ServerID}, PolicyDigest: digest})
	if err != nil {
		return 22
	}
	defer func() {
		if w.Close() != nil {
			code = 22
		}
	}()
	status, err := connectedstatus.Open(connectedprofile.RichProcessesStateDirectory + "/collector-status")
	if err != nil {
		return 22
	}
	defer func() {
		if status.Close() != nil {
			code = 22
		}
	}()
	provider := collector.NewInstalled(nil, collector.GopsutilProvider{}, collector.Config{CollectorVersion: identity.Version, CPUSampleDuration: time.Second, MaxProcesses: 200, MaxFilesystems: 16, CollectProcesses: true})
	for ctx.Err() == nil {
		// A root policy/config change is not a new consent for this invocation.
		current, err := connectedprofile.LoadRichProcessesConfig()
		if err != nil || current.PolicySHA256 != c.PolicySHA256 || current.ArtifactSHA256 != c.ArtifactSHA256 || connectedprofile.CheckRichProcessesIdentity(current, true) != nil {
			return 22
		}
		if now, err := loadPolicy(current); err != nil || now != digest {
			return 22
		}
		cycle, cancel := context.WithTimeout(ctx, 10*time.Second)
		raw := provider.Collect(cycle)
		cancel()
		if ctx.Err() != nil {
			break
		}
		body, err := encodeProcesses(raw, c.ServerID, identity.Version)
		if err != nil {
			return 22
		}
		// Revalidate consent after collection and before publishing sensitive rows.
		if now, err := loadPolicy(c); err != nil || now != digest {
			return 22
		}
		if w.Publish(body) != nil {
			return 22
		}
		if status.Write(connectedstatus.Record{Version: "observer-connected-status/v1", InvocationID: os.Getenv("INVOCATION_ID"), State: "COLLECTING", UpdatedAt: time.Now().UTC()}) != nil {
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
