//go:build linux

package connectedinstall

import "context"

const actionWorkerUnit = "home-lab-observer-action-worker.service"
const actionBrokerUnit = "home-lab-observer-action-broker.service"
const systemUnitDirectory = "/etc/systemd/system"

// The caller holds the installation lease and provides resources from the
// verified action template renderer. Never starts or enables either service.
func registerStoppedActionUnits(ctx context.Context, network actionNetworkResources) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrUnsafe
	}
	parent, err := rootDirectory(systemUnitDirectory)
	if err != nil {
		return ErrUnsafe
	}
	defer parent.Close()
	for _, item := range []struct{ name, content string }{
		{actionBrokerUnit, network.brokerUnit},
		{actionWorkerUnit, network.workerUnit},
	} {
		if ctx.Err() != nil || publishFileAt(parent, fileSpec{name: item.name, mode: 0644, limit: 16384}, []byte(item.content), nil) != nil {
			return ErrRecovery
		}
	}
	if _, err := runTool(ctx, toolSystemctl, []string{"daemon-reload"}, nil, 1024); err != nil {
		return ErrRecovery
	}
	return verifyStoppedActionUnits(ctx)
}

func verifyStoppedActionUnits(ctx context.Context) error {
	for _, unit := range []string{actionBrokerUnit, actionWorkerUnit} {
		out, err := runTool(ctx, toolSystemctl, []string{"show", "--no-pager",
			"--property=Id,LoadState,ActiveState,UnitFileState,FragmentPath,DropInPaths,NeedDaemonReload", "--", unit}, nil, 1024)
		if err != nil || !isStoppedActionUnit(out, unit) {
			return ErrRecovery
		}
	}
	return nil
}

func isStoppedActionUnit(raw []byte, unit string) bool {
	if unit != actionWorkerUnit && unit != actionBrokerUnit {
		return false
	}
	state, err := parseUnitRuntimeState(raw, unit)
	return err == nil && len(state) == 7 && state["LoadState"] == "loaded" &&
		state["ActiveState"] == "inactive" && state["UnitFileState"] == "static" &&
		state["FragmentPath"] == systemUnitDirectory+"/"+unit && state["DropInPaths"] == "" &&
		state["NeedDaemonReload"] == "no"
}
