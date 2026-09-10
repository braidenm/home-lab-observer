package logprocess

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
	"github.com/braidenm/home-lab-observer/internal/logprotocol"
)

type readerFunc func(context.Context, logobs.ReadRequest) (logobs.Batch, error)

func (f readerFunc) Read(c context.Context, q logobs.ReadRequest) (logobs.Batch, error) {
	return f(c, q)
}

type trackedInput struct {
	input io.Reader
	read  *bool
}

func (t trackedInput) Read(p []byte) (int, error) { *t.read = true; return t.input.Read(p) }
func serveFixture(t *testing.T) (logprotocol.Build, logobs.ReadRequest, []byte) {
	t.Helper()
	b := logprotocol.Build{Version: "0.1.0-preview.3", Commit: strings.Repeat("a", 40), OS: "linux", Arch: "amd64"}
	q := logobs.ReadRequest{Source: logobs.SourceSystem, QueryStartedAt: time.Now().UTC().Add(-time.Second)}
	p, err := logprotocol.EncodeRequest(b, q)
	if err != nil {
		t.Fatal(err)
	}
	return b, q, p
}
func TestChildHardensBeforePrivateInput(t *testing.T) {
	b, _, p := serveFixture(t)
	read, opened := false, false
	cfg := HelperConfig{Build: b, Harden: func() error {
		if read {
			t.Fatal("private input before hardening")
		}
		return errors.New("private-hardening-error")
	}, Open: func() (logobs.Reader, func() error, error) { opened = true; return nil, nil, nil }}
	var out bytes.Buffer
	if Serve(context.Background(), cfg, trackedInput{bytes.NewReader(p), &read}, &out) == 0 || read || opened || out.Len() != 0 {
		t.Fatal("failed hardening did not fail closed")
	}
}
func TestChildInvalidPacketNeverOpensNative(t *testing.T) {
	b, _, _ := serveFixture(t)
	for _, p := range [][]byte{[]byte("private-invalid-cursor"), bytes.Repeat([]byte(" "), logprotocol.MaxRequestBytes+1)} {
		opened := false
		cfg := HelperConfig{Build: b, Harden: func() error { return nil }, Open: func() (logobs.Reader, func() error, error) { opened = true; return nil, nil, nil }}
		var out bytes.Buffer
		if Serve(context.Background(), cfg, bytes.NewReader(p), &out) == 0 || opened || out.Len() != 0 {
			t.Fatal("invalid packet reached native or output")
		}
	}
}
func TestChildNormalizesOpenFailureAndClosesBeforeOutput(t *testing.T) {
	b, q, p := serveFixture(t)
	closed := false
	cfg := HelperConfig{Build: b, Harden: func() error { return nil }, Open: func() (logobs.Reader, func() error, error) {
		return nil, func() error { closed = true; return nil }, errors.New("private-loader-error")
	}}
	var out bytes.Buffer
	if Serve(context.Background(), cfg, bytes.NewReader(p), &out) != 0 || !closed {
		t.Fatal("unavailable packet failed")
	}
	batch, err := logprotocol.DecodeResponse(out.Bytes(), b, q)
	if err != nil || batch.ReasonCode == nil || *batch.ReasonCode != logobs.ReasonLogHelperUnavailable || batch.CaughtUp {
		t.Fatal("invented acquisition")
	}
	if bytes.Contains(out.Bytes(), []byte("private")) {
		t.Fatal("private error leaked")
	}
}
func TestChildRejectsCleanupCancellationAndWrongBatch(t *testing.T) {
	for _, mode := range []string{"cancel-close", "wrong-batch", "reader-failure", "close-failure"} {
		t.Run(mode, func(t *testing.T) {
			b, q, p := serveFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			closed := false
			cfg := HelperConfig{Build: b, Harden: func() error { return nil }, Open: func() (logobs.Reader, func() error, error) {
				return readerFunc(func(_ context.Context, _ logobs.ReadRequest) (logobs.Batch, error) {
						if mode == "reader-failure" {
							return logobs.Batch{}, errors.New("private-reader-error")
						}
						reason := logobs.ReasonLogHelperUnavailable
						batch := logobs.Batch{Kind: logobs.BatchNormal, Source: q.Source, QueryStartedAt: q.QueryStartedAt, StartedAt: q.QueryStartedAt, FinishedAt: q.QueryStartedAt, SupportState: logobs.SupportUnavailable, CollectionState: logobs.CollectionNotRun, ReasonCode: &reason}
						if mode == "wrong-batch" {
							batch.ExpectedRevision = 2
						}
						return batch, nil
					}), func() error {
						closed = true
						if mode == "cancel-close" {
							cancel()
						}
						if mode == "close-failure" {
							return errors.New("private-cleanup-error")
						}
						return nil
					}, nil
			}}
			var out bytes.Buffer
			if Serve(ctx, cfg, bytes.NewReader(p), &out) == 0 || !closed || out.Len() != 0 {
				t.Fatal("invalid or canceled acquisition emitted output")
			}
		})
	}
}
