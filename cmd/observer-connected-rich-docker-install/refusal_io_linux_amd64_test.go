//go:build linux && amd64

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func TestDockerInstallRefusalOutput(t *testing.T) {
	const child = "--docker-refusal-output-child"
	if len(os.Args) == 4 && os.Args[3] == child {
		var err error
		releaseIdentity, err = connectedidentity.Encode(connectedidentity.Identity{
			Role: "rich-docker-install", Version: "0.1.0-canary.3", Commit: strings.Repeat("a", 40),
			OS: "linux", Arch: "amd64", ContractSHA256: connectedprofile.RichProcessesDockerDefinitionSHA256,
		})
		if err != nil {
			os.Exit(99)
		}
		os.Args = []string{"observer-connected-rich-docker-install", "install", "/unused", strings.Repeat("a", 64), "invalid-server"}
		// CheckRequest refuses nil before any host or bundle inspection. Run in
		// a child because secret hardening intentionally changes process state.
		os.Exit(run(nil))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDockerInstallRefusalOutput$", "--", child)
	var output, diagnostic bytes.Buffer
	command.Stdout, command.Stderr = &output, &diagnostic
	err := command.Run()
	var exit *exec.ExitError
	if ctx.Err() != nil || !errors.As(err, &exit) || exit.ExitCode() != 22 || output.Len() != 0 ||
		diagnostic.String() != "PREFLIGHT_REFUSED: verify privileges, supported host and rich bundle before enrollment.\n" {
		t.Fatal("fixed pre-grant refusal output contract changed")
	}
}
