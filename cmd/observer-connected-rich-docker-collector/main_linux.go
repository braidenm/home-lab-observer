//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/hex"
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
	return connectedidentity.Resolve(identity, "rich-docker-collector")
}

func loadPolicy(c connectedprofile.RichProcessesDockerConfig) ([32]byte, error) {
	p, err := connectedprofile.LoadRichProcessesDockerPolicy()
	if err != nil || connectedprofile.VerifyRichProcessesDockerBinding(c, p) != nil {
		return [32]byte{}, connectedprofile.ErrUnsafe
	}
	return connectedprofile.DigestRichProcessesDockerPolicy(p)
}

type frameReader interface {
	Read(context.Context, string, time.Time) (workloadprojection.ContainerSection, error)
}

func readContainers(ctx context.Context, reader frameReader, server string, now time.Time) workloadprojection.ContainerSection {
	section, err := reader.Read(ctx, server, now)
	if err != nil {
		return workloadprojection.UnavailableContainers()
	}
	return section
}

func encodeDocker(raw observation.Snapshot, server, version string, section workloadprojection.ContainerSection) ([]byte, error) {
	host, err := remoteprojection.EncodeNative(raw, remoteprojection.Identity{SourceID: server, Version: version, OS: "linux"})
	if err != nil {
		return nil, err
	}
	workloads, err := workloadprojection.EncodeProcessesAndContainers(raw, workloadprojection.Identity{SourceID: server, Version: version}, section)
	if err != nil {
		return nil, err
	}
	return combinedprojection.Encode(host, workloads, server)
}

func currentPolicy(expected []byte, digest [32]byte) error {
	current, err := connectedprofile.LoadRichProcessesDockerConfig()
	if err != nil || connectedprofile.CheckRichProcessesDockerIdentity(current, true) != nil {
		return connectedprofile.ErrUnsafe
	}
	canonical, err := connectedprofile.EncodeRichProcessesDockerConfig(current)
	if err != nil || !bytes.Equal(canonical, expected) {
		return connectedprofile.ErrUnsafe
	}
	now, err := loadPolicy(current)
	if err != nil || now != digest {
		return connectedprofile.ErrUnsafe
	}
	return nil
}

func run(ctx context.Context) (code int) {
	identity, err := validateLaunch(os.Environ(), releaseIdentity, os.Args)
	if err != nil {
		return 22
	}
	c, err := connectedprofile.LoadRichProcessesDockerConfig()
	if err != nil || connectedprofile.CheckRichProcessesDockerIdentity(c, true) != nil {
		return 22
	}
	expected, err := connectedprofile.EncodeRichProcessesDockerConfig(c)
	if err != nil {
		return 22
	}
	digest, err := loadPolicy(c)
	if err != nil {
		return 22
	}
	if _, err := connectedprocess.Audit(ctx, os.Getpid(), connectedprofile.RichProcessesDockerReleaseDirectory+"/"+c.ArtifactSHA256+"/observer-connected-rich-docker-collector"); err != nil {
		return 22
	}
	if connectedruntime.CheckPrimitives(true) != nil || connectedruntime.CheckRichProcessesDockerView(true) != nil {
		return 22
	}
	w, err := sharedhandoff.OpenRichProcessesDockerWriter(connectedprofile.RichProcessesDockerStateDirectory+"/handoff", sharedhandoff.RichProcessesDockerPolicy{Policy: sharedhandoff.Policy{CollectorUID: c.CollectorUID, UploaderUID: c.UploaderUID, SharedGID: c.SharedGID, ServerID: c.ServerID}, PolicyDigest: digest})
	if err != nil {
		return 22
	}
	defer func() {
		if w.Close() != nil {
			code = 22
		}
	}()
	definition, err := hex.DecodeString(c.DefinitionSHA256)
	if err != nil || len(definition) != 32 {
		return 22
	}
	framePolicy := sharedhandoff.DockerFramePolicy{Policy: sharedhandoff.Policy{CollectorUID: c.ReaderUID, UploaderUID: c.CollectorUID, SharedGID: c.FrameGID, ServerID: c.ServerID}, PolicyDigest: digest}
	copy(framePolicy.DefinitionDigest[:], definition)
	frame, err := sharedhandoff.OpenDockerFrameReader(connectedprofile.RichProcessesDockerFrameDirectory, framePolicy)
	if err != nil {
		return 22
	}
	defer func() {
		if frame.Close() != nil {
			code = 22
		}
	}()
	status, err := connectedstatus.Open(connectedprofile.RichProcessesDockerStateDirectory + "/collector-status")
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
		if currentPolicy(expected, digest) != nil {
			return 22
		}
		cycle, cancel := context.WithTimeout(ctx, 10*time.Second)
		raw := provider.Collect(cycle)
		cancel()
		if ctx.Err() != nil {
			break
		}
		section := readContainers(ctx, frame, c.ServerID, time.Now().UTC())
		if ctx.Err() != nil {
			break
		}
		body, err := encodeDocker(raw, c.ServerID, identity.Version, section)
		if err != nil {
			return 22
		}
		// Revalidate consent after collection and before publishing sensitive rows.
		if currentPolicy(expected, digest) != nil {
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
