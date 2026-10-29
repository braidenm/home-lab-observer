package enrollmentstore

import (
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

// RichProcessesDockerPrincipals pins only the one separately approved Docker
// profile. Neither callers nor persisted files supply a profile identifier.
type RichProcessesDockerPrincipals struct {
	CollectorUID, UploaderUID, UploaderGID, SharedGID, ReaderUID, FrameGID, SocketGID uint32
}

func (p RichProcessesDockerPrincipals) zero() bool { return p == (RichProcessesDockerPrincipals{}) }
func (p RichProcessesDockerPrincipals) profile(binding uploadstate.Binding) (uploadstate.ProfileBinding, error) {
	policy, err := connectedprofile.NewRichProcessesDockerPolicy(binding.ServerID, binding.ConnectorID, p.CollectorUID, p.UploaderUID, p.UploaderGID, p.SharedGID, p.ReaderUID, p.FrameGID, p.SocketGID)
	if err != nil {
		return uploadstate.ProfileBinding{}, ErrRecovery
	}
	digest, err := connectedprofile.DigestRichProcessesDockerPolicy(policy)
	if err != nil {
		return uploadstate.ProfileBinding{}, ErrRecovery
	}
	return uploadstate.ProfileBinding{Kind: uploadstate.RichProcessesDockerProfile, PolicyDigest: digest}, nil
}
