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

func richGrantFixture() RichGrantInput {
	return RichGrantInput{
		ServerID: "srv_" + strings.Repeat("a", 32), Grant: "hle_" + strings.Repeat("b", 43),
		CollectorUID: 60101, UploaderUID: 60102, UploaderGID: 60102, SharedGID: 60103,
		DefinitionSHA256: connectedprofile.RichProcessesDefinitionSHA256,
	}
}

func TestRichPrivateInputsAreDistinctCanonicalAndDefinitionBound(t *testing.T) {
	g := richGrantFixture()
	b, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RichGrantInput
	if input(bytes.NewReader(b), &decoded) != nil || decoded != g {
		t.Fatal("canonical rich grant refused")
	}
	var numeric GrantInput
	if input(bytes.NewReader(b), &numeric) == nil {
		t.Fatal("rich input admitted by numeric protocol")
	}
	changed := g
	changed.DefinitionSHA256 = strings.Repeat("0", 64)
	data, _ := json.Marshal(changed)
	var output bytes.Buffer
	if RunRichProcesses(context.Background(), "enroll", bytes.NewReader(data), &output) != 22 || output.Len() != 0 {
		t.Fatal("foreign definition reached rich enrollment")
	}
}

func TestRichModesFailClosedBeforeStateForInvalidPrincipal(t *testing.T) {
	for _, mode := range []string{"enroll", "validate-enrollment", "validate-ledger", "validate-existing-ledger", "unknown"} {
		var output bytes.Buffer
		if RunRichProcesses(context.Background(), mode, strings.NewReader(`{}`), &output) != 22 || output.Len() != 0 {
			t.Fatalf("mode %s succeeded or leaked output", mode)
		}
	}
	if RunRichProcesses(nil, "enroll", strings.NewReader(`{}`), &bytes.Buffer{}) != 22 || RunRichProcesses(context.Background(), "enroll", nil, &bytes.Buffer{}) != 22 {
		t.Fatal("missing rich pipe boundary accepted")
	}
}
