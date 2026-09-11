package platformtransport

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

var transportBinding = uploadstate.Binding{
	ServerID:    "srv_0123456789abcdef0123456789abcdef",
	ConnectorID: "agent_0123456789abcdef0123456789abcdef",
}

const transportCredential = "hlc_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq"

type fakeCredentials struct {
	credential string
	err        error
	calls      int
	binding    uploadstate.Binding
	deadline   time.Time
	hook       func()
}

func (p *fakeCredentials) Credential(ctx context.Context, binding uploadstate.Binding) (string, error) {
	p.calls++
	p.binding = binding
	p.deadline, _ = ctx.Deadline()
	if p.hook != nil {
		p.hook()
	}
	return p.credential, p.err
}

func validTransportBody(t *testing.T, serverID ...string) []byte {
	t.Helper()
	source := transportBinding.ServerID
	if len(serverID) == 1 {
		source = serverID[0]
	}
	body, err := remoteprojection.Encode(
		observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)},
		remoteprojection.Identity{SourceID: source, Version: "0.1.0-preview.3", OS: "linux"},
	)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func ownedRoots(server *httptest.Server) *x509.CertPool {
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	return roots
}

func newOwnedTransport(t *testing.T, server *httptest.Server, provider *fakeCredentials) *Transport {
	t.Helper()
	transport, err := newTransport(server.URL, provider, ownedRoots(server))
	if err != nil {
		t.Fatal(err)
	}
	return transport
}

func TestSendUsesExactBoundedRequestAndCorrelatedAcknowledgement(t *testing.T) {
	body := validTransportBody(t)
	provider := &fakeCredentials{credential: transportCredential}
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.URL.Path != "/v1/connectors/home-lab/servers/"+transportBinding.ServerID+"/snapshot" || request.URL.RawQuery != "" {
			t.Error("unexpected request target")
		}
		if request.Header.Get("Accept") != "application/json" || request.Header.Get("Content-Type") != "application/json" ||
			request.Header.Get("Authorization") != "Connector "+transportCredential || request.Header.Get("X-Connector-Sequence") != "7" {
			t.Error("unexpected request headers")
		}
		if request.Header.Get("Cookie") != "" || request.Header.Get("Referer") != "" || request.UserAgent() != "" {
			t.Error("ambient request headers present")
		}
		got, err := io.ReadAll(request.Body)
		if err != nil || !bytes.Equal(got, body) || request.ContentLength != int64(len(body)) {
			t.Error("request body changed")
		}
		response.Header().Set("Content-Type", "application/json; charset=UTF-8")
		fmt.Fprintf(response, `{"server_id":%q,"sequence":7,"future":{"secret-canary":"discard-me"}}`, transportBinding.ServerID)
	}))
	defer server.Close()

	transport := newOwnedTransport(t, server, provider)
	started := time.Now()
	result, err := transport.Send(context.Background(), uploadstate.Request{Binding: transportBinding, Sequence: 7, Body: body})
	if err != nil || result != (uploadstate.Response{Outcome: uploadstate.Acknowledged, ServerID: transportBinding.ServerID, Sequence: 7}) {
		t.Fatalf("unexpected result %#v error %v", result, err)
	}
	if provider.calls != 1 || provider.binding != transportBinding || provider.deadline.IsZero() || provider.deadline.Sub(started) > SendTimeout+100*time.Millisecond {
		t.Fatal("credential provider did not receive bounded expected binding")
	}
}

func TestSendRejectsInvalidRequestBeforeCredentialOrNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	provider := &fakeCredentials{credential: transportCredential}
	transport := newOwnedTransport(t, server, provider)
	validBody := validTransportBody(t)
	otherServer := "srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	cases := []uploadstate.Request{
		{},
		{Binding: uploadstate.Binding{ServerID: "bad", ConnectorID: transportBinding.ConnectorID}, Sequence: 1, Body: validBody},
		{Binding: uploadstate.Binding{ServerID: transportBinding.ServerID, ConnectorID: "bad"}, Sequence: 1, Body: validBody},
		{Binding: transportBinding, Sequence: 0, Body: validBody},
		{Binding: transportBinding, Sequence: 1},
		{Binding: transportBinding, Sequence: 1, Body: []byte("private-canary")},
		{Binding: transportBinding, Sequence: 1, Body: validTransportBody(t, otherServer)},
		{Binding: transportBinding, Sequence: 1, Body: bytes.Repeat([]byte{'x'}, remoteprojection.MaxBytes+1)},
	}
	for index, request := range cases {
		if _, err := transport.Send(context.Background(), request); err != ErrRequest {
			t.Fatalf("case %d returned %v", index, err)
		}
	}
	if _, err := transport.Send(nil, uploadstate.Request{Binding: transportBinding, Sequence: 1, Body: validBody}); err != ErrRequest {
		t.Fatalf("nil context returned %v", err)
	}
	if provider.calls != 0 || requests.Load() != 0 {
		t.Fatal("invalid request reached credential or network boundary")
	}
}

func TestSendDetachesValidatedBodyBeforeCredentialLookup(t *testing.T) {
	body := validTransportBody(t)
	expected := bytes.Clone(body)
	provider := &fakeCredentials{credential: transportCredential}
	provider.hook = func() {
		for index := range body {
			body[index] = 'x'
		}
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		got, err := io.ReadAll(request.Body)
		if err != nil || !bytes.Equal(got, expected) {
			t.Error("caller mutation changed admitted request bytes")
		}
		response.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(response, `{"server_id":%q,"sequence":1}`, transportBinding.ServerID)
	}))
	defer server.Close()
	transport := newOwnedTransport(t, server, provider)
	result, err := transport.Send(context.Background(), uploadstate.Request{Binding: transportBinding, Sequence: 1, Body: body})
	if err != nil || result.Outcome != uploadstate.Acknowledged {
		t.Fatalf("got %#v error %v", result, err)
	}
}

func TestStatusMapping(t *testing.T) {
	cases := []struct {
		status int
		want   uploadstate.Outcome
	}{
		{http.StatusBadRequest, uploadstate.Rejected},
		{http.StatusUnauthorized, uploadstate.CredentialRejected},
		{http.StatusConflict, uploadstate.Conflict},
		{http.StatusRequestEntityTooLarge, uploadstate.Rejected},
		{http.StatusTooManyRequests, uploadstate.RateLimited},
		{http.StatusRequestTimeout, uploadstate.Retry},
		{http.StatusNoContent, uploadstate.Retry},
		{http.StatusTeapot, uploadstate.Retry},
		{http.StatusInternalServerError, uploadstate.Retry},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("status-%d", tc.status), func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(tc.status)
				_, _ = response.Write(bytes.Repeat([]byte{'x'}, MaxResponseBytes+1))
			}))
			defer server.Close()
			transport := newOwnedTransport(t, server, &fakeCredentials{credential: transportCredential})
			result, err := transport.Send(context.Background(), uploadstate.Request{Binding: transportBinding, Sequence: 1, Body: validTransportBody(t)})
			if err != nil || result.Outcome != tc.want || result.ServerID != "" || result.Sequence != 0 {
				t.Fatalf("got %#v error %v", result, err)
			}
		})
	}
}

func TestAcknowledgementDecoderRejectsHostileKnownFields(t *testing.T) {
	serverID := transportBinding.ServerID
	valid := fmt.Sprintf(`{"server_id":%q,"sequence":3}`, serverID)
	cases := []struct {
		name        string
		body        string
		contentType []string
		encoding    string
	}{
		{"missing-content-type", valid, nil, ""},
		{"wrong-content-type", valid, []string{"text/plain"}, ""},
		{"extra-content-type", valid, []string{"application/json", "application/json"}, ""},
		{"wrong-charset", valid, []string{"application/json; charset=iso-8859-1"}, ""},
		{"unknown-content-type-parameter", valid, []string{"application/json; boundary=something"}, ""},
		{"charset-and-unknown-parameter", valid, []string{"application/json; charset=utf-8; boundary=something"}, ""},
		{"encoded", valid, []string{"application/json"}, "gzip"},
		{"array", `[]`, []string{"application/json"}, ""},
		{"missing-server", `{"sequence":3}`, []string{"application/json"}, ""},
		{"missing-sequence", fmt.Sprintf(`{"server_id":%q}`, serverID), []string{"application/json"}, ""},
		{"wrong-server-type", `{"server_id":7,"sequence":3}`, []string{"application/json"}, ""},
		{"wrong-sequence-type", fmt.Sprintf(`{"server_id":%q,"sequence":"3"}`, serverID), []string{"application/json"}, ""},
		{"fractional-sequence", fmt.Sprintf(`{"server_id":%q,"sequence":3.0}`, serverID), []string{"application/json"}, ""},
		{"zero-sequence", fmt.Sprintf(`{"server_id":%q,"sequence":0}`, serverID), []string{"application/json"}, ""},
		{"duplicate-known", fmt.Sprintf(`{"server_id":%q,"server_id":%q,"sequence":3}`, serverID, serverID), []string{"application/json"}, ""},
		{"escaped-duplicate-known", fmt.Sprintf(`{"server_id":%q,"\u0073erver_id":%q,"sequence":3}`, serverID, serverID), []string{"application/json"}, ""},
		{"trailing", valid + `{}`, []string{"application/json"}, ""},
		{"bom", "\ufeff" + valid, []string{"application/json"}, ""},
		{"wrong-correlation-server", `{"server_id":"srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sequence":3}`, []string{"application/json"}, ""},
		{"wrong-correlation-sequence", fmt.Sprintf(`{"server_id":%q,"sequence":4}`, serverID), []string{"application/json"}, ""},
		{"oversized", strings.Repeat(" ", MaxResponseBytes) + valid, []string{"application/json"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				for _, value := range tc.contentType {
					response.Header().Add("Content-Type", value)
				}
				if tc.encoding != "" {
					response.Header().Set("Content-Encoding", tc.encoding)
				}
				_, _ = io.WriteString(response, tc.body)
			}))
			defer server.Close()
			transport := newOwnedTransport(t, server, &fakeCredentials{credential: transportCredential})
			result, err := transport.Send(context.Background(), uploadstate.Request{Binding: transportBinding, Sequence: 3, Body: validTransportBody(t)})
			if err != nil || result != (uploadstate.Response{Outcome: uploadstate.Retry}) {
				t.Fatalf("got %#v error %v", result, err)
			}
		})
	}
}

func TestOriginCredentialTLSProxyRedirectAndHeaderPolicies(t *testing.T) {
	provider := &fakeCredentials{credential: transportCredential}
	for _, origin := range []string{
		"", "http://platform.example.com", "https://platform.example.com/", "https://PLATFORM.example.com",
		"https://user@platform.example.com", "https://platform.example.com?x=1", "https://platform.example.com#x",
		"https://platform.example.com:0", "https://platform.example.com:65536", "https://bad_host.example.com",
	} {
		if _, err := New(origin, provider); err != ErrConfig {
			t.Fatalf("origin %q returned %v", origin, err)
		}
	}
	if _, err := New("https://platform.example.com", nil); err != ErrConfig {
		t.Fatal("nil credential provider accepted")
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(response, `{"server_id":%q,"sequence":1}`, transportBinding.ServerID)
	}))
	defer server.Close()
	request := uploadstate.Request{Binding: transportBinding, Sequence: 1, Body: validTransportBody(t)}
	production, err := New(server.URL, provider)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = production.Send(context.Background(), request); err != ErrUnavailable {
		t.Fatalf("untrusted test certificate returned %v", err)
	}

	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("https_proxy", "http://127.0.0.1:1")
	trusted := newOwnedTransport(t, server, provider)
	if result, err := trusted.Send(context.Background(), request); err != nil || result.Outcome != uploadstate.Acknowledged {
		t.Fatalf("environment proxy affected fixed transport: %#v %v", result, err)
	}

	provider.err = errors.New("secret-canary-provider-error")
	if _, err := trusted.Send(context.Background(), request); err != ErrCredential || strings.Contains(err.Error(), "canary") {
		t.Fatalf("credential error leaked: %v", err)
	}
	provider.err = nil
	provider.credential = "hlc_secret-canary"
	if _, err := trusted.Send(context.Background(), request); err != ErrCredential {
		t.Fatalf("invalid credential returned %v", err)
	}

	var redirected atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
	defer target.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, target.URL+"/capture", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	redirectTransport := newOwnedTransport(t, redirect, &fakeCredentials{credential: transportCredential})
	if result, err := redirectTransport.Send(context.Background(), request); err != nil || result.Outcome != uploadstate.Retry || redirected.Load() != 0 {
		t.Fatalf("redirect policy failed: %#v %v target=%d", result, err, redirected.Load())
	}
}

func TestResponseHeaderAndDeadlineBoundsReturnFixedErrors(t *testing.T) {
	request := uploadstate.Request{Binding: transportBinding, Sequence: 1, Body: validTransportBody(t)}
	headerServer := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("X-Oversized", strings.Repeat("x", MaxResponseHeaderBytes*4))
		response.WriteHeader(http.StatusOK)
	}))
	defer headerServer.Close()
	transport := newOwnedTransport(t, headerServer, &fakeCredentials{credential: transportCredential})
	if _, err := transport.Send(context.Background(), request); err != ErrUnavailable {
		t.Fatalf("oversized headers returned %v", err)
	}

	releaseHandler := make(chan struct{})
	deadlineServer := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		select {
		case <-request.Context().Done():
		case <-releaseHandler:
		}
	}))
	transport = newOwnedTransport(t, deadlineServer, &fakeCredentials{credential: transportCredential})
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := transport.Send(ctx, request); err != ErrUnavailable {
		t.Fatalf("deadline returned %v", err)
	}
	close(releaseHandler)
	deadlineServer.Close()
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("caller deadline not honored: %v", elapsed)
	}
	if transport.client.Timeout != SendTimeout {
		t.Fatalf("direct transport timeout %v", transport.client.Timeout)
	}

	closedServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedTransport := newOwnedTransport(t, closedServer, &fakeCredentials{credential: transportCredential})
	closedServer.Close()
	if _, err := closedTransport.Send(context.Background(), request); err != ErrUnavailable || !reflect.DeepEqual(err, ErrUnavailable) {
		t.Fatalf("network error was not fixed: %v", err)
	}
}
