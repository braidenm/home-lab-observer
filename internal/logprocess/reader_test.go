package logprocess

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
	"github.com/braidenm/home-lab-observer/internal/logprotocol"
)

func TestReaderConsumesOnlyCorrelatedPrivatePackets(t *testing.T) {
	b, q, _ := serveFixture(t)
	for _, mode := range []string{"valid", "wrong-identity", "bad-packet", "process-failure", "missing", "mismatch"} {
		t.Run(mode, func(t *testing.T) {
			r := &Reader{build: b, now: time.Now, transport: &transport{slot: make(chan struct{}, 1), run: func(_ context.Context, payload []byte) ([]byte, error) {
				request, err := logprotocol.DecodeRequest(payload, b)
				if err != nil {
					t.Fatal("request codec mismatch")
				}
				if mode == "missing" {
					return nil, ErrHelperUnavailable
				}
				if mode == "mismatch" {
					return nil, ErrHelperMismatch
				}
				if mode == "process-failure" {
					return nil, errors.New("private-native-error")
				}
				if mode == "bad-packet" {
					return []byte("private-invalid-packet"), nil
				}
				reason := logobs.ReasonNoVisibleJournal
				batch := logobs.Batch{Kind: logobs.BatchNormal, Source: request.Source, QueryStartedAt: request.QueryStartedAt,
					StartedAt: q.QueryStartedAt, FinishedAt: q.QueryStartedAt, SupportState: logobs.SupportUnavailable, CollectionState: logobs.CollectionNotRun, ReasonCode: &reason}
				identity := b
				if mode == "wrong-identity" {
					identity.Version = "0.1.0-preview.4"
				}
				return logprotocol.EncodeResponse(identity, request, batch)
			}}}
			batch, err := r.Read(context.Background(), q)
			if mode == "bad-packet" || mode == "process-failure" {
				if err != errProcessFailed || batch.ReasonCode != nil {
					t.Fatal("invalid output became progress")
				}
				return
			}
			if err != nil || batch.Validate() != nil || batch.CaughtUp || len(batch.NextOpaque) != 0 {
				t.Fatal("invalid batch")
			}
			want := logobs.ReasonNoVisibleJournal
			if mode == "wrong-identity" || mode == "mismatch" {
				want = logobs.ReasonLogHelperMismatch
			}
			if mode == "missing" {
				want = logobs.ReasonLogHelperUnavailable
			}
			if batch.ReasonCode == nil || *batch.ReasonCode != want {
				t.Fatal("wrong fixed reason")
			}
		})
	}
}

func TestUnsupportedAndInvalidReaderNeverLaunch(t *testing.T) {
	b, q, _ := serveFixture(t)
	launched := false
	r := &Reader{build: b, now: time.Now, transport: newTransport(func() (commandSpec, error) { launched = true; return commandSpec{}, nil })}
	r.build.OS = "darwin"
	batch, err := r.Read(context.Background(), q)
	if err != nil || batch.SupportState != logobs.SupportUnsupported || launched {
		t.Fatal("unsupported platform launched helper")
	}
	r.build.OS = "linux"
	q.Source = logobs.SourceApplication
	if _, err := r.Read(context.Background(), q); err != errInputInvalid || launched {
		t.Fatal("Linux accepted application source")
	}
	q.Source = logobs.SourceSystem
	r.build.Version = "dev"
	batch, err = r.Read(context.Background(), q)
	if err != nil || *batch.ReasonCode != logobs.ReasonLogHelperMismatch || launched {
		t.Fatal("unversioned identity launched")
	}
}

func TestOnlyCanonicalResolverCodesEscape(t *testing.T) {
	for _, known := range []error{ErrHelperUnavailable, ErrHelperMismatch} {
		tr := newTransport(func() (commandSpec, error) { return commandSpec{}, fmt.Errorf("private-path: %w", known) })
		out, err := tr.exchange(context.Background(), nil)
		if err != known || out != nil {
			t.Fatal("wrapped resolver details escaped")
		}
	}
}
