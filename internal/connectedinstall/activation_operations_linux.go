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
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
)

// Private operation boundaries permit deterministic transaction fault tests.
// Production always constructs these fixed adapters; no caller, config file or
// command argument can replace policy or select different paths/commands.
type activationOperations struct {
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
}

func (p pinnedActivationRecords) publish(name string, data []byte) error {
	return publishActivation(p.directory, name, data, p.gid)
}
func (p pinnedActivationRecords) close()     { p.directory.Close() }
func (p *probePair) ports() (uint16, uint16) { return p.v4.port(), p.v6.port() }

func systemActivationOperations() activationOperations {
	return activationOperations{
		inspect: inspectWorker, service: service, audit: connectedprocess.Audit,
		pin: func(gid uint32) (activationRecords, error) {
			d, err := pinActivation(gid)
			if err != nil {
				return nil, err
			}
			return pinnedActivationRecords{d, gid}, nil
		},
		clearResponse: clearActivationResponse, response: readActivationResponse,
		status: readStatusRecord,
		probes: func(ctx context.Context) (activationProbes, error) { return newProbePair(ctx) },
		policy: connectedpolicy.ValidateEffective, installed: inspectInstalled,
		random: rand.Read, wait: activationWait,
	}
}
