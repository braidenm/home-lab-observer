//go:build linux && (amd64 || arm64)

package main

import (
	"bytes"
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

func helperIdentity(t *testing.T) logprotocol.Build {
	t.Helper()
	previous := releaseIdentity
	t.Cleanup(func() { releaseIdentity = previous })
	b := logprotocol.Build{Version: "0.1.0-preview.99", Commit: strings.Repeat("a", 40), OS: "linux", Arch: runtime.GOARCH}
	var err error
	releaseIdentity, err = buildidentity.Encode(buildidentity.Identity{Role: "journal-helper", Version: b.Version, Commit: b.Commit, OS: b.OS, Arch: b.Arch})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type guardedInput struct {
	hardened *bool
	t        *testing.T
	reader   io.Reader
}

func (r guardedInput) Read(p []byte) (int, error) {
	if !*r.hardened {
		r.t.Fatal("private input read before hardening")
	}
	return r.reader.Read(p)
}

func TestFixedEntrypointHardensBeforeInputAndNativeOpen(t *testing.T) {
	b := helperIdentity(t)
	request := logobs.ReadRequest{Source: logobs.SourceSystem, QueryStartedAt: time.Now().UTC()}
	payload, err := logprotocol.EncodeRequest(b, request)
	if err != nil {
		t.Fatal(err)
	}
	hardened, opened := false, false
	var output bytes.Buffer
	code := run(nil, guardedInput{&hardened, t, bytes.NewReader(payload)}, &output, func() error { hardened = true; return nil }, func() (logobs.Reader, func() error, error) {
		if !hardened {
			t.Fatal("native library opened before hardening")
		}
		opened = true
		return nil, nil, errors.New("synthetic private library failure")
	})
	if code != 0 || !opened || bytes.Contains(output.Bytes(), []byte("synthetic")) {
		t.Fatal("closed helper failure response lost")
	}
}

func TestFixedEntrypointRejectsOptionsAndInvalidIdentityWithoutReading(t *testing.T) {
	helperIdentity(t)
	for _, invalidIdentity := range []bool{false, true} {
		args := []string{"--path", "/synthetic/private"}
		if invalidIdentity {
			releaseIdentity = "invalid"
			args = nil
		}
		code := run(args, nil, nil, func() error { t.Fatal("invalid invocation hardened"); return nil }, func() (logobs.Reader, func() error, error) {
			t.Fatal("invalid invocation opened native source")
			return nil, nil, nil
		})
		if code != 1 {
			t.Fatal("invalid invocation accepted")
		}
	}
}
