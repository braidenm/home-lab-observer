//go:build linux

package connectedenroll

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPrivateInputIsCanonicalBoundedAndClosed(t *testing.T) {
	g := GrantInput{ServerID: "srv_" + strings.Repeat("a", 32), Grant: "hle_" + strings.Repeat("b", 43), UploaderUID: 60102, UploaderGID: 60102, SharedGID: 60103}
	data, _ := json.Marshal(g)
	var got GrantInput
	if err := input(bytes.NewReader(data), &got); err != nil || got != g {
		t.Fatal("canonical private input refused")
	}
	for _, bad := range [][]byte{append(append([]byte(nil), data...), '\n'), bytes.Repeat([]byte("x"), 513), []byte(`{"server_id":"synthetic","unknown":"private"}`), nil} {
		if input(bytes.NewReader(bad), &got) == nil {
			t.Fatal("ambiguous private input accepted")
		}
	}
}

func TestModeAndPrincipalRefusalBeforeStoreMutation(t *testing.T) {
	for _, mode := range []string{"enroll", "validate-enrollment", "validate-ledger", "validate-existing-ledger", "unknown"} {
		var output bytes.Buffer
		// Empty/zero principal is rejected before any fixed state path or transport
		// is opened. This test never invokes a live enrollment or manager unit.
		if Run(context.Background(), mode, strings.NewReader(`{}`), &output) != 22 || output.Len() != 0 {
			t.Fatal("invalid principal leaked output or succeeded")
		}
	}
	if Run(nil, "enroll", strings.NewReader(`{}`), &bytes.Buffer{}) != 22 || Run(context.Background(), "enroll", nil, &bytes.Buffer{}) != 22 {
		t.Fatal("missing boundary accepted")
	}
}
