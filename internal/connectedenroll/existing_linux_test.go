//go:build linux

package connectedenroll

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/ledgerwitness"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

type witnessWriter func([]byte) (int, error)

func (w witnessWriter) Write(p []byte) (int, error) { return w(p) }

func TestExistingLedgerModePrivateBoundaries(t *testing.T) {
	expected := ValidationInput{ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32), UploaderUID: 60102, UploaderGID: 60102, SharedGID: 60103}
	encoded, _ := json.Marshal(expected)
	binding := uploadstate.Binding{ServerID: expected.ServerID, ConnectorID: expected.ConnectorID}
	for _, scenario := range []string{"success", "bad-input", "bad-binding", "principal", "cancel-identity", "canceled", "inspect-error", "cancel-inspect", "short-write", "write-error", "cancel-write"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "canceled" {
				cancel()
			}
			inputBytes := encoded
			if scenario == "bad-input" {
				inputBytes = append(append([]byte(nil), encoded...), '\n')
			}
			if scenario == "bad-binding" {
				bad := expected
				bad.ConnectorID = "bad"
				inputBytes, _ = json.Marshal(bad)
			}
			called := 0
			fingerprint := [32]byte{0xab}
			inspect := func(ctx context.Context, b uploadstate.Binding) ([32]byte, error) {
				called++
				if b != binding {
					t.Fatal("inspection received wrong binding")
				}
				if scenario == "inspect-error" {
					return [32]byte{}, errors.New("synthetic")
				}
				if scenario == "cancel-inspect" {
					cancel()
				}
				return fingerprint, nil
			}
			identity := func(u, g, s uint32) bool {
				if scenario == "cancel-identity" {
					cancel()
				}
				return scenario != "principal" && u == expected.UploaderUID && g == expected.UploaderGID && s == expected.SharedGID
			}
			var output bytes.Buffer
			var writer io.Writer = &output
			switch scenario {
			case "short-write":
				writer = witnessWriter(func(p []byte) (int, error) { return len(p) - 1, nil })
			case "write-error":
				writer = witnessWriter(func([]byte) (int, error) { return 0, errors.New("synthetic") })
			case "cancel-write":
				writer = witnessWriter(func(p []byte) (int, error) { cancel(); return len(p), nil })
			}
			code := validateExistingLedgerWith(ctx, bytes.NewReader(inputBytes), writer, identity, inspect)
			if scenario == "success" {
				got, err := ledgerwitness.DecodeResult(output.Bytes(), binding)
				if code != 0 || err != nil || got != fingerprint || called != 1 {
					t.Fatal("private validation success failed")
				}
			} else if code != 22 || output.Len() != 0 {
				t.Fatal("failed inspection exposed result or succeeded")
			}
			if (scenario == "bad-input" || scenario == "bad-binding" || scenario == "principal" || scenario == "cancel-identity" || scenario == "canceled") && called != 0 {
				t.Fatal("inspection preceded admission")
			}
		})
	}
}
