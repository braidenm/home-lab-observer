//go:build linux

package connectedstartup

import (
	"context"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
)

// RunRichProcessesDocker uses the existing same-process startup barrier, bound
// to the separately enrolled immutable Docker profile before any secret access.
func RunRichProcessesDocker(parent context.Context, c connectedprofile.RichProcessesDockerConfig, status *connectedstatus.Writer) error {
	if parent == nil || parent.Err() != nil || status == nil || c.Validate() != nil {
		return ErrUnsafe
	}
	configuration, err := dockerConfiguration(c)
	if err != nil {
		return ErrUnsafe
	}
	return runBound(parent, status, startupBinding{configuration: configuration, artifact: c.ArtifactSHA256, generation: c.PolicyGeneration, gid: c.UploaderGID, addresses: c.Addresses, current: func() ([]byte, error) {
		current, err := connectedprofile.LoadRichProcessesDockerConfig()
		if err != nil {
			return nil, ErrUnsafe
		}
		return dockerConfiguration(current)
	}})
}

func dockerConfiguration(c connectedprofile.RichProcessesDockerConfig) ([]byte, error) {
	p, err := connectedprofile.LoadRichProcessesDockerPolicy()
	if err != nil {
		return nil, ErrUnsafe
	}
	return dockerConfigurationWithPolicy(c, p)
}

func dockerConfigurationWithPolicy(c connectedprofile.RichProcessesDockerConfig, p connectedprofile.RichProcessesDockerPolicy) ([]byte, error) {
	if connectedprofile.VerifyRichProcessesDockerBinding(c, p) != nil {
		return nil, ErrUnsafe
	}
	return connectedprofile.EncodeRichProcessesDockerConfig(c)
}
