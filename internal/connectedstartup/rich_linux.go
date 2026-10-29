//go:build linux

package connectedstartup

import (
	"context"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
)

// RunRichProcesses preserves the same-process request/response/commit barrier,
// using the exact rich installed configuration digest and immutable local policy.
// It never reads a credential, opens a ledger or admits ordinary upload work.
func RunRichProcesses(parent context.Context, c connectedprofile.RichProcessesConfig, status *connectedstatus.Writer) error {
	if parent == nil || parent.Err() != nil || status == nil || c.Validate() != nil {
		return ErrUnsafe
	}
	configuration, err := richConfiguration(c)
	if err != nil {
		return ErrUnsafe
	}
	return runBound(parent, status, startupBinding{configuration: configuration, artifact: c.ArtifactSHA256, generation: c.PolicyGeneration, gid: c.UploaderGID, addresses: c.Addresses, current: func() ([]byte, error) {
		current, err := connectedprofile.LoadRichProcessesConfig()
		if err != nil {
			return nil, ErrUnsafe
		}
		return richConfiguration(current)
	}})
}

func richConfiguration(c connectedprofile.RichProcessesConfig) ([]byte, error) {
	policy, err := connectedprofile.LoadRichProcessesPolicy()
	if err != nil || connectedprofile.VerifyRichProcessesBinding(c, policy) != nil {
		return nil, ErrUnsafe
	}
	return connectedprofile.EncodeRichProcessesConfig(c)
}
