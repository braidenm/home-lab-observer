package platformtransport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

func TestDockerTransportRequiresBothApprovedGroupsBeforeCredential(t *testing.T) {
	body := richProcessesBody(t, workloadprojection.Consent{Processes: true, Containers: true})
	provider := &fakeCredentials{credential: transportCredential}
	var sends int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sends++
		got, _ := io.ReadAll(r.Body)
		if !bytes.Equal(got, body) {
			t.Error("Docker request bytes changed")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"server_id":%q,"sequence":1}`, transportBinding.ServerID)
	}))
	defer server.Close()
	transport, err := newRichProcessesDockerTransport(server.URL, provider, ownedRoots(server))
	if err != nil {
		t.Fatal(err)
	}
	defer transport.CloseIdleConnections()
	request := uploadstate.Request{Binding: transportBinding, Sequence: 1, Body: body}
	for i := 0; i < 2; i++ {
		if result, err := transport.Send(context.Background(), request); err != nil || result.Outcome != uploadstate.Acknowledged {
			t.Fatal("Docker request refused", err)
		}
	}
	for _, invalid := range [][]byte{
		validTransportBody(t),
		richProcessesBody(t, workloadprojection.Consent{}),
		richProcessesBody(t, workloadprojection.Consent{Processes: true}),
		richProcessesBody(t, workloadprojection.Consent{Containers: true}),
		bytes.Replace(body, []byte(transportBinding.ServerID), []byte("srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), -1),
		bytes.Repeat([]byte{'x'}, combinedprojection.MaxBytes+1),
	} {
		request.Body = invalid
		if _, err := transport.Send(context.Background(), request); err != ErrRequest {
			t.Fatal("unapproved Docker body accepted")
		}
	}
	if provider.calls != 2 || sends != 2 {
		t.Fatal("invalid body accessed credentials or network")
	}
	for _, rich := range []bool{false, true} {
		old, err := newTransport(server.URL, provider, ownedRoots(server), rich)
		if err != nil {
			t.Fatal(err)
		}
		request.Body = body
		if _, err := old.Send(context.Background(), request); err != ErrRequest {
			t.Fatal("existing profile adopted Docker body")
		}
		old.CloseIdleConnections()
	}
	if provider.calls != 2 {
		t.Fatal("old profile refusal accessed credential")
	}
}

func TestNamedDockerTransportRetainsFixedHTTPSPolicy(t *testing.T) {
	provider := &fakeCredentials{credential: transportCredential}
	if _, err := NewRichProcessesDocker("http://example.invalid", provider); err != ErrConfig {
		t.Fatal("plaintext transport accepted")
	}
	if _, err := NewRichProcessesDocker("https://example.invalid", nil); err != ErrConfig {
		t.Fatal("missing credential provider accepted")
	}
}
