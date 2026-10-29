//go:build linux

package connectedstartup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedactivation"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func TestRichStartupRequiresExactConfigBeforeSameProcessCommit(t *testing.T) {
	configuration := []byte(`{"synthetic":"rich-installed-config"}`)
	digest := sha256.Sum256(configuration)
	request := connectedactivation.Request{Version: connectedactivation.RequestVersion, Nonce: strings.Repeat("a", 64), ArtifactSHA256: strings.Repeat("b", 64), ConfigSHA256: hex.EncodeToString(digest[:]), PolicyGeneration: 1, IPv4Port: 40001, IPv6Port: 40002}
	for _, change := range []string{"valid", "foreign-config", "changed-current", "missing-commit", "foreign-challenge"} {
		t.Run(change, func(t *testing.T) {
			var response connectedactivation.Response
			proofs := 0
			p := boundStartupPorts{random: bytes.NewReader(bytes.Repeat([]byte{1}, 32)), invocation: strings.Repeat("c", 32), current: func() ([]byte, error) {
				if change == "changed-current" {
					return []byte(`{"changed":true}`), nil
				}
				return configuration, nil
			}}
			p.read = func(_ context.Context, name string) ([]byte, error) {
				if name == "request.json" {
					r := request
					if change == "foreign-config" {
						r.ConfigSHA256 = strings.Repeat("d", 64)
					}
					return connectedactivation.EncodeRequest(r)
				}
				if change == "missing-commit" {
					return nil, ErrUnsafe
				}
				commit := connectedactivation.Commit{Version: connectedactivation.CommitVersion, RequestSHA256: response.RequestSHA256, InvocationID: response.InvocationID, Challenge: response.Challenge}
				if change == "foreign-challenge" {
					commit.Challenge = strings.Repeat("e", 64)
				}
				return connectedactivation.EncodeCommit(commit)
			}
			p.prove = func(context.Context, connectedactivation.Request) error { proofs++; return nil }
			p.publish = func(b []byte) error { var err error; response, err = connectedactivation.DecodeResponse(b); return err }
			err := rendezvousBound(context.Background(), configuration, request.ArtifactSHA256, 1, p)
			if (change == "valid" && err != nil) || (change != "valid" && err != ErrUnsafe) {
				t.Fatal("invalid rich startup barrier result")
			}
			if change == "foreign-config" && proofs != 0 {
				t.Fatal("foreign config reached network proof")
			}
		})
	}
	if RunRichProcesses(nil, connectedprofile.RichProcessesConfig{}, nil) != ErrUnsafe {
		t.Fatal("invalid rich startup context admitted")
	}
}
