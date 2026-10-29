//go:build linux

package connectedinstall

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func TestDockerFreshInstallerBindsApprovedPrincipalTuple(t *testing.T) {
	l := dockerInstallLayout{richInstallLayout: richInstallLayout{installLayout{digest: strings.Repeat("c", 64), collectorUID: 62001, uploaderUID: 62002, uploaderGID: 62003, sharedGID: 62004}}, readerUID: 62005, frameGID: 62006, socketGID: 998}
	c, p, err := dockerConfigForPrincipals("srv_"+strings.Repeat("a", 32), "agent_"+strings.Repeat("b", 32), l, []string{"1.1.1.1"})
	if err != nil || connectedprofile.VerifyRichProcessesDockerBinding(c, p) != nil {
		t.Fatal("fresh Docker binding")
	}
	b := dockerWorkerBinding(c)
	if b.CollectorUID != c.CollectorUID || b.SharedGID != c.SharedGID || b.ArtifactSHA256 != c.ArtifactSHA256 {
		t.Fatal("operational tuple lost identity")
	}
	if c.Profile != connectedprofile.RichProcessesDockerProfile || !p.ProcessesEnabled || !p.ContainersEnabled {
		t.Fatal("Docker policy silently changed")
	}
	l.socketGID = l.sharedGID
	if _, _, err := dockerConfigForPrincipals(c.ServerID, c.ConnectorID, l, c.Addresses); err != ErrUnsafe {
		t.Fatal("daemon membership used for handoff authority")
	}
}
func TestDockerInstallerInvalidRequestDoesNotReachHostAndClearsGrant(t *testing.T) {
	grant := []byte("synthetic-private-input")
	err := InstallRichProcessesDocker(nil, Request{ServerID: "invalid", Grant: grant})
	var phase *PhaseError
	if !errors.As(err, &phase) || phase.Phase != "PREFLIGHT_REFUSED" {
		t.Fatal("invalid request phase")
	}
	for _, b := range grant {
		if b != 0 {
			t.Fatal("grant retained after refusal")
		}
	}
	if CheckRichProcessesDockerRequest(context.Background(), "unused", "unused", "invalid") != ErrUnsafe {
		t.Fatal("invalid request reached host")
	}
}
