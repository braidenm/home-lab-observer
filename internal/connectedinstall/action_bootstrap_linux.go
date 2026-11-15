//go:build linux

package connectedinstall

import (
	"context"
	"io"
	"os/exec"
	"time"

	"github.com/braidenm/home-lab-observer/packaging/actions"
)

func runLocalActionBootstrap(ctx context.Context, verb, digest string, gid uint32, network actionNetworkResources) error {
	if ctx == nil || ctx.Err() != nil || (verb != "provision-journal" && verb != "seed-enrollment") {
		return ErrUnsafe
	}
	props, err := actions.BootstrapProperties(digest, verb, gid, network.addresses)
	if err != nil {
		return ErrUnsafe
	}
	args := []string{"--quiet", "--wait", "--collect", "--no-ask-password", "--expand-environment=no",
		"--unit=home-lab-observer-action-" + verb + ".service"}
	for _, property := range props {
		args = append(args, "--property="+property)
	}
	args = append(args, "--property=StandardInput=null", "--property=StandardOutput=null",
		"--property=StandardError=null", "--", "/bin/observer-action-setup", verb)
	child, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(child, "/usr/bin/systemd-run", args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	cmd.WaitDelay = time.Second
	if cmd.Run() != nil || child.Err() != nil {
		// The manager also applies its own runtime bound. Refusal never
		// clears the durable state or reruns the operation.
		return ErrRecovery
	}
	return nil
}
