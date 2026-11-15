//go:build linux

package connectedinstall

import (
	"context"
	"os"
	"path/filepath"

	"github.com/braidenm/home-lab-observer/internal/actionengine"
	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedinstalllease"
)

// prepareActionConfig provisions only the stopped service accounts and
// configuration, then registers the inactive units. It does not mount
// namespaces, seed credentials, or claim readiness. Partial state is retained.
func prepareActionConfig(ctx context.Context, digest string, identity connectedidentity.Identity,
	inputs actionConfigInputs, approved actionengine.SocketWitness) error {
	if ctx == nil || ctx.Err() != nil || os.Getuid() != 0 || os.Geteuid() != 0 ||
		!digestPattern.MatchString(digest) {
		return ErrUnsafe
	}
	config, err := encodeActionConfig(inputs, approved)
	if err != nil {
		return err
	}
	lease, err := connectedinstalllease.Acquire()
	if err != nil {
		return err
	}
	defer lease.Close()
	release := filepath.Join(actionReleaseRoot, "releases", digest)
	root, err := rootDirectory(release)
	if err != nil {
		return ErrUnsafe
	}
	defer root.Close()
	if preflight(ctx) != nil || verifyActionStageSource(release, digest, identity) != nil ||
		actionPrincipalsAbsent(ctx) != nil {
		return ErrUnsafe
	}
	for _, path := range []string{actionConfigRoot, actionStateRoot, actionRunRoot} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return ErrUnsafe
		}
	}
	for _, unit := range []string{actionWorkerUnit, actionBrokerUnit} {
		raw, err := runTool(ctx, toolSystemctl, []string{"show", "--property=LoadState", "--value", "--", unit}, nil, 1024)
		if err != nil || string(raw) != "not-found\n" {
			return ErrUnsafe
		}
	}
	socket, err := captureDockerSocket(approved.GroupID)
	if err != nil {
		return ErrUnsafe
	}
	defer socket.Close()
	if socket.dev != approved.Device || socket.ino != approved.Inode {
		return ErrUnsafe
	}
	network, err := resolveActionNetwork(ctx, digest, approved.GroupID)
	if err != nil || !sameDockerSocket(socket) {
		return ErrUnsafe
	}
	i, err := createActionPrincipals(ctx, socket.gid)
	if err != nil || createActionDirectories(ctx, i) != nil || !sameDockerSocket(socket) {
		return ErrRecovery
	}
	if publishActionConfig(ctx, i, config, network) != nil || !sameDockerSocket(socket) {
		return ErrRecovery
	}
	return registerStoppedActionUnits(ctx, network)
}
