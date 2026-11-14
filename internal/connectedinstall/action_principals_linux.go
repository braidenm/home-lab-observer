//go:build linux

package connectedinstall

import (
	"context"
	"os/user"

	"github.com/braidenm/home-lab-observer/internal/actionprofile"
)

func actionPrincipalsAbsent(ctx context.Context) error {
	for _, name := range []string{actionprofile.WorkerName, actionprofile.BrokerName, actionprofile.IPCName} {
		if _, err := user.Lookup(name); !missingUser(err) {
			return ErrUnsafe
		}
		if _, err := user.LookupGroup(name); !missingGroup(err) {
			return ErrUnsafe
		}
	}
	return ctx.Err()
}

func createActionPrincipals(ctx context.Context, engineGID uint32) (actionprofile.Identity, error) {
	if actionPrincipalsAbsent(ctx) != nil {
		return actionprofile.Identity{}, ErrUnsafe
	}
	for _, name := range []string{actionprofile.WorkerName, actionprofile.BrokerName, actionprofile.IPCName} {
		if _, err := runTool(ctx, toolGroupadd, []string{"--system", name}, nil, 1024); err != nil {
			return actionprofile.Identity{}, ErrRecovery
		}
	}
	for _, name := range []string{actionprofile.WorkerName, actionprofile.BrokerName} {
		if _, err := runTool(ctx, toolUseradd, []string{"--system", "--no-create-home", "--home-dir", "/nonexistent",
			"--shell", "/usr/sbin/nologin", "--gid", name, "--groups", actionprofile.IPCName, name}, nil, 1024); err != nil {
			return actionprofile.Identity{}, ErrRecovery
		}
	}
	return actionprofile.InspectInstallPrincipals(engineGID)
}
