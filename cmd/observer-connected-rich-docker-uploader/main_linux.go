//go:build linux

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedcredential"
	"github.com/braidenm/home-lab-observer/internal/connectedenroll"
	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedstartup"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
	"github.com/braidenm/home-lab-observer/internal/platformtransport"
	"github.com/braidenm/home-lab-observer/internal/sharedhandoff"
	"github.com/braidenm/home-lab-observer/internal/uploadledger"
	"github.com/braidenm/home-lab-observer/internal/uploadloop"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

var releaseIdentity string

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	code := run(ctx)
	cancel()
	os.Exit(code)
}

func validateLaunch(environ []string, identity string, args []string) (string, error) {
	if connectedprofile.CheckEnvironment(environ) != nil {
		return "", connectedprofile.ErrUnsafe
	}
	if _, err := connectedidentity.Resolve(identity, "rich-docker-uploader"); err != nil {
		return "", err
	}
	if len(args) == 1 {
		return "upload", nil
	}
	if len(args) == 2 {
		switch args[1] {
		case "enroll", "validate-enrollment", "validate-ledger", "validate-existing-ledger":
			return args[1], nil
		}
	}
	return "", connectedprofile.ErrUnsafe
}

func run(ctx context.Context) (code int) {
	if connectedprofile.HardenSecretProcess() != nil {
		return 22
	}
	mode, err := validateLaunch(os.Environ(), releaseIdentity, os.Args)
	if err != nil {
		return 22
	}
	if mode != "upload" {
		return connectedenroll.RunRichProcessesDocker(ctx, mode, os.Stdin, os.Stdout)
	}
	c, err := connectedprofile.LoadRichProcessesDockerConfig()
	if err != nil || connectedprofile.CheckRichProcessesDockerIdentity(c, false) != nil {
		return 70
	}
	guard, err := newPolicyGuard(c, policyPorts{config: connectedprofile.LoadRichProcessesDockerConfig, policy: connectedprofile.LoadRichProcessesDockerPolicy, identity: func(c connectedprofile.RichProcessesDockerConfig) error {
		return connectedprofile.CheckRichProcessesDockerIdentity(c, false)
	}})
	if err != nil || guard.Check(ctx) != nil {
		return 22
	}
	status, err := connectedstatus.Open("/state/status")
	if err != nil {
		return 71
	}
	defer func() {
		if status.Close() != nil {
			code = 22
		}
	}()
	invocation := os.Getenv("INVOCATION_ID")
	if status.Write(connectedstatus.Record{Version: "observer-connected-status/v1", InvocationID: invocation, State: "STOPPED", UpdatedAt: time.Now().UTC()}) != nil {
		return 72
	}
	if err := connectedstartup.RunRichProcessesDocker(ctx, c, status); err != nil {
		return connectedstartup.ExitCode(err)
	}
	secret, err := guard.loadCredential(ctx, connectedcredential.LoadRichProcessesDocker)
	if err != nil {
		return 77
	}
	binding := uploadstate.Binding{ServerID: c.ServerID, ConnectorID: c.ConnectorID}
	profile := uploadstate.ProfileBinding{Kind: uploadstate.RichProcessesDockerProfile, PolicyDigest: guard.digest}
	ledger, err := uploadledger.OpenExistingForProfile(ctx, "/state/ledger", binding, profile)
	if err != nil {
		return 78
	}
	defer func() {
		if ledger.Close() != nil {
			code = 22
		}
	}()
	reader, err := sharedhandoff.OpenRichProcessesDockerReader("/handoff", sharedhandoff.RichProcessesDockerPolicy{Policy: sharedhandoff.Policy{CollectorUID: c.CollectorUID, UploaderUID: c.UploaderUID, SharedGID: c.SharedGID, ServerID: c.ServerID}, PolicyDigest: guard.digest})
	if err != nil {
		return 22
	}
	defer func() {
		if reader.Close() != nil {
			code = 22
		}
	}()
	transport, err := platformtransport.NewRichProcessesDocker(connectedprofile.Origin, credentialSource{record: secret, guard: guard})
	if err != nil {
		return 22
	}
	defer transport.CloseIdleConnections()
	observed := &observedTransport{delegate: transport, guard: guard}
	machine, err := uploadstate.NewForProfile(binding, profile, guardedSource{delegate: reader, guard: guard}, realClock{}, observed, guardedLedger{delegate: ledger, guard: guard})
	if err != nil {
		return 22
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	wrapper := &observedStepper{machine: machine, transport: observed, guard: guard, status: status, cancel: cancel, now: time.Now, record: connectedstatus.Record{Version: "observer-connected-status/v1", InvocationID: invocation, State: "WAITING_FIRST_UPLOAD", UpdatedAt: time.Now().UTC()}}
	if status.Write(wrapper.record) != nil {
		return 22
	}
	loop, err := uploadloop.New(wrapper)
	if err != nil {
		return 22
	}
	result, _ := loop.Run(child)
	if wrapper.failed {
		return 22
	}
	return finishUpload(result, status, wrapper.record)
}
