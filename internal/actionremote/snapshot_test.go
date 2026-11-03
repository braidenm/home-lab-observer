package actionremote

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

const syntheticSecret = "hlac_11111111111111111111111111111111.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

type credentialsFixture struct {
	mu        sync.Mutex
	deadlines []time.Time
	binding   Binding
}

func (p *credentialsFixture) Credential(ctx context.Context, binding Binding) (Credential, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.binding = binding
	deadline, _ := ctx.Deadline()
	p.deadlines = append(p.deadlines, deadline)
	return NewCredential(syntheticSecret)
}

func controlFixture(t *testing.T) (Binding, actionprotocol.SnapshotQuery, actionprotocol.SnapshotReceipt) {
	t.Helper()
	raw, err := os.ReadFile("../actionprotocol/testdata/paired-container-execution-v1.json")
	if err != nil {
		t.Fatal("public fixture")
	}
	var fixture struct {
		Objects map[string]json.RawMessage `json:"positive_codec_objects"`
	}
	if json.Unmarshal(raw, &fixture) != nil {
		t.Fatal("fixture JSON")
	}
	p, err := actionprotocol.DecodePublication(fixture.Objects["Publication"])
	if err != nil {
		t.Fatal("fixture publication")
	}
	request, err := actionprotocol.DecodeRequest(fixture.Objects["RequestReceipt"])
	if err != nil {
		t.Fatal("fixture request")
	}
	r, err := actionprotocol.DecodeSnapshotReceipt(fixture.Objects["SnapshotReceipt"])
	if err != nil {
		t.Fatal("fixture receipt")
	}
	r.SnapshotRevision = "1"
	binding := Binding{request.ServerID, request.OwnerUserID, request.ConnectorInstanceID}
	q := actionprotocol.SnapshotQuery{ServerID: binding.ServerID, OwnerUserID: binding.OwnerUserID, ConnectorInstanceID: binding.ConnectorInstanceID, PublicationID: r.PublicationID, Entries: p.Entries}
	r.SnapshotDigest, err = actionprotocol.SnapshotDigest(binding.ServerID, binding.OwnerUserID, binding.ConnectorInstanceID, r, p.Entries)
	if err != nil {
		t.Fatal("fixture snapshot digest")
	}
	return binding, q, r
}

func ownedClient(t *testing.T, server *httptest.Server, b Binding, provider CredentialProvider) *Client {
	t.Helper()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	c, err := newClient(server.URL, b, provider, roots)
	if err != nil {
		t.Fatal("owned TLS client")
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestSnapshotRefreshReadsVerifiedHeadAndPublishesOnceInsideOneBudget(t *testing.T) {
	b, q, receipt := controlFixture(t)
	provider := &credentialsFixture{}
	var calls []string
	var callsMu sync.Mutex
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callsMu.Lock()
		calls = append(calls, r.Method)
		callsMu.Unlock()
		if r.URL.Path != "/v1/connectors/home-lab/servers/"+b.ServerID+"/container-actions/approvals" || r.URL.RawQuery != "" ||
			r.Header.Get("Authorization") != "ActionConnector "+syntheticSecret || r.Header.Get("X-Home-Lab-Connector-Instance") != b.ConnectorInstanceID {
			t.Error("wrong purpose or fixed route")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(actionprotocol.SnapshotView{ServerID: b.ServerID, OwnerUserID: b.OwnerUserID, ConnectorInstanceID: b.ConnectorInstanceID, Readiness: "UNAVAILABLE", Entries: []actionprotocol.EntryView{}})
			return
		}
		if r.Method != "POST" {
			t.Error("unexpected method")
		}
		var p actionprotocol.Publication
		if json.NewDecoder(r.Body).Decode(&p) != nil || p.ExpectedSnapshotRevision != nil || p.PublicationID != q.PublicationID || len(p.Entries) != 32 {
			t.Error("missing head not used as exact first CAS")
		}
		json.NewEncoder(w).Encode(receipt)
	}))
	defer server.Close()
	c := ownedClient(t, server, b, provider)
	reply, err := c.RefreshSnapshot(context.Background(), q)
	callsMu.Lock()
	sequence := strings.Join(calls, ",")
	callsMu.Unlock()
	if err != nil || actionprotocol.MatchSnapshotReply(q, reply) != nil || sequence != "GET,POST" {
		t.Fatal("single fresh correlated publication")
	}
	if provider.binding != b || len(provider.deadlines) != 2 || provider.deadlines[0].IsZero() || !provider.deadlines[0].Equal(provider.deadlines[1]) {
		t.Fatal("control budget restarted between GET and POST")
	}
}

func TestPublicationRejectsValidDigestWithWrongCASRevision(t *testing.T) {
	for _, expected := range []string{"", "1", "9223372036854775807"} {
		t.Run("expected_"+expected, func(t *testing.T) {
			b, q, receipt := controlFixture(t)
			if expected == "" {
				receipt.SnapshotRevision = "2"
			}
			var err error
			receipt.SnapshotDigest, err = actionprotocol.SnapshotDigest(b.ServerID, b.OwnerUserID, b.ConnectorInstanceID, receipt, q.Entries)
			if err != nil {
				t.Fatal("synthetic digest")
			}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var p actionprotocol.Publication
				if json.NewDecoder(r.Body).Decode(&p) != nil {
					t.Error("fixture publication")
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(receipt)
			}))
			defer server.Close()
			p := actionprotocol.Publication{PublicationID: q.PublicationID, Entries: q.Entries}
			if expected != "" {
				p.ExpectedSnapshotRevision = &expected
			}
			if _, err := ownedClient(t, server, b, &credentialsFixture{}).Publish(context.Background(), p); err == nil {
				t.Fatal("wrong CAS revision admitted")
			}
		})
	}
}

func TestSnapshotRefreshNeverTreatsFailedHeadAsMissingOrRetriesPost(t *testing.T) {
	for _, failPost := range []bool{false, true} {
		t.Run(fmt.Sprint(failPost), func(t *testing.T) {
			b, q, _ := controlFixture(t)
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if failPost && r.Method == "GET" {
					json.NewEncoder(w).Encode(actionprotocol.SnapshotView{ServerID: b.ServerID, OwnerUserID: b.OwnerUserID, ConnectorInstanceID: b.ConnectorInstanceID, Readiness: "UNAVAILABLE", Entries: []actionprotocol.EntryView{}})
					return
				}
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer server.Close()
			c := ownedClient(t, server, b, &credentialsFixture{})
			reply, err := c.RefreshSnapshot(context.Background(), q)
			if err == nil || reply.Receipt != nil || reply.Outcome != "UNAVAILABLE" || calls.Load() != map[bool]int32{false: 1, true: 2}[failPost] {
				t.Fatal("failure became first head or automatic retry")
			}
		})
	}
}

func TestSnapshotHeadFailuresNeverPublishOrExposePrivateBodies(t *testing.T) {
	for _, scenario := range []string{"unauthorized", "forbidden", "conflict", "busy", "redirect", "type", "encoding", "duplicate-type", "oversized", "wrong-binding"} {
		t.Run(scenario, func(t *testing.T) {
			b, q, _ := controlFixture(t)
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				switch scenario {
				case "unauthorized":
					w.WriteHeader(401)
				case "forbidden":
					w.WriteHeader(403)
				case "conflict":
					w.WriteHeader(409)
				case "busy":
					w.WriteHeader(429)
				case "redirect":
					w.Header().Set("Location", "/another-path")
					w.WriteHeader(307)
				case "type":
					w.Header().Set("Content-Type", "text/plain")
				case "encoding":
					w.Header().Set("Content-Encoding", "gzip")
				case "duplicate-type":
					w.Header().Add("Content-Type", "text/plain")
				case "oversized":
					fmt.Fprint(w, strings.Repeat(" ", actionprotocol.MaxEnvelopeBytes+1))
					return
				case "wrong-binding":
					json.NewEncoder(w).Encode(actionprotocol.SnapshotView{ServerID: b.ServerID, OwnerUserID: "usr_" + strings.Repeat("f", 32), ConnectorInstanceID: b.ConnectorInstanceID, Readiness: "UNAVAILABLE", Entries: []actionprotocol.EntryView{}})
					return
				}
				fmt.Fprint(w, "private-error-canary-"+syntheticSecret)
			}))
			defer server.Close()
			client := ownedClient(t, server, b, &credentialsFixture{})
			reply, err := client.RefreshSnapshot(context.Background(), q)
			if err == nil || reply.Receipt != nil || calls.Load() != 1 || strings.Contains(err.Error(), "canary") || strings.Contains(err.Error(), syntheticSecret) {
				t.Fatal("unsafe head fallback, redirect/retry or error disclosure")
			}
		})
	}
}

func TestExpiredHeadRemainsExactCASInputAfterWorkerRestart(t *testing.T) {
	b, q, original := controlFixture(t)
	next := original
	next.SnapshotRevision = "2"
	next.SnapshotDigest, _ = actionprotocol.SnapshotDigest(b.ServerID, b.OwnerUserID, b.ConnectorInstanceID, next, q.Entries)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(actionprotocol.SnapshotView{ServerID: b.ServerID, OwnerUserID: b.OwnerUserID, ConnectorInstanceID: b.ConnectorInstanceID, Readiness: "UNAVAILABLE", SnapshotRevision: &original.SnapshotRevision, SnapshotDigest: &original.SnapshotDigest, AdvertisedAt: &original.AdvertisedAt, ExpiresAt: &original.ExpiresAt, Entries: []actionprotocol.EntryView{}})
			return
		}
		var p actionprotocol.Publication
		if json.NewDecoder(r.Body).Decode(&p) != nil || p.ExpectedSnapshotRevision == nil || *p.ExpectedSnapshotRevision != "1" {
			t.Error("expired head incorrectly reset")
		}
		json.NewEncoder(w).Encode(next)
	}))
	defer server.Close()
	client := ownedClient(t, server, b, &credentialsFixture{})
	if reply, err := client.RefreshSnapshot(context.Background(), q); err != nil || reply.Receipt == nil || reply.Receipt.SnapshotRevision != "2" {
		t.Fatal("stateless restart refresh")
	}
}

func TestInvalidOriginsAndClosedClientPerformNoRequests(t *testing.T) {
	b, _, _ := controlFixture(t)
	for _, origin := range []string{"http://example.invalid", "https://user:secret@example.invalid", "https://example.invalid/path", "https://example.invalid?x=1", "https://example.invalid#fragment", "https://EXAMPLE.invalid", "https://example.invalid:0443"} {
		if _, err := New(origin, b, &credentialsFixture{}); !errors.Is(err, ErrInvalid) {
			t.Fatal("noncanonical origin accepted")
		}
	}
	var zero Client
	if zero.Close() != nil {
		t.Fatal("zero client close")
	}
	if _, err := zero.SnapshotHead(context.Background()); err == nil {
		t.Fatal("zero client performed work")
	}
}

func TestCommittedPublicationWithLostResponseIsNotRetried(t *testing.T) {
	b, q, _ := controlFixture(t)
	var gets, committed atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			gets.Add(1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(actionprotocol.SnapshotView{ServerID: b.ServerID, OwnerUserID: b.OwnerUserID, ConnectorInstanceID: b.ConnectorInstanceID, Readiness: "UNAVAILABLE", Entries: []actionprotocol.EntryView{}})
			return
		}
		committed.Add(1) // Synthetic server accepted; response is deliberately lost.
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error("owned connection hijack")
			return
		}
		connection.Close()
	}))
	defer server.Close()
	client := ownedClient(t, server, b, &credentialsFixture{})
	reply, err := client.RefreshSnapshot(context.Background(), q)
	if err == nil || reply.Receipt != nil || reply.Outcome != "UNAVAILABLE" || gets.Load() != 1 || committed.Load() != 1 {
		t.Fatal("lost committed response retried or installed readiness")
	}
}

func TestCloseCancelsAndJoinsActiveHTTPWithoutReplacement(t *testing.T) {
	b, _, _ := controlFixture(t)
	entered := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() }))
	defer server.Close()
	client := ownedClient(t, server, b, &credentialsFixture{})
	finished := make(chan error, 1)
	go func() { _, err := client.SnapshotHead(context.Background()); finished <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("owned HTTP request did not enter")
	}
	if client.Close() != nil {
		t.Fatal("close")
	}
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("canceled active request succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("close did not join request")
	}
	if _, err := client.SnapshotHead(context.Background()); err == nil {
		t.Fatal("closed client admitted replacement")
	}
}

func TestActionCredentialFormattingIsRedactedAndWrongPurposeRefuses(t *testing.T) {
	c, err := NewCredential(syntheticSecret)
	if err != nil || strings.Contains(fmt.Sprintf("%v %#v", c, c), syntheticSecret) {
		t.Fatal("credential formatting leaked")
	}
	for _, secret := range []string{"", "hlc_" + strings.Repeat("A", 43), strings.Replace(syntheticSecret, "hlac_", "hlae_", 1)} {
		if _, err := NewCredential(secret); err == nil {
			t.Fatal("wrong credential purpose")
		}
	}
}
