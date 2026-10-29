//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedinstall"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

var releaseIdentity string

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	code := run(ctx)
	cancel()
	os.Exit(code)
}

func run(ctx context.Context) int {
	if connectedprofile.HardenSecretProcess() != nil {
		return 22
	}
	if _, err := connectedidentity.Resolve(releaseIdentity, "install"); err != nil {
		return 22
	}
	if len(os.Args) == 2 {
		return runLifecycle(ctx, os.Args[1], os.Stdout, os.Stderr)
	}
	if len(os.Args) != 5 || os.Args[1] != "install" {
		fmt.Fprintln(os.Stderr, "Usage: observer-connected-install install <verified-bundle-directory> <manifest-sha256> <server-id> | start | stop | status")
		return 22
	}
	if connectedinstall.CheckRequest(ctx, os.Args[2], os.Args[3], os.Args[4]) != nil {
		fmt.Fprintln(os.Stderr, "PREFLIGHT_REFUSED: verify privileges, supported host and bundle before enrollment.")
		return 22
	}
	grant, err := readGrant(ctx)
	if err != nil {
		return 22
	}
	defer clear(grant)
	err = connectedinstall.Install(ctx, connectedinstall.Request{BundleDirectory: os.Args[2], ManifestSHA256: os.Args[3], ServerID: os.Args[4], Grant: grant})
	if err != nil {
		var phase *connectedinstall.PhaseError
		if errors.As(err, &phase) && phase.Phase == "PREFLIGHT_REFUSED" {
			fmt.Fprintln(os.Stderr, "PREFLIGHT_REFUSED: no enrollment attempted; verify bundle, supported host and network prerequisites.")
		} else if errors.As(err, &phase) && phase.Phase == "LOCAL_SETUP_INCOMPLETE" {
			fmt.Fprintln(os.Stderr, "LOCAL_SETUP_INCOMPLETE: enrollment was not attempted; preserve partial local setup for explicit recovery.")
		} else {
			fmt.Fprintln(os.Stderr, "ENROLLMENT_RECOVERY_REQUIRED: exchange may have occurred; preserve setup and revoke registration before new enrollment.")
		}
		return 22
	}
	fmt.Fprintln(os.Stdout, "INSTALLED_PENDING_ACCEPTANCE: services remain stopped; packaged isolation acceptance is required before activation.")
	return 0
}

func runLifecycle(ctx context.Context, operation string, output, diagnostic io.Writer) int {
	var err error
	switch operation {
	case "start":
		err = connectedinstall.Start(ctx)
		if err == nil {
			fmt.Fprintln(output, "WAITING_FIRST_UPLOAD: workers started; check status for receiver acknowledgement.")
		}
	case "stop":
		err = connectedinstall.Stop(ctx)
		if err == nil {
			fmt.Fprintln(output, "STOPPED: collector and uploader joined; delivery state retained.")
		}
	case "status":
		var status connectedinstall.Status
		status, err = connectedinstall.ReadStatus(ctx)
		if err == nil {
			err = json.NewEncoder(output).Encode(status)
		}
	default:
		fmt.Fprintln(diagnostic, "Usage: observer-connected-install start | stop | status")
		return 22
	}
	if err != nil {
		return lifecycleFailure(err, diagnostic)
	}
	return 0
}

func lifecycleFailure(err error, diagnostic io.Writer) int {
	if errors.Is(err, connectedinstall.ErrRecovery) {
		fmt.Fprintf(diagnostic, "RECOVERY_REQUIRED: %s. Preserve installed state and inspect the owned services before retrying.\n", connectedinstall.LifecyclePhase(err))
	} else {
		fmt.Fprintf(diagnostic, "OPERATION_REFUSED: %s. Verify the installed bundle, service state and policy.\n", connectedinstall.LifecyclePhase(err))
	}
	return 22
}

func readGrant(ctx context.Context) ([]byte, error) { return readGrantFrom(ctx, os.Stdin, os.Stderr) }

func readGrantFrom(ctx context.Context, terminal *os.File, prompt io.Writer) (result []byte, resultErr error) {
	return connectedinstall.ReadEnrollmentGrantFrom(ctx, terminal, prompt)
}
