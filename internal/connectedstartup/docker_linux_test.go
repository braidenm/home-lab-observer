//go:build linux

package connectedstartup

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func TestDockerStartupBindsCanonicalConfigAndBothConsentGroups(t *testing.T) {
	p, err := connectedprofile.NewRichProcessesDockerPolicy("srv_0123456789abcdef0123456789abcdef", "agent_0123456789abcdef0123456789abcdef", 60101, 60102, 60103, 60104, 60105, 60106, 999)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := connectedprofile.DigestRichProcessesDockerPolicy(p)
	c := connectedprofile.RichProcessesDockerConfig{Version: connectedprofile.RichProcessesDockerVersion, State: "INSTALLED_READY", Profile: connectedprofile.RichProcessesDockerProfile, ServerID: p.ServerID, ConnectorID: p.ConnectorID, CollectorUID: p.CollectorUID, UploaderUID: p.UploaderUID, UploaderGID: p.UploaderGID, SharedGID: p.SharedGID, ReaderUID: p.ReaderUID, FrameGID: p.FrameGID, SocketGID: p.SocketGID, DefinitionSHA256: p.DefinitionSHA256, PolicySHA256: hex.EncodeToString(digest[:]), ArtifactSHA256: strings.Repeat("a", 64), PolicyGeneration: 1, Addresses: []string{"1.1.1.1"}}
	got, err := dockerConfigurationWithPolicy(c, p)
	want, _ := connectedprofile.EncodeRichProcessesDockerConfig(c)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("startup digest input is not canonical Docker configuration")
	}
	for _, changed := range []string{"policy", "container-group", "reader", "socket", "profile"} {
		bad := p
		switch changed {
		case "policy":
			bad.ConnectorID = "agent_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		case "container-group":
			bad.ContainersEnabled = false
		case "reader":
			bad.ReaderUID++
		case "socket":
			bad.SocketGID++
		case "profile":
			bad.Profile = connectedprofile.RichProcessesProfile
		}
		if _, err := dockerConfigurationWithPolicy(c, bad); err != ErrUnsafe {
			t.Fatal("startup accepted unbound Docker policy")
		}
	}
	if RunRichProcessesDocker(nil, c, nil) != ErrUnsafe {
		t.Fatal("missing startup barrier accepted")
	}
}
