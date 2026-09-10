//go:build windows && (amd64 || arm64)

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/buildidentity"
	"github.com/braidenm/home-lab-observer/internal/logobs"
	"github.com/braidenm/home-lab-observer/internal/logprotocol"
)

func windowsHelperFixture(t *testing.T) (string, logprotocol.Build, logobs.ReadRequest, []byte) {
	t.Helper()
	build := logprotocol.Build{Version: "0.1.0-preview.99", Commit: strings.Repeat("a", 40), OS: "windows", Arch: runtime.GOARCH}
	record, err := buildidentity.Encode(buildidentity.Identity{Role: "observer", Version: build.Version, Commit: build.Commit, OS: build.OS, Arch: build.Arch})
	if err != nil {
		t.Fatal(err)
	}
	request := logobs.ReadRequest{Source: logobs.SourceSystem, QueryStartedAt: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)}
	payload, err := logprotocol.EncodeRequest(build, request)
	if err != nil {
		t.Fatal(err)
	}
	return record, build, request, payload
}

type windowsHelperReaderFunc func(context.Context, logobs.ReadRequest) (logobs.Batch, error)

func (f windowsHelperReaderFunc) Read(ctx context.Context, request logobs.ReadRequest) (logobs.Batch, error) {
	return f(ctx, request)
}

type unreadableHelperInput struct{ t *testing.T }

func (input unreadableHelperInput) Read([]byte) (int, error) {
	input.t.Fatal("private input read for invalid helper invocation")
	return 0, io.EOF
}

type preparedHelperInput struct {
	t        *testing.T
	prepared *bool
	input    io.Reader
}

func (input preparedHelperInput) Read(buffer []byte) (int, error) {
	if !*input.prepared {
		input.t.Fatal("private input read before helper preparation")
	}
	return input.input.Read(buffer)
}

func TestWindowsHelperRejectsArgumentsAndIdentityBeforeInputOrNativeOpen(t *testing.T) {
	record, _, _, _ := windowsHelperFixture(t)
	for _, test := range []struct {
		name     string
		args     []string
		identity string
	}{
		{name: "arbitrary option", args: []string{"--channel", "Security"}, identity: record},
		{name: "arbitrary positional argument", args: []string{"System"}, identity: record},
		{name: "missing identity", identity: ""},
		{name: "invalid identity", identity: "private-token-canary"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			code := runWindowsLogHelper(test.args, test.identity, unreadableHelperInput{t}, &output, func() error {
				t.Fatal("invalid helper invocation reached hardening")
				return nil
			}, func() (logobs.Reader, func() error, error) {
				t.Fatal("invalid helper invocation opened native reader")
				return nil, nil, nil
			})
			if code != 1 || output.Len() != 0 {
				t.Fatalf("code=%d output=%q", code, output.String())
			}
		})
	}
}

func TestWindowsHelperInvalidPacketDoesNotOpenNativeReader(t *testing.T) {
	record, _, _, _ := windowsHelperFixture(t)
	opened := false
	var output bytes.Buffer
	code := runWindowsLogHelper(nil, record, bytes.NewReader([]byte(`{"private":"secret-canary"}`)), &output, func() error { return nil }, func() (logobs.Reader, func() error, error) {
		opened = true
		return nil, nil, nil
	})
	if code != 1 || opened || output.Len() != 0 {
		t.Fatalf("code=%d opened=%v output=%q", code, opened, output.String())
	}
}

func TestWindowsHelperEmitsOnlyClosedCorrelatedResponse(t *testing.T) {
	record, build, request, payload := windowsHelperFixture(t)
	prepared, opened, closed := false, false, false
	var output bytes.Buffer
	code := runWindowsLogHelper(nil, record, preparedHelperInput{t, &prepared, bytes.NewReader(payload)}, &output, func() error {
		prepared = true
		return nil
	}, func() (logobs.Reader, func() error, error) {
		if !prepared {
			t.Fatal("native reader opened before helper preparation")
		}
		opened = true
		reader := windowsHelperReaderFunc(func(_ context.Context, actual logobs.ReadRequest) (logobs.Batch, error) {
			if actual.Source != request.Source || !actual.QueryStartedAt.Equal(request.QueryStartedAt) {
				return logobs.Batch{}, errors.New("private-request-canary")
			}
			reason := logobs.ReasonNoVisibleJournal
			return logobs.Batch{
				Kind: logobs.BatchNormal, Source: actual.Source, ExpectedRevision: actual.Checkpoint.Revision,
				QueryStartedAt: actual.QueryStartedAt, StartedAt: actual.QueryStartedAt, FinishedAt: actual.QueryStartedAt,
				SupportState: logobs.SupportUnavailable, CollectionState: logobs.CollectionNotRun, ReasonCode: &reason,
			}, nil
		})
		return reader, func() error { closed = true; return nil }, nil
	})
	if code != 0 || !opened || !closed {
		t.Fatalf("code=%d opened=%v closed=%v", code, opened, closed)
	}
	batch, err := logprotocol.DecodeResponse(output.Bytes(), build, request)
	if err != nil || batch.ReasonCode == nil || *batch.ReasonCode != logobs.ReasonNoVisibleJournal {
		t.Fatalf("closed response rejected: batch=%+v err=%v", batch, err)
	}
	for _, canary := range []string{"private-request-canary", "secret-canary", "Security"} {
		if bytes.Contains(output.Bytes(), []byte(canary)) {
			t.Fatalf("private diagnostic leaked: %q", canary)
		}
	}
}

func TestWindowsPrivateDispatchIsExactAndSilent(t *testing.T) {
	previous := releaseIdentity
	t.Cleanup(func() { releaseIdentity = previous })
	record, _, _, _ := windowsHelperFixture(t)
	releaseIdentity = record
	if handled, code := dispatchPrivateLogHelper([]string{"version"}, nil, nil); handled || code != 0 {
		t.Fatal("ordinary command intercepted")
	}
	var output bytes.Buffer
	handled, code := dispatchPrivateLogHelper([]string{privateLogHelperCommand, "unexpected"}, unreadableHelperInput{t}, &output)
	if !handled || code != 1 || output.Len() != 0 {
		t.Fatalf("handled=%v code=%d output=%q", handled, code, output.String())
	}
}
