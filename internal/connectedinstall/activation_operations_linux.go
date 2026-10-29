//go:build linux

package connectedinstall

import (
	"context"
	"crypto/rand"
	"os"

	"github.com/braidenm/home-lab-observer/internal/connectedactivation"
	"github.com/braidenm/home-lab-observer/internal/connectedpolicy"
	"github.com/braidenm/home-lab-observer/internal/connectedprocess"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedruntime"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
)

// Private operation boundaries permit deterministic transaction fault tests.
// Production always constructs these fixed adapters; no caller, config file or
// command argument can replace policy or select different paths/commands.
type activationOperations struct {
	pair          workerPair
	configBytes   func(connectedprofile.Config) ([]byte, error)
	inspect       func(context.Context, string) (workerInstance, error)
	service       func(context.Context, string, string) error
	audit         func(context.Context, int, string) (connectedprocess.Identity, error)
	pin           func(uint32) (activationRecords, error)
	clearResponse func(connectedprofile.Config) error
	response      func(connectedprofile.Config) (connectedactivation.Response, error)
	status        func(string, uint32) (connectedstatus.Record, error)
	probes        func(context.Context) (activationProbes, error)
	policy        func(context.Context, string, []string) error
	installed     func() (connectedprofile.Config, error)
	random        func([]byte) (int, error)
	wait          func(context.Context) error
}

type activationRecords interface {
	publish(string, []byte) error
	close()
}

type activationProbes interface {
	ports() (uint16, uint16)
	denied() bool
	finish(context.Context) error
	close()
}

type pinnedActivationRecords struct {
	directory *os.File
	gid       uint32
	pair      workerPair
}

func (p pinnedActivationRecords) publish(name string, data []byte) error {
	return publishActivationForPair(p.directory, name, data, p.gid, p.pair)
}
func (p pinnedActivationRecords) close()     { p.directory.Close() }
func (p *probePair) ports() (uint16, uint16) { return p.v4.port(), p.v6.port() }

func systemActivationOperations() activationOperations {
	return activationOperations{
		inspect: inspectWorker, service: workerService, audit: auditActivationProcess,
		pin: func(gid uint32) (activationRecords, error) {
			d, err := pinActivation(gid)
			if err != nil {
				return nil, err
			}
			return pinnedActivationRecords{directory: d, gid: gid}, nil
		},
		clearResponse: clearActivationResponse, response: readActivationResponse,
		status: readStatusRecord,
		probes: func(ctx context.Context) (activationProbes, error) { return newProbePair(ctx) },
		policy: connectedpolicy.ValidateEffective, installed: inspectInstalled,
		random: rand.Read, wait: activationWait,
	}
}

func auditActivationProcess(ctx context.Context, pid int, executable string) (connectedprocess.Identity, error) {
	before, err := connectedprocess.Audit(ctx, pid, executable)
	if err != nil {
		return before, ErrUnsafe
	}
	c, err := connectedprofile.Load()
	if err != nil {
		return before, ErrUnsafe
	}
	sources := map[string]string{
		"/bin/observer-connected-uploader":                connectedprofile.ReleaseDirectory + "/" + c.ArtifactSHA256 + "/observer-connected-uploader",
		"/etc/home-lab-observer-connected/installed.json": connectedprofile.ConfigPath,
		"/handoff":      connectedprofile.StateDirectory + "/handoff",
		"/activation":   "/run/home-lab-observer-connected/activation",
		"/state/ledger": connectedprofile.StateDirectory + "/ledger",
		"/state/status": connectedprofile.StateDirectory + "/uploader-status",
	}
	if connectedruntime.CheckUploaderNamespace(pid, connectedprofile.StateDirectory+"/uploader-root", sources) != nil {
		return before, ErrUnsafe
	}
	after, err := connectedprocess.Audit(ctx, pid, executable)
	if err != nil || before != after {
		return before, ErrUnsafe
	}
	return after, nil
}
