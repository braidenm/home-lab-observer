//go:build linux

package connectedinstall

import (
	"context"

	"github.com/braidenm/home-lab-observer/internal/actionbroker"
	"github.com/braidenm/home-lab-observer/internal/actionengine"
	"github.com/braidenm/home-lab-observer/internal/actionjournal"
	"github.com/braidenm/home-lab-observer/internal/actionprofile"
	"github.com/braidenm/home-lab-observer/internal/actionremote"
	"github.com/braidenm/home-lab-observer/internal/actionworker"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

const actionConfigRoot = "/etc/home-lab-observer-actions"

type actionConfigInputs struct {
	binding                       actionjournal.Binding
	target, generation, container string
	grace                         uint8
}

type actionConfigRecords struct {
	worker, policy, engine []byte
}

func encodeActionConfig(inputs actionConfigInputs, witness actionengine.SocketWitness) (actionConfigRecords, error) {
	b := inputs.binding
	worker, err := actionworker.EncodeInstallSettings(actionworker.Settings{
		Origin: connectedprofile.Origin,
		Binding: actionremote.Binding{
			ServerID: b.ServerID, OwnerUserID: b.OwnerID, ConnectorInstanceID: b.InstallationID,
		},
	})
	if err != nil {
		return actionConfigRecords{}, ErrUnsafe
	}
	policy, engine, err := actionbroker.InitialRecords(b, inputs.target, inputs.generation, inputs.container, witness, inputs.grace)
	if err != nil {
		return actionConfigRecords{}, ErrUnsafe
	}
	return actionConfigRecords{worker, policy, engine}, nil
}

func publishActionConfig(ctx context.Context, i actionprofile.Identity, config actionConfigRecords, network actionNetworkResources) error {
	if ctx == nil || ctx.Err() != nil || i.IPCGID == 0 || i.WorkerGID == 0 || i.BrokerGID == 0 ||
		i.IPCGID == i.WorkerGID || i.IPCGID == i.BrokerGID || i.WorkerGID == i.BrokerGID {
		return ErrUnsafe
	}
	parent, err := rootDirectory(actionConfigRoot)
	if err != nil {
		return ErrRecovery
	}
	defer parent.Close()
	for _, file := range []struct {
		spec fileSpec
		gid  uint32
		data []byte
	}{
		{fileSpec{name: "worker.json", mode: 0640, limit: 4096}, i.WorkerGID, config.worker},
		{fileSpec{name: "policy.json", mode: 0640, limit: 2 << 20}, i.BrokerGID, config.policy},
		{fileSpec{name: "engine.json", mode: 0640, limit: 4096}, i.BrokerGID, config.engine},
		{fileSpec{name: "worker-hosts", mode: 0644, limit: 1024}, 0, []byte(network.hosts)},
	} {
		if ctx.Err() != nil || publishGroupedFileAt(parent, i.IPCGID, file.gid, file.spec, file.data, nil) != nil {
			return ErrRecovery
		}
	}
	return nil
}
