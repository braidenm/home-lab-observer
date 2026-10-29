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

func TestDockerReaderCannotLeaveBarrierWithoutCurrentSameProcessCommit(t *testing.T) {
	configuration := []byte(`{"version":"observer-connected-docker-reader-startup/v1","role":"rich-docker-reader","configuration":{"synthetic":"docker-reader-binding"}}`)
	digest := sha256.Sum256(configuration)
	request := connectedactivation.Request{Version: connectedactivation.RequestVersion, Nonce: strings.Repeat("a", 64), ArtifactSHA256: strings.Repeat("b", 64), ConfigSHA256: hex.EncodeToString(digest[:]), PolicyGeneration: 1, IPv4Port: 1024, IPv6Port: 1024}
	for _, change := range []string{"valid", "missing-request", "missing-commit", "foreign-config", "raw-config", "foreign-role", "foreign-version", "changed-config", "stale-challenge", "foreign-invocation", "ipv4-destination", "ipv6-destination", "canceled"} {
		t.Run(change, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var response connectedactivation.Response
			proofs, published := 0, 0
			p := boundStartupPorts{random: bytes.NewReader(bytes.Repeat([]byte{1}, 32)), invocation: strings.Repeat("c", 32), current: func() ([]byte, error) {
				if change == "changed-config" {
					return []byte(`{"foreign":true}`), nil
				}
				return configuration, nil
			}}
			p.read = func(_ context.Context, name string) ([]byte, error) {
				if name == "request.json" {
					if change == "missing-request" {
						return nil, ErrUnsafe
					}
					r := request
					switch change {
					case "foreign-config":
						r.ConfigSHA256 = strings.Repeat("d", 64)
					case "raw-config", "foreign-role", "foreign-version":
						foreign := []byte(`{"synthetic":"docker-reader-binding"}`)
						if change == "foreign-role" {
							foreign = bytes.Replace(configuration, []byte(`"role":"rich-docker-reader"`), []byte(`"role":"rich-docker-uploader"`), 1)
						}
						if change == "foreign-version" {
							foreign = bytes.Replace(configuration, []byte("reader-startup/v1"), []byte("reader-startup/v2"), 1)
						}
						hash := sha256.Sum256(foreign)
						r.ConfigSHA256 = hex.EncodeToString(hash[:])
					case "ipv4-destination":
						r.IPv4Port = 40001
					case "ipv6-destination":
						r.IPv6Port = 40002
					}
					return connectedactivation.EncodeRequest(r)
				}
				if change == "missing-commit" {
					return nil, ErrUnsafe
				}
				commit := connectedactivation.Commit{Version: connectedactivation.CommitVersion, RequestSHA256: response.RequestSHA256, InvocationID: response.InvocationID, Challenge: response.Challenge}
				if change == "stale-challenge" {
					commit.Challenge = strings.Repeat("e", 64)
				}
				if change == "foreign-invocation" {
					commit.InvocationID = strings.Repeat("f", 32)
				}
				return connectedactivation.EncodeCommit(commit)
			}
			p.prove = func(context.Context, connectedactivation.Request) error {
				proofs++
				if change == "canceled" {
					cancel()
				}
				return nil
			}
			p.publish = func(b []byte) error {
				published++
				var err error
				response, err = connectedactivation.DecodeResponse(b)
				return err
			}
			err := rendezvousDockerReader(ctx, configuration, request.ArtifactSHA256, request.PolicyGeneration, p)
			if (err == nil) != (change == "valid") {
				t.Fatal("reader left a refused startup barrier")
			}
			if (change == "missing-request" || change == "foreign-config" || change == "raw-config" || change == "foreign-role" || change == "foreign-version" || change == "ipv4-destination" || change == "ipv6-destination") && (proofs != 0 || published != 0) {
				t.Fatal("unapproved request reached primitive proof")
			}
			if change == "canceled" && published != 0 {
				t.Fatal("canceled proof published startup success")
			}
		})
	}
	if RunRichProcessesDockerReader(nil, connectedprofile.RichProcessesDockerConfig{}, nil) != ErrUnsafe {
		t.Fatal("missing reader context authorized startup")
	}
}
