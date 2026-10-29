//go:build linux

package main

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
	"github.com/braidenm/home-lab-observer/internal/uploadloop"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

type statusSink interface {
	Write(connectedstatus.Record) error
}
type stepper interface {
	Step(context.Context) (uploadstate.Outcome, error)
}

type observedTransport struct {
	delegate    uploadstate.Transport
	guard       *policyGuard
	attempts    uint64
	collectedAt time.Time
}

func (t *observedTransport) Send(ctx context.Context, r uploadstate.Request) (uploadstate.Response, error) {
	if err := t.guard.Check(ctx); err != nil {
		return uploadstate.Response{}, err
	}
	at, err := combinedprojection.CollectionTime(r.Body, r.Binding.ServerID)
	if err != nil {
		return uploadstate.Response{}, uploadstate.ErrSnapshot
	}
	if t.attempts < math.MaxUint64 {
		t.attempts++
	}
	t.collectedAt = at
	return t.delegate.Send(ctx, r)
}

type observedStepper struct {
	machine   stepper
	transport *observedTransport
	guard     *policyGuard
	status    statusSink
	record    connectedstatus.Record
	now       func() time.Time
	cancel    context.CancelFunc
	failed    bool
}

func (s *observedStepper) Step(ctx context.Context) (uploadstate.Outcome, error) {
	var outcome uploadstate.Outcome
	err := s.guard.Check(ctx)
	if err == nil {
		outcome, err = s.machine.Step(ctx)
	}
	if s.guard.failed {
		outcome, err = "", uploadstate.ErrRecovery
	}
	now := s.now().UTC()
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
			s.record.AcknowledgedAt = now
			s.record.State = freshness(s.record.CollectedAt, now)
			if s.record.Acknowledgements < math.MaxUint64 {
				s.record.Acknowledgements++
			}
		case uploadstate.Idle:
			s.record.State = "WAITING_FIRST_UPLOAD"
			if !s.record.AcknowledgedAt.IsZero() {
				s.record.State = freshness(s.record.CollectedAt, now)
			}
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
	return outcome, err
}

func freshness(collected, now time.Time) string {
	if !collected.IsZero() && now.Sub(collected) <= connectedstatus.FreshnessWindow && !collected.After(now.Add(uploadstate.MaxFuture)) {
		return "ACKNOWLEDGED_FRESH"
	}
	return "ACKNOWLEDGED_STALE"
}

func finishUpload(result uploadloop.Result, status statusSink, record connectedstatus.Record) int {
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
