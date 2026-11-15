//go:build linux

package connectedinstall

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionprofile"
	"github.com/braidenm/home-lab-observer/internal/actionremote"
	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedinstalllease"
	"github.com/braidenm/home-lab-observer/internal/connectedpolicy"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

// EnrollStoppedAction exchanges one action grant in the isolated worker view.
// The grant is never accepted through argv or environment or stored by root.
func EnrollStoppedAction(ctx context.Context, identity connectedidentity.Identity, digest string, terminal *os.File, prompt io.Writer) error {
	if ctx == nil || ctx.Err() != nil || os.Getuid() != 0 || os.Geteuid() != 0 ||
		!digestPattern.MatchString(digest) || terminal == nil || prompt == nil {
		return ErrUnsafe
	}
	lease, err := connectedinstalllease.Acquire()
	if err != nil {
		return err
	}
	defer lease.Close()
	release := filepath.Join(actionReleaseRoot, "releases", digest)
	dir, err := rootDirectory(release)
	if err != nil {
		return ErrUnsafe
	}
	defer dir.Close()
	if preflight(ctx) != nil || verifyActionStageSource(release, digest, identity) != nil ||
		actionprofile.VerifyHostNamespace() != nil || verifyStoppedActionUnits(ctx) != nil {
		return ErrUnsafe
	}
	socket, err := discoverDockerSocket()
	if err != nil {
		return ErrUnsafe
	}
	defer socket.Close()
	i, err := actionprofile.InspectInstallPrincipals(socket.gid)
	if err != nil {
		return ErrUnsafe
	}
	network, err := readInstalledActionNetwork(ctx, digest, socket.gid)
	if err != nil {
		return err
	}
	wait, cancel := context.WithTimeout(ctx, 2*time.Minute)
	raw, err := readFixedGrantFrom(wait, terminal, prompt, 81, "One-use Docker action enrollment grant (paste only hlae_ value): ")
	cancel()
	defer clear(raw)
	if err != nil {
		return ErrUnsafe
	}
	_, err = actionremote.NewEnrollmentSecret(string(raw))
	if err != nil || ctx.Err() != nil || verifyStoppedActionUnits(ctx) != nil || !sameDockerSocket(socket) {
		return ErrUnsafe
	}
	input, err := actionGrantPipe(raw, i)
	if err != nil {
		return err
	}
	defer input.Close()
	clear(raw)
	if runActionBootstrap(ctx, "exchange", digest, socket.gid, network, input) != nil ||
		verifyStoppedActionUnits(ctx) != nil || !sameDockerSocket(socket) ||
		runActionBootstrap(ctx, "verify-enrollment", digest, socket.gid, network, nil) != nil ||
		verifyStoppedActionUnits(ctx) != nil {
		return ErrRecovery
	}
	return nil
}

func actionGrantPipe(raw []byte, i actionprofile.Identity) (*os.File, error) {
	if len(raw) != 81 || i.WorkerUID == 0 || i.WorkerGID == 0 {
		return nil, ErrUnsafe
	}
	input, writer, err := os.Pipe()
	if err != nil {
		return nil, ErrRecovery
	}
	if err := input.Chown(int(i.WorkerUID), int(i.WorkerGID)); err != nil || input.Chmod(0600) != nil {
		writer.Close()
		input.Close()
		return nil, ErrRecovery
	}
	n, err := writer.Write(raw)
	closeErr := writer.Close()
	if n != len(raw) || err != nil || closeErr != nil {
		input.Close()
		return nil, ErrRecovery
	}
	return input, nil
}

func readInstalledActionNetwork(ctx context.Context, digest string, engineGID uint32) (actionNetworkResources, error) {
	raw, err := connectedprofile.ReadRootFileMode(actionConfigRoot+"/worker-hosts", 1024, 0644)
	if err != nil {
		return actionNetworkResources{}, ErrUnsafe
	}
	var addresses []string
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		host, name, ok := strings.Cut(line, " ")
		if !ok || name != connectedprofile.Hostname {
			return actionNetworkResources{}, ErrUnsafe
		}
		addresses = append(addresses, host)
	}
	network, err := renderActionNetwork(digest, engineGID, addresses)
	if err != nil || network.hosts != string(raw) || connectedpolicy.ValidateParent(ctx, addresses) != nil {
		return actionNetworkResources{}, ErrUnsafe
	}
	for _, file := range []struct{ name, want string }{{actionWorkerUnit, network.workerUnit}, {actionBrokerUnit, network.brokerUnit}} {
		got, err := connectedprofile.ReadRootFileMode(systemUnitDirectory+"/"+file.name, 16384, 0644)
		if err != nil || string(got) != file.want {
			return actionNetworkResources{}, ErrUnsafe
		}
	}
	return network, nil
}
