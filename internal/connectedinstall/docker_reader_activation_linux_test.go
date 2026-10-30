//go:build linux

package connectedinstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedactivation"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
)

type readerFixture struct {
	t                 *testing.T
	fail              string
	events            []string
	active, committed bool
	foreign           bool
	cancel            context.CancelFunc
	audits            int
	request           connectedactivation.Request
	response          connectedactivation.Response
}

func (f *readerFixture) publish(name string, b []byte) error {
	f.events = append(f.events, name)
	if f.fail == name {
		return ErrUnsafe
	}
	switch name {
	case "request.json":
		if f.audits != 1 {
			f.t.Fatal("request released before paused descriptor audit")
		}
		var err error
		f.request, err = connectedactivation.DecodeRequest(b)
		if err != nil || f.request.IPv4Port != 1024 || f.request.IPv6Port != 1024 {
			f.t.Fatal("reader request changed")
		}
		digest, _ := connectedactivation.RequestDigest(f.request)
		f.response = connectedactivation.Response{Version: connectedactivation.ResponseVersion, RequestSHA256: digest, InvocationID: strings.Repeat("a", 32), Challenge: strings.Repeat("b", 64), Result: connectedactivation.Pass}
	case "commit.json":
		if f.audits != 2 {
			f.t.Fatal("commit released before fresh-response descriptor audit")
		}
		commit, err := connectedactivation.DecodeCommit(b)
		if err != nil || !connectedactivation.MatchCommit(f.request, f.response, commit) {
			f.t.Fatal("reader commit changed")
		}
		f.committed = true
	default:
		f.t.Fatal("unknown activation record")
	}
	return nil
}
func (f *readerFixture) close() { f.events = append(f.events, "close") }
func (f *readerFixture) operations() dockerReaderOperations {
	return dockerReaderOperations{
		inspect: func(_ context.Context, unit string) (workerInstance, error) {
			if unit != connectedunits.RichProcessesDockerReaderUnit {
				f.t.Fatal("reader targeted another unit")
			}
			if f.active {
				if f.foreign {
					return workerInstance{PID: 43, Invocation: strings.Repeat("c", 32), Active: "active"}, nil
				}
				return workerInstance{PID: 42, Invocation: strings.Repeat("a", 32), Active: "active"}, nil
			}
			return workerInstance{Active: "inactive"}, nil
		},
		service: func(_ context.Context, verb, unit string) error {
			if unit != connectedunits.RichProcessesDockerReaderUnit {
				f.t.Fatal("reader targeted another unit")
			}
			f.events = append(f.events, verb)
			if verb == "start" {
				f.active = true
				if f.fail == "uncertain-start" {
					return ErrUnsafe
				}
			} else if verb == "stop" {
				f.active = false
			} else {
				f.t.Fatal("unreviewed manager action")
			}
			return nil
		},
		pin: func(uint32) (activationRecords, error) { return f, nil }, clearResponse: func() error { return nil },
		response: func() (connectedactivation.Response, error) {
			if f.fail == "cancelled" {
				f.cancel()
			}
			if f.fail == "replaced" {
				f.foreign = true
			}
			if f.fail == "missing-response" {
				return connectedactivation.Response{}, os.ErrNotExist
			}
			r := f.response
			if f.fail == "stale-response" {
				r.InvocationID = strings.Repeat("c", 32)
			}
			return r, nil
		},
		audit: func(context.Context, workerInstance) error {
			f.audits++
			f.events = append(f.events, "audit")
			if f.fail == "audit-before" && f.audits == 1 || f.fail == "audit-response" && f.audits == 2 {
				return ErrUnsafe
			}
			return nil
		},
		status: func() (connectedstatus.Record, error) {
			if !f.committed {
				f.t.Fatal("reader status accepted before commit")
			}
			r := connectedstatus.Record{InvocationID: strings.Repeat("a", 32), State: "COLLECTING"}
			if f.fail == "stale-status" {
				r.InvocationID = strings.Repeat("c", 32)
			}
			return r, nil
		},
		unchanged: func(context.Context, workerInstance) bool {
			return !f.foreign && !(f.fail == "changed-after-commit" && f.committed)
		},
		random: func(b []byte) (int, error) {
			for i := range b {
				b[i] = 1
			}
			return len(b), nil
		}, wait: func(context.Context) error { return ErrUnsafe },
	}
}
func dockerLifecycleConfig(t *testing.T) connectedprofile.RichProcessesDockerConfig {
	t.Helper()
	b := principalFixture()
	l := dockerInstallLayout{richInstallLayout: richInstallLayout{installLayout{digest: b.ArtifactSHA256, collectorUID: b.CollectorUID, uploaderUID: b.UploaderUID, uploaderGID: b.UploaderGID, sharedGID: b.SharedGID}}, readerUID: 62005, frameGID: 62006, socketGID: 998}
	c, _, err := dockerConfigForPrincipals(b.ServerID, b.ConnectorID, l, b.Addresses)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestDockerReaderActivationAuditsBothPausedBoundariesAndCleansFailures(t *testing.T) {
	for _, failure := range []string{"", "audit-before", "request.json", "missing-response", "stale-response", "audit-response", "commit.json", "stale-status", "changed-after-commit", "uncertain-start", "cancelled", "replaced"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := &readerFixture{t: t, fail: failure, cancel: cancel}
			c := dockerLifecycleConfig(t)
			_, err := startDockerReaderWith(ctx, c, f.operations())
			if failure == "" {
				startup, _ := connectedprofile.EncodeRichProcessesDockerReaderStartup(c)
				domain := sha256.Sum256(startup)
				plain, _ := connectedprofile.EncodeRichProcessesDockerConfig(c)
				plainHash := sha256.Sum256(plain)
				if f.request.ConfigSHA256 != hex.EncodeToString(domain[:]) || f.request.ConfigSHA256 == hex.EncodeToString(plainHash[:]) {
					t.Fatal("reader request lost role domain")
				}
				if err != nil || !f.committed || f.audits != 2 || slices.Contains(f.events, "stop") {
					t.Fatal("valid reader handshake refused")
				}
			}
			if failure != "" {
				if failure == "replaced" {
					if !errors.Is(err, ErrRecovery) || !f.active || slices.Contains(f.events, "stop") {
						t.Fatal("replacement invocation stopped or uncertainty hidden")
					}
					return
				}
				if err == nil || f.active || !slices.Contains(f.events, "stop") {
					t.Fatal("failed reader handshake not joined")
				}
				if strings.HasPrefix(failure, "audit-") && f.committed {
					t.Fatal("audit failure released Engine I/O")
				}
			}
		})
	}
}

func TestDockerReaderPreexistingActiveInvocationIsNotAdopted(t *testing.T) {
	f := &readerFixture{t: t, active: true}
	if _, err := startDockerReaderWith(context.Background(), dockerLifecycleConfig(t), f.operations()); !errors.Is(err, ErrUnsafe) || len(f.events) != 0 || !f.active {
		t.Fatal("preexisting reader adopted or stopped")
	}
}
func TestDockerStatusDoesNotHideReaderFailureOrInventHealthyEmpty(t *testing.T) {
	now := time.Now().UTC()
	active := workerInstance{PID: 42, Invocation: strings.Repeat("a", 32), Active: "active"}
	inactive := workerInstance{Active: "inactive"}
	collected := &connectedstatus.Record{InvocationID: active.Invocation, State: "COLLECTING"}
	uploaded := &connectedstatus.Record{InvocationID: active.Invocation, State: "PENDING"}
	for _, state := range []string{"COLLECTING", "SOURCE_UNAVAILABLE"} {
		r := DockerStatus{Collector: collected, Uploader: uploaded, Reader: &connectedstatus.Record{InvocationID: active.Invocation, State: state}}
		got := classifyDockerStatus(active, active, active, r, now)
		if got.State != "PENDING" || got.Reader.State != state {
			t.Fatal("reader quality/delivery semantics lost")
		}
	}
	if got := classifyDockerStatus(active, active, inactive, DockerStatus{Collector: collected, Uploader: uploaded}, now); got.State != "RECOVERY_REQUIRED" {
		t.Fatal("missing reader reported healthy")
	}
	if got := classifyDockerStatus(inactive, inactive, inactive, DockerStatus{}, now); got.State != "STOPPED" {
		t.Fatal("joined services not stopped")
	}
}
