//go:build paired_platform_acceptance

package actionremote

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

type pairedPlatformCredential struct{ input pairedPlatformInput }

func (p pairedPlatformCredential) Credential(ctx context.Context, binding Binding) (Credential, error) {
	if ctx.Err() != nil || binding != p.input.binding {
		return Credential{}, ErrCredential
	}
	return p.input.credential, nil
}

// This manually compiled probe consumes only a private synthetic fixture. It
// exercises the real typed client and real application replies, not an Engine.
func TestPairedPlatformClientAcceptance(t *testing.T) {
	in, err := readPairedPlatformInput(os.Stdin)
	if err != nil || runPairedPlatformClient(in) != nil {
		t.Fatal("paired_platform_client_acceptance_refused")
	}
	// Emitted only after the client, forwarding handlers and transports close.
	fmt.Println("HLO_PAIRED_CLIENT_ACCEPTANCE_PASS")
}

func runPairedPlatformClient(in pairedPlatformInput) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	target := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(in.port))}
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	transport := &http.Transport{Proxy: nil, Protocols: protocols, DisableKeepAlives: true, DisableCompression: true,
		DialContext: (&net.Dialer{Timeout: time.Second}).DialContext, ResponseHeaderTimeout: 4 * time.Second,
		MaxConnsPerHost: 1, MaxResponseHeaderBytes: 16384}
	defer transport.CloseIdleConnections()
	proxy := &httputil.ReverseProxy{
		Rewrite:   func(r *httputil.ProxyRequest) { r.SetURL(target); r.Out.Host = target.Host },
		Transport: transport, ErrorLog: log.New(io.Discard, "", 0),
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) { w.WriteHeader(http.StatusBadGateway) },
		ModifyResponse: func(r *http.Response) error {
			raw, err := io.ReadAll(io.LimitReader(r.Body, 16385))
			closeErr := r.Body.Close()
			if err != nil || closeErr != nil || len(raw) > 16384 {
				return errPairedPlatformFixture
			}
			r.Body = io.NopCloser(bytes.NewReader(raw))
			return nil
		},
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !pairedPlatformAllowed(r, in) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Body != nil {
			raw, err := io.ReadAll(io.LimitReader(r.Body, 16385))
			closeErr := r.Body.Close()
			if err != nil || closeErr != nil || int64(len(raw)) != r.ContentLength || len(raw) > 16384 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
		forwarded, stop := context.WithTimeout(r.Context(), 5*time.Second)
		defer stop()
		proxy.ServeHTTP(w, r.WithContext(forwarded))
	}))
	server.Config.ReadHeaderTimeout = time.Second
	server.Config.ReadTimeout = 5 * time.Second
	server.Config.WriteTimeout = 6 * time.Second
	server.Config.MaxHeaderBytes = 16384
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.Config.SetKeepAlivesEnabled(false)
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	client, err := newClient(server.URL, in.binding, pairedPlatformCredential{in}, roots)
	if err != nil {
		return errPairedPlatformFixture
	}
	defer client.Close()
	nonce := func() (string, error) {
		var raw [16]byte
		if _, err := rand.Read(raw[:]); err != nil || raw == [16]byte{} {
			return "", errPairedPlatformFixture
		}
		return hex.EncodeToString(raw[:]), nil
	}
	claimNonce, err := nonce()
	if err != nil {
		return err
	}
	claim, err := client.Claim(ctx, claimNonce, []string{in.target})
	if err != nil || claim == nil || claim.Request.ActionID != in.action || claim.Request.TargetID != in.target ||
		string(claim.Request.Operation) != in.operation {
		return errPairedPlatformFixture
	}
	for _, checkpoint := range []string{"1", "2"} {
		challenge, err := nonce()
		if err != nil {
			return err
		}
		decision, err := client.Check(ctx, actionprotocol.CheckRequest{Request: claim.Request,
			ExecutionLeaseID: claim.ExecutionLeaseID, Checkpoint: checkpoint, CheckNonce: challenge})
		if err != nil || actionprotocol.MatchExecutionDecision(*claim, checkpoint, challenge, decision) != nil {
			return errPairedPlatformFixture
		}
	}
	code := actioncore.Restarted
	switch claim.Request.Operation {
	case actioncore.Start:
		code = actioncore.Started
	case actioncore.Stop:
		code = actioncore.Stopped
	}
	report := actionprotocol.Report{Request: claim.Request, ExecutionLeaseID: claim.ExecutionLeaseID,
		ResultCode: code, EvidenceKind: "RETAINED_TERMINAL", CorrelationID: claim.CorrelationID}
	receipt, err := client.Report(ctx, in.action, report)
	if err != nil || actionprotocol.MatchResultReceipt(*claim, receipt) != nil {
		return errPairedPlatformFixture
	}
	equal, err := client.Report(ctx, in.action, report)
	if err != nil || equal != receipt {
		return errPairedPlatformFixture
	}
	retained, err := client.Retained(ctx, in.action)
	if err != nil || retained.State != "SUCCEEDED" || retained.LaneState != "RELEASED" ||
		retained.Request == nil || *retained.Request != claim.Request || retained.Claim == nil || *retained.Claim != *claim ||
		retained.Result == nil || *retained.Result != receipt {
		return errPairedPlatformFixture
	}
	return nil
}
