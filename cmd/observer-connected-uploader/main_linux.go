//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedcredential"
	"github.com/braidenm/home-lab-observer/internal/connectedenroll"
	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedstartup"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
	"github.com/braidenm/home-lab-observer/internal/platformtransport"
	"github.com/braidenm/home-lab-observer/internal/sharedhandoff"
	"github.com/braidenm/home-lab-observer/internal/uploadledger"
	"github.com/braidenm/home-lab-observer/internal/uploadloop"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

var releaseIdentity string

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type credentialSource struct {
	record connectedcredential.CredentialRecord
}

func (s credentialSource) Credential(ctx context.Context, b uploadstate.Binding) (string, error) {
	if ctx == nil || ctx.Err() != nil || s.record.ServerID != b.ServerID || s.record.ConnectorID != b.ConnectorID {
		return "", connectedcredential.ErrUnsafe
	}
	return s.record.Secret, nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	code := run(ctx)
	cancel()
	os.Exit(code)
}

// validateLaunch grants only the installed upload path or one fixed offline
// enrollment mode. It does not read a credential or perform an exchange.
func validateLaunch(environ []string, identity string, args []string) (string, error) {
	if connectedprofile.CheckEnvironment(environ) != nil {
		return "", connectedprofile.ErrUnsafe
	}
	if _, err := connectedidentity.Resolve(identity, "uploader"); err != nil {
		return "", err
	}
	if len(args) == 1 {
		return "upload", nil
	}
	if len(args) == 2 {
		switch args[1] {
		case "enroll", "validate-enrollment", "validate-ledger", "validate-existing-ledger":
			return args[1], nil
		}
	}
	return "", connectedprofile.ErrUnsafe
}

func run(ctx context.Context) (code int) {
	if connectedprofile.HardenSecretProcess() != nil {
		return 22
	}
	mode, err := validateLaunch(os.Environ(), releaseIdentity, os.Args)
	if err != nil {
		return 22
	}
	if mode != "upload" {
		return connectedenroll.Run(ctx, mode, os.Stdin, os.Stdout)
	}
	c, err := connectedprofile.Load()
	if err != nil || connectedprofile.CheckIdentity(c, false) != nil {
		return 22
	}
	status, err := connectedstatus.Open("/state/status")
	if err != nil {
		return 22
	}
	defer func() {
		if status.Close() != nil {
			code = 22
		}
	}()
	if connectedstartup.Run(ctx, c, status) != nil {
		return 22
	}
	secret, err := connectedcredential.Load(c.ServerID, c.ConnectorID)
	if err != nil {
		return 22
	}
	binding := uploadstate.Binding{ServerID: c.ServerID, ConnectorID: c.ConnectorID}
	ledger, err := uploadledger.OpenExisting(ctx, "/state/ledger", binding)
	if err != nil {
		return 22
	}
	defer func() {
		if ledger.Close() != nil {
			code = 22
		}
	}()
	reader, err := sharedhandoff.OpenReader("/handoff", sharedhandoff.Policy{CollectorUID: c.CollectorUID, UploaderUID: c.UploaderUID, SharedGID: c.SharedGID, ServerID: c.ServerID})
	if err != nil {
		return 22
	}
	defer func() {
		if reader.Close() != nil {
			code = 22
		}
	}()
	transport, err := platformtransport.New(connectedprofile.Origin, credentialSource{secret})
	if err != nil {
		return 22
	}
	defer transport.CloseIdleConnections()
	observedTransport := &observedTransport{delegate: transport}
	machine, err := uploadstate.New(binding, reader, realClock{}, observedTransport, ledger)
	if err != nil {
		return 22
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	wrapper := &observedStepper{machine: machine, transport: observedTransport, status: status, cancel: cancel, record: connectedstatus.Record{Version: "observer-connected-status/v1", InvocationID: os.Getenv("INVOCATION_ID"), State: "WAITING_FIRST_UPLOAD", UpdatedAt: time.Now().UTC()}}
	if status.Write(wrapper.record) != nil {
		return 22
	}
	loop, err := uploadloop.New(wrapper)
	if err != nil {
		return 22
	}
	result, _ := loop.Run(child)
	if wrapper.failed {
		return 22
	}
	return finishUpload(result, status, wrapper.record)
}

// A canceled pacing wait does not call Step, so persist STOPPED here as well.
// Keep the last acknowledged timestamps for diagnostics, not liveness authority.
func finishUpload(result uploadloop.Result, status *connectedstatus.Writer, record connectedstatus.Record) int {
	switch result {
	case uploadloop.Canceled:
		record.State = "STOPPED"
		record.UpdatedAt = time.Now().UTC()
		if status.Write(record) != nil {
			return 22
		}
		return 0
	case uploadloop.CredentialRejected:
		return 20
	case uploadloop.Conflict, uploadloop.Rejected, uploadloop.Exhausted:
		return 21
	default:
		return 22
	}
}

type observedStepper struct {
	machine   *uploadstate.Machine
	transport *observedTransport
	status    *connectedstatus.Writer
	cancel    context.CancelFunc
	record    connectedstatus.Record
	failed    bool
}

func (s *observedStepper) Step(ctx context.Context) (uploadstate.Outcome, error) {
	outcome, err := s.machine.Step(ctx)
	now := time.Now().UTC()
	s.record.UpdatedAt = now
	if s.record.Steps < math.MaxUint64 {
		s.record.Steps++
	}
	s.record.Attempts = s.transport.attempts
	s.record.State = "RECOVERY_REQUIRED"
	if err == nil {
		switch outcome {
		case uploadstate.Acknowledged:
			s.record.CollectedAt = s.transport.collectedAt
			s.record.State = freshness(s.record.CollectedAt, now)
			s.record.AcknowledgedAt = now
			if s.record.Acknowledgements < math.MaxUint64 {
				s.record.Acknowledgements++
			}
		case uploadstate.Idle:
			s.record.State = idleState(s.record, now)
		case uploadstate.Retry:
			s.record.State = "RETRYING"
		case uploadstate.RateLimited:
			s.record.State = "RATE_LIMITED"
		case uploadstate.SourceUnavailable, uploadstate.ClockSkew, uploadstate.Expired:
			s.record.State = "SOURCE_UNAVAILABLE"
		case uploadstate.CredentialRejected:
			s.record.State = "CREDENTIAL_REJECTED"
		}
	} else if errors.Is(err, uploadstate.ErrSnapshot) {
		s.record.State = "INVALID_SNAPSHOT"
	} else if errors.Is(err, uploadstate.ErrCanceled) && ctx.Err() != nil {
		s.record.State = "STOPPED"
	}
	if s.status.Write(s.record) != nil {
		s.failed = true
		s.cancel()
	}
	// Preserve a durable C1 ACK even if status persistence failed; loop cancellation
	// joins normally, while run reports the separate operational failure.
	return outcome, err
}

func freshness(collected, now time.Time) string {
	if !collected.IsZero() && now.Sub(collected) <= connectedstatus.FreshnessWindow && !collected.After(now.Add(uploadstate.MaxFuture)) {
		return "ACKNOWLEDGED_FRESH"
	}
	return "ACKNOWLEDGED_STALE"
}

func idleState(record connectedstatus.Record, now time.Time) string {
	if record.AcknowledgedAt.IsZero() {
		return "WAITING_FIRST_UPLOAD"
	}
	return freshness(record.CollectedAt, now)
}

// This wrapper records logical transport attempts and the collection timestamp
// from C1's already validated request, without retaining payload/identity data.
type observedTransport struct {
	delegate    uploadstate.Transport
	attempts    uint64
	collectedAt time.Time
}

func (t *observedTransport) Send(ctx context.Context, r uploadstate.Request) (uploadstate.Response, error) {
	if t.attempts < math.MaxUint64 {
		t.attempts++
	}
	var stamp struct {
		CollectedAt time.Time `json:"collected_at"`
	}
	if len(r.Body) <= 16*1024 && json.Unmarshal(r.Body, &stamp) == nil {
		t.collectedAt = stamp.CollectedAt
	} else {
		t.collectedAt = time.Time{}
	}
	return t.delegate.Send(ctx, r)
}
