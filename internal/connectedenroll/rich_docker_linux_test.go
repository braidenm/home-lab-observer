//go:build linux

package connectedenroll

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func dockerGrantFixture() RichDockerGrantInput {
	return RichDockerGrantInput{ServerID: "srv_" + strings.Repeat("a", 32), Grant: "hle_" + strings.Repeat("b", 43), CollectorUID: 62001, UploaderUID: 62002, UploaderGID: 62003, SharedGID: 62004, ReaderUID: 62005, FrameGID: 62006, SocketGID: 998, DefinitionSHA256: connectedprofile.RichProcessesDockerDefinitionSHA256}
}
func TestDockerPrivateInputIsCanonicalAndCannotBecomeOldProfile(t *testing.T) {
	g := dockerGrantFixture()
	b, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RichDockerGrantInput
	if input(bytes.NewReader(b), &decoded) != nil || decoded != g {
		t.Fatal("canonical Docker grant refused")
	}
	var old RichGrantInput
	if input(bytes.NewReader(b), &old) == nil {
		t.Fatal("Docker input reached old profile")
	}
	for _, change := range []func(*RichDockerGrantInput){func(g *RichDockerGrantInput) { g.DefinitionSHA256 = connectedprofile.RichProcessesDefinitionSHA256 }, func(g *RichDockerGrantInput) { g.ReaderUID = g.UploaderUID }, func(g *RichDockerGrantInput) { g.FrameGID = g.SocketGID }, func(g *RichDockerGrantInput) { g.SocketGID = 0 }} {
		g := dockerGrantFixture()
		change(&g)
		b, _ := json.Marshal(g)
		var out bytes.Buffer
		if RunRichProcessesDocker(context.Background(), "enroll", bytes.NewReader(b), &out) != 22 || out.Len() != 0 {
			t.Fatal("invalid Docker approval reached state/exchange")
		}
	}
	for _, mode := range []string{"enroll", "validate-enrollment", "validate-ledger", "validate-existing-ledger", "other"} {
		var out bytes.Buffer
		if RunRichProcessesDocker(context.Background(), mode, strings.NewReader(`{}`), &out) != 22 || out.Len() != 0 {
			t.Fatal("invalid Docker mode/input emitted output")
		}
	}
}
