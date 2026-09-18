//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedcompat"
	"github.com/braidenm/home-lab-observer/internal/connectedcredential"
	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
	"github.com/braidenm/home-lab-observer/internal/uploadloop"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

type sourceUnavailable struct{}

func (sourceUnavailable) Read(context.Context, string) ([]byte, error) {
	return nil, errors.New("synthetic unavailable")
}

type testLedger struct{ binding uploadstate.Binding }

func (l testLedger) Load(context.Context) (uploadstate.Record, error) {
	return uploadstate.Record{Binding: l.binding}, nil
}
func (testLedger) Commit(context.Context, uploadstate.Record, uploadstate.Record) error {
	return errors.New("unexpected commit")
}

type unusedTransport struct{ t *testing.T }

func (t unusedTransport) Send(context.Context, uploadstate.Request) (uploadstate.Response, error) {
	t.t.Fatal("unexpected request")
	return uploadstate.Response{}, nil
}

type unexpectedStep struct{ t *testing.T }

func (s unexpectedStep) Step(context.Context) (uploadstate.Outcome, error) {
	s.t.Fatal("canceled startup wait called Step")
	return "", nil
}

func TestUploaderLaunchAdmission(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("the first connected worker profile is Linux/amd64")
	}
	identity := connectedidentity.Identity{
		Role: "uploader", Version: "0.1.0-canary.1", Commit: strings.Repeat("a", 40),
		OS: "linux", Arch: "amd64", ContractSHA256: connectedcompat.Digest(),
	}
	record, err := connectedidentity.Encode(identity)
	if err != nil {
		t.Fatal(err)
	}
	environ := []string{"GODEBUG=netdns=go", "INVOCATION_ID=synthetic"}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"observer-connected-uploader"}, "upload"},
		{[]string{"observer-connected-uploader", "enroll"}, "enroll"},
		{[]string{"observer-connected-uploader", "validate-enrollment"}, "validate-enrollment"},
		{[]string{"observer-connected-uploader", "validate-ledger"}, "validate-ledger"},
		{[]string{"observer-connected-uploader", "validate-existing-ledger"}, "validate-existing-ledger"},
	} {
		if got, err := validateLaunch(environ, record, tc.args); err != nil || got != tc.want {
			t.Fatalf("fixed mode %q refused: %v", tc.want, err)
		}
	}
	wrongRole := identity
	wrongRole.Role = "collector"
	wrongRecord, err := connectedidentity.Encode(wrongRole)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		environ  []string
		identity string
		args     []string
	}{
		{"missing build identity", environ, "", []string{"uploader"}},
		{"wrong role", environ, wrongRecord, []string{"uploader"}},
		{"unknown mode", environ, record, []string{"uploader", "repair"}},
		{"extra argument", environ, record, []string{"uploader", "enroll", "extra"}},
		{"missing DNS policy", []string{"INVOCATION_ID=synthetic"}, record, []string{"uploader"}},
		{"proxy override", append(append([]string{}, environ...), "HTTPS_PROXY=https://proxy.example"), record, []string{"uploader"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateLaunch(tc.environ, tc.identity, tc.args); err == nil {
				t.Fatal("unsafe launch admitted")
			}
		})
	}
}

func TestCredentialSourceNeverCrossesBinding(t *testing.T) {
	binding := uploadstate.Binding{ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32)}
	source := credentialSource{connectedcredential.CredentialRecord{ServerID: binding.ServerID, ConnectorID: binding.ConnectorID, Secret: "synthetic-secret"}}
	if got, err := source.Credential(context.Background(), binding); err != nil || got != "synthetic-secret" {
		t.Fatal("matching binding refused")
	}
	for _, altered := range []uploadstate.Binding{
		{ServerID: "srv_" + strings.Repeat("c", 32), ConnectorID: binding.ConnectorID},
		{ServerID: binding.ServerID, ConnectorID: "agent_" + strings.Repeat("d", 32)},
	} {
		if got, err := source.Credential(context.Background(), altered); err == nil || got != "" {
			t.Fatal("cross-bound credential exposed")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := source.Credential(ctx, binding); err == nil || got != "" {
		t.Fatal("canceled credential lookup exposed secret")
	}
	if got, err := source.Credential(nil, binding); err == nil || got != "" {
		t.Fatal("nil-context credential lookup exposed secret")
	}
}

func TestCollectionFreshnessDoesNotUseAckReceipt(t *testing.T) {
	now := time.Now().UTC()
	if freshness(now.Add(-uploadstate.MaxAge-time.Nanosecond), now) != "ACKNOWLEDGED_STALE" || freshness(time.Time{}, now) != "ACKNOWLEDGED_STALE" || freshness(now.Add(uploadstate.MaxFuture+time.Nanosecond), now) != "ACKNOWLEDGED_STALE" {
		t.Fatal("stale source presented fresh")
	}
	if freshness(now.Add(-time.Second), now) != "ACKNOWLEDGED_FRESH" {
		t.Fatal("fresh source lost")
	}
	if freshness(now.Add(-connectedstatus.FreshnessWindow), now) != "ACKNOWLEDGED_FRESH" || freshness(now.Add(-connectedstatus.FreshnessWindow-time.Nanosecond), now) != "ACKNOWLEDGED_STALE" || freshness(now.Add(-2*time.Minute), now) != "ACKNOWLEDGED_STALE" {
		t.Fatal("admission age confused with hosted freshness")
	}
}

func TestIdleDoesNotInventPreviousAcknowledgement(t *testing.T) {
	now := time.Now().UTC()
	if idleState(connectedstatus.Record{}, now) != "WAITING_FIRST_UPLOAD" {
		t.Fatal("initial idle invented acknowledgement")
	}
	if idleState(connectedstatus.Record{AcknowledgedAt: now, CollectedAt: now.Add(-time.Second)}, now) != "ACKNOWLEDGED_FRESH" {
		t.Fatal("known acknowledgement lost")
	}
	if idleState(connectedstatus.Record{AcknowledgedAt: now, CollectedAt: now.Add(-time.Minute)}, now) != "ACKNOWLEDGED_STALE" {
		t.Fatal("known stale capture became fresh")
	}
}

func TestPollingIsNotCountedAsTransportAttempt(t *testing.T) {
	b := uploadstate.Binding{ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32)}
	transport := &observedTransport{delegate: unusedTransport{t}}
	machine, err := uploadstate.New(b, sourceUnavailable{}, realClock{}, transport, testLedger{b})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("chmod")
	}
	status, err := connectedstatus.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer status.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := observedStepper{machine: machine, transport: transport, status: status, cancel: cancel, record: connectedstatus.Record{Version: "observer-connected-status/v1"}}
	if outcome, err := w.Step(ctx); err != nil || outcome != uploadstate.SourceUnavailable {
		t.Fatal("poll result")
	}
	if w.record.Steps != 1 || w.record.Attempts != 0 || w.record.Acknowledgements != 0 || w.record.State != "SOURCE_UNAVAILABLE" {
		t.Fatal("poll incorrectly reported network or success")
	}
}

func TestCanceledStartupWaitPersistsStoppedStatus(t *testing.T) {
	loop, err := uploadloop.New(unexpectedStep{t})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := loop.Run(ctx)
	if result != uploadloop.Canceled || !errors.Is(err, uploadloop.ErrCanceled) {
		t.Fatal("canceled startup wait was not joined")
	}
	dir := t.TempDir()
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("chmod")
	}
	status, err := connectedstatus.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	record := connectedstatus.Record{Version: "observer-connected-status/v1", State: "ACKNOWLEDGED_FRESH", UpdatedAt: now, CollectedAt: now, AcknowledgedAt: now, Acknowledgements: 1}
	if status.Write(record) != nil || finishUpload(result, status, record) != 0 {
		t.Fatal("cancellation did not publish stopped status")
	}
	data, err := os.ReadFile(filepath.Join(dir, "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := connectedstatus.Decode(data)
	if err != nil || got.State != "STOPPED" || got.Acknowledgements != 1 || !got.CollectedAt.Equal(now) || got.UpdatedAt.Before(now) {
		t.Fatal("cancellation retained misleading live status")
	}
	if status.Close() != nil || finishUpload(result, status, record) != 22 {
		t.Fatal("status write failure was hidden")
	}
}
