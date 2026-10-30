//go:build linux

package connectedinstall

import "github.com/braidenm/home-lab-observer/internal/connectedprofile"

// This tuple is only the common operational identity; it is never persisted or
// used as numeric enrollment approval. Callers verify the Docker policy first.
func dockerWorkerBinding(c connectedprofile.RichProcessesDockerConfig) connectedprofile.Config {
	return connectedprofile.Config{Version: connectedprofile.Version, State: c.State, ServerID: c.ServerID, ConnectorID: c.ConnectorID, CollectorUID: c.CollectorUID, UploaderUID: c.UploaderUID, UploaderGID: c.UploaderGID, SharedGID: c.SharedGID, ArtifactSHA256: c.ArtifactSHA256, PolicyGeneration: c.PolicyGeneration, Addresses: c.Addresses}
}
