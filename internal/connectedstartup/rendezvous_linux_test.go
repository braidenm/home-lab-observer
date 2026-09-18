//go:build linux

package connectedstartup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedactivation"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func TestRendezvousRefusesBeforeLaterPhases(t *testing.T) {
	c := connectedprofile.Config{Version: connectedprofile.Version, State: "INSTALLED_READY", ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32), CollectorUID: 60101, UploaderUID: 60102, UploaderGID: 60102, SharedGID: 60103, ArtifactSHA256: strings.Repeat("c", 64), PolicyGeneration: 1, Addresses: []string{"1.1.1.1"}}
	encoded, _ := connectedprofile.Encode(c)
	hash := sha256.Sum256(encoded)
	request := connectedactivation.Request{Version: connectedactivation.RequestVersion, Nonce: strings.Repeat("d", 64), ArtifactSHA256: c.ArtifactSHA256, ConfigSHA256: hex.EncodeToString(hash[:]), PolicyGeneration: 1, IPv4Port: 40001, IPv6Port: 40002}
	for _, tc := range []struct {
		name string
		want []string
	}{
		{"missing-request", []string{"request.json"}},
		{"malformed-request", []string{"request.json"}},
		{"wrong-binding", []string{"request.json"}},
		{"random-failure", []string{"request.json"}},
		{"invocation-failure", []string{"request.json"}},
		{"probe-failure", []string{"request.json", "prove"}},
		{"cancel-after-probe", []string{"request.json", "prove"}},
		{"publish-failure", []string{"request.json", "prove", "publish"}},
		{"missing-commit", []string{"request.json", "prove", "publish", "commit.json"}},
		{"stale-challenge", []string{"request.json", "prove", "publish", "commit.json"}},
		{"wrong-generation", []string{"request.json", "prove", "publish", "commit.json", "current"}},
		{"success", []string{"request.json", "prove", "publish", "commit.json", "current"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var calls []string
			var response connectedactivation.Response
			p := startupPorts{random: bytes.NewReader(bytes.Repeat([]byte{0xaa}, 32)), invocation: strings.Repeat("e", 32)}
			if tc.name == "random-failure" {
				p.random = bytes.NewReader(nil)
			}
			if tc.name == "invocation-failure" {
				p.invocation = ""
			}
			p.read = func(_ context.Context, name string) ([]byte, error) {
				calls = append(calls, name)
				if name == "request.json" {
					if tc.name == "missing-request" {
						return nil, ErrUnsafe
					}
					if tc.name == "malformed-request" {
						return []byte("{}"), nil
					}
					r := request
					if tc.name == "wrong-binding" {
						r.PolicyGeneration++
					}
					return connectedactivation.EncodeRequest(r)
				}
				if name != "commit.json" {
					t.Fatal("read of untrusted response authority")
				}
				if tc.name == "missing-commit" {
					return nil, ErrUnsafe
				}
				commit := connectedactivation.Commit{Version: connectedactivation.CommitVersion, RequestSHA256: response.RequestSHA256, InvocationID: response.InvocationID, Challenge: response.Challenge}
				if tc.name == "stale-challenge" {
					commit.Challenge = strings.Repeat("f", 64)
				}
				return connectedactivation.EncodeCommit(commit)
			}
			p.prove = func(context.Context, connectedactivation.Request) error {
				calls = append(calls, "prove")
				if tc.name == "cancel-after-probe" {
					cancel()
				}
				if tc.name == "probe-failure" {
					return ErrUnsafe
				}
				return nil
			}
			p.publish = func(b []byte) error {
				calls = append(calls, "publish")
				var err error
				response, err = connectedactivation.DecodeResponse(b)
				if err != nil {
					t.Fatal(err)
				}
				if tc.name == "publish-failure" {
					return ErrUnsafe
				}
				return nil
			}
			p.current = func() (connectedprofile.Config, error) {
				calls = append(calls, "current")
				copy := c
				if tc.name == "wrong-generation" {
					copy.PolicyGeneration++
				}
				return copy, nil
			}
			err := rendezvous(ctx, c, p)
			if tc.name == "success" {
				if err != nil {
					t.Fatal("matching commit refused", err)
				}
			} else if err != ErrUnsafe {
				t.Fatal("failed boundary released worker")
			}
			if !reflect.DeepEqual(calls, tc.want) {
				t.Fatalf("later phase executed: got%v want%v", calls, tc.want)
			}
		})
	}
}
