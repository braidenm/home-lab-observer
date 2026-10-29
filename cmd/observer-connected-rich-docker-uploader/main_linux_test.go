//go:build linux

package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
	"github.com/braidenm/home-lab-observer/internal/uploadloop"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

func TestRichUploaderLaunchIsFixedAndExcludesOtherRoles(t *testing.T) {
	id := connectedidentity.Identity{Role: "rich-docker-uploader", Version: "0.2.0-canary.1", Commit: strings.Repeat("a", 40), OS: "linux", Arch: "amd64", ContractSHA256: connectedprofile.RichProcessesDockerDefinitionSHA256}
	encoded, err := connectedidentity.Encode(id)
	if err != nil {
		t.Fatal(err)
	}
	env := []string{"GODEBUG=netdns=go", "INVOCATION_ID=" + strings.Repeat("b", 32)}
	for _, mode := range []string{"upload", "enroll", "validate-enrollment", "validate-ledger", "validate-existing-ledger"} {
		args := []string{"observer-connected-rich-docker-uploader"}
		if mode != "upload" {
			args = append(args, mode)
		}
		if got, err := validateLaunch(env, encoded, args); err != nil || got != mode {
			t.Fatal("fixed mode refused")
		}
	}
	for _, args := range [][]string{nil, {"uploader", "--profile=numeric"}, {"uploader", "repair"}, {"uploader", "enroll", "extra"}} {
		if _, err := validateLaunch(env, encoded, args); err == nil {
			t.Fatal("arbitrary mode/profile admitted")
		}
	}
	for _, role := range []string{"rich-docker-collector", "rich-docker-install"} {
		other := id
		other.Role = role
		record, err := connectedidentity.Encode(other)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := validateLaunch(env, record, []string{"uploader"}); err == nil {
			t.Fatal("wrong role admitted")
		}
	}
	if _, err := validateLaunch(append(env, "HTTPS_PROXY=https://proxy.example"), encoded, []string{"uploader"}); err == nil {
		t.Fatal("proxy override admitted")
	}
	if _, err := validateLaunch(env, "", []string{"uploader"}); err == nil {
		t.Fatal("unidentified artifact admitted")
	}
}

func TestRichUploaderDependencyClosureHasNoAcquisitionAuthority(t *testing.T) {
	allowed := map[string]bool{}
	for _, name := range strings.Fields("connectedcompat connectedcredential connectedprofile connectedactivation connectedruntime connectedstartup observation remoteprojection combinedprojection workloadprojection containerobs/model uploadstate enrollmentcoord ownerfs uploadledger ledgerwitness enrollmentstore platformtransport connectedenroll connectedidentity connectedstatus sharedhandoff uploadloop") {
		allowed[name] = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-f", "{{.ImportPath}}", ".")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	cmd.WaitDelay = time.Second
	data, err := cmd.Output()
	if err != nil {
		t.Fatal("dependency inspection failed")
	}
	for _, path := range strings.Fields(string(data)) {
		if name, ok := strings.CutPrefix(path, "github.com/braidenm/home-lab-observer/internal/"); ok && !allowed[name] {
			t.Fatalf("unexpected uploader authority: %s", name)
		}
		if strings.HasPrefix(path, "github.com/shirou/gopsutil/") {
			t.Fatal("uploader acquired host process inspection")
		}
	}
}

type statusCapture struct {
	record connectedstatus.Record
	fail   bool
}

func (s *statusCapture) Write(r connectedstatus.Record) error {
	s.record = r
	if s.fail {
		return connectedstatus.ErrUnsafe
	}
	return nil
}

type ackStep struct{}

func (ackStep) Step(context.Context) (uploadstate.Outcome, error) {
	return uploadstate.Acknowledged, nil
}

type stepFunc func(context.Context) (uploadstate.Outcome, error)

func (f stepFunc) Step(ctx context.Context) (uploadstate.Outcome, error) { return f(ctx) }

func TestGuardFailureCannotBeTranslatedIntoAnotherRetry(t *testing.T) {
	c, p, _ := testPolicy(t)
	valid := true
	g, err := newPolicyGuard(c, policyPorts{config: func() (connectedprofile.RichProcessesDockerConfig, error) { return c, nil }, policy: func() (connectedprofile.RichProcessesDockerPolicy, error) {
		if !valid {
			return p, connectedprofile.ErrUnsafe
		}
		return p, nil
	}, identity: func(connectedprofile.RichProcessesDockerConfig) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	sink := &statusCapture{}
	w := &observedStepper{machine: stepFunc(func(ctx context.Context) (uploadstate.Outcome, error) {
		valid = false
		g.Check(ctx)
		return uploadstate.Retry, nil
	}), transport: &observedTransport{}, guard: g, status: sink, now: time.Now, cancel: func() {}, record: connectedstatus.Record{Version: "observer-connected-status/v1"}}
	if outcome, err := w.Step(context.Background()); outcome != "" || err != uploadstate.ErrRecovery || sink.record.State != "RECOVERY_REQUIRED" {
		t.Fatal("policy refusal continued as retry")
	}
}

func TestLargeCombinedAckStatusUsesCollectionNotReceiptTime(t *testing.T) {
	c, p, _ := testPolicy(t)
	g, err := newPolicyGuard(c, policyPorts{config: func() (connectedprofile.RichProcessesDockerConfig, error) { return c, nil }, policy: func() (connectedprofile.RichProcessesDockerPolicy, error) { return p, nil }, identity: func(connectedprofile.RichProcessesDockerConfig) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	body := richFixture(t, c.ServerID, at)
	tr := &observedTransport{delegate: &fixtureTransport{}, guard: g}
	if _, err := tr.Send(context.Background(), uploadstate.Request{Binding: uploadstate.Binding{ServerID: c.ServerID, ConnectorID: c.ConnectorID}, Sequence: 1, Body: body}); err != nil || !tr.collectedAt.Equal(at) {
		t.Fatal("large combined body lost collection timestamp")
	}
	sink := &statusCapture{}
	w := &observedStepper{machine: ackStep{}, transport: tr, guard: g, status: sink, now: func() time.Time { return at.Add(time.Minute) }, cancel: func() {}, record: connectedstatus.Record{Version: "observer-connected-status/v1"}}
	if outcome, err := w.Step(context.Background()); outcome != uploadstate.Acknowledged || err != nil {
		t.Fatal("ack status failed")
	}
	if sink.record.State != "ACKNOWLEDGED_STALE" || !sink.record.CollectedAt.Equal(at) || sink.record.Attempts != 1 || sink.record.Acknowledgements != 1 {
		t.Fatal("receipt time presented as fresh source or polling counted as attempt")
	}
	if finishUpload(uploadloop.Canceled, sink, sink.record) != 0 || sink.record.State != "STOPPED" {
		t.Fatal("cancelled loop retained active status")
	}
	sink.fail = true
	if finishUpload(uploadloop.Canceled, sink, sink.record) != 22 {
		t.Fatal("status failure hidden")
	}
}
