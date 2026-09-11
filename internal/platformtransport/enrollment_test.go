package platformtransport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/enrollmentcoord"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

const enrollmentGrant = "hle_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq"

func validEnrollmentRequestFixture() enrollmentcoord.ExchangeRequest {
	return enrollmentcoord.ExchangeRequest{
		Binding:         transportBinding,
		EnrollmentGrant: []byte(enrollmentGrant),
	}
}

func newOwnedEnrollment(t *testing.T, server *httptest.Server) *EnrollmentTransport {
	t.Helper()
	transport, err := newEnrollmentTransport(server.URL, ownedRoots(server))
	if err != nil {
		t.Fatal(err)
	}
	return transport
}

func TestEnrollmentExchangeUsesExactOneShotWireAndCorrelatesServer(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != enrollmentPath || request.URL.RawQuery != "" {
			t.Error("unexpected request target")
		}
		if request.Header.Get("Accept") != "application/json" || request.Header.Get("Content-Type") != "application/json" {
			t.Error("required JSON headers missing")
		}
		if request.Header.Get("Authorization") != "" || request.Header.Get("Cookie") != "" ||
			request.Header.Get("Referer") != "" || request.UserAgent() != "" {
			t.Error("ambient or misplaced credential header present")
		}
		body, err := io.ReadAll(request.Body)
		expected := fmt.Sprintf(`{"enrollment_secret":%q,"connector_instance_id":%q}`, enrollmentGrant, transportBinding.ConnectorID)
		if err != nil || string(body) != expected || request.ContentLength != int64(len(expected)) {
			t.Errorf("request body or length changed")
		}
		response.Header().Set("Content-Type", "application/json; charset=UTF-8")
		fmt.Fprintf(response, `{"server_id":%q,"connector_secret":%q,"future":{"secret-canary":"discard"}}`,
			transportBinding.ServerID, transportCredential)
	}))
	defer server.Close()

	transport := newOwnedEnrollment(t, server)
	request := validEnrollmentRequestFixture()
	originalGrant := bytes.Clone(request.EnrollmentGrant)
	result, err := transport.Exchange(context.Background(), request)
	if err != nil || result.Outcome != enrollmentcoord.ExchangeAccepted || result.ServerID != transportBinding.ServerID ||
		string(result.ConnectorSecret) != transportCredential {
		t.Fatalf("Exchange() = %#v, %v", result, err)
	}
	clear(result.ConnectorSecret)
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}
	if !bytes.Equal(request.EnrollmentGrant, originalGrant) {
		t.Fatal("adapter mutated caller-owned enrollment grant")
	}
}

func TestEnrollmentRequestIsNonRewindableAndUsesOneLogicalRoundTrip(t *testing.T) {
	transport, err := NewEnrollment("https://platform.example.com")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	transport.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		if request.GetBody != nil {
			t.Error("secret-bearing request is rewindable")
		}
		body, readErr := io.ReadAll(request.Body)
		if readErr != nil || !bytes.Contains(body, []byte(enrollmentGrant)) {
			t.Error("request body missing enrollment grant")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
				`{"server_id":%q,"connector_secret":%q}`, transportBinding.ServerID, transportCredential,
			))),
		}, nil
	})
	result, err := transport.Exchange(context.Background(), validEnrollmentRequestFixture())
	if err != nil || result.Outcome != enrollmentcoord.ExchangeAccepted || calls.Load() != 1 {
		t.Fatalf("Exchange() = %#v, %v, calls=%d", result, err, calls.Load())
	}
	clear(result.ConnectorSecret)
}

func TestOneShotBodySynchronizesConcurrentReadAndClose(t *testing.T) {
	raw := bytes.Repeat([]byte(enrollmentGrant), 128)
	body := newOneShotBody(raw)
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := io.ReadAll(body)
			if err != nil && err != io.ErrClosedPipe {
				t.Errorf("concurrent read returned %v", err)
			}
		}()
	}
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := body.Close(); err != nil {
				t.Errorf("concurrent close returned %v", err)
			}
		}()
	}
	wait.Wait()
	if _, err := body.Read(make([]byte, 1)); err != io.ErrClosedPipe {
		t.Fatalf("read after close returned %v", err)
	}
	for _, value := range raw {
		if value != 0 {
			t.Fatal("owned request buffer was not cleared")
		}
	}
}

func TestEnrollmentClosesRequestBodyAfterFailedAndEarlyRoundTrip(t *testing.T) {
	for _, test := range []struct {
		name     string
		response *http.Response
		err      error
	}{
		{"failed", nil, fmt.Errorf("secret-canary-failure")},
		{"early response", &http.Response{StatusCode: http.StatusInternalServerError, Header: http.Header{}, Body: http.NoBody}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport, err := NewEnrollment("https://platform.example.com")
			if err != nil {
				t.Fatal(err)
			}
			var captured io.ReadCloser
			transport.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
				captured = request.Body
				return test.response, test.err
			})
			result, err := transport.Exchange(context.Background(), validEnrollmentRequestFixture())
			if test.err != nil {
				if err != ErrUnavailable || result.Outcome != "" || result.ServerID != "" || len(result.ConnectorSecret) != 0 {
					t.Fatalf("Exchange() = %#v, %v", result, err)
				}
			} else if err != nil || result.Outcome != enrollmentcoord.ExchangeAmbiguous {
				t.Fatalf("Exchange() = %#v, %v", result, err)
			}
			if captured == nil {
				t.Fatal("round trip did not receive request body")
			}
			if _, err := captured.Read(make([]byte, 1)); err != io.ErrClosedPipe {
				t.Fatalf("late request-body read returned %v", err)
			}
		})
	}
}

func TestEnrollmentRejectsInvalidLengthsAndSyntaxBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	transport := newOwnedEnrollment(t, server)
	valid := validEnrollmentRequestFixture()
	cases := []enrollmentcoord.ExchangeRequest{
		{},
		{Binding: valid.Binding, EnrollmentGrant: []byte("hle_short")},
		{Binding: valid.Binding, EnrollmentGrant: bytes.Repeat([]byte{'x'}, 1<<16)},
		{Binding: valid.Binding, EnrollmentGrant: []byte("hle_!BCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq")},
		{Binding: valid.Binding, EnrollmentGrant: append([]byte(nil), valid.EnrollmentGrant...)},
		{Binding: valid.Binding, EnrollmentGrant: append([]byte(nil), valid.EnrollmentGrant...)},
	}
	cases[4].Binding.ServerID = "srv_short"
	cases[5].Binding.ConnectorID = strings.Repeat("a", 1<<16)
	for index, request := range cases {
		if _, err := transport.Exchange(context.Background(), request); err != ErrRequest {
			t.Fatalf("case %d returned %v", index, err)
		}
	}
	if _, err := transport.Exchange(nil, valid); err != ErrRequest {
		t.Fatalf("nil context returned %v", err)
	}
	if requests.Load() != 0 {
		t.Fatal("invalid enrollment reached network")
	}
}

func TestEnrollmentStatusMappingAndStrictRetryAfter(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		retryAfter []string
		want       enrollmentcoord.ExchangeOutcome
	}{
		{"rate limited", http.StatusTooManyRequests, []string{"60"}, enrollmentcoord.ExchangeRateLimited},
		{"rate limit missing delay", http.StatusTooManyRequests, nil, enrollmentcoord.ExchangeAmbiguous},
		{"rate limit wrong delay", http.StatusTooManyRequests, []string{"61"}, enrollmentcoord.ExchangeAmbiguous},
		{"rate limit duplicate delay", http.StatusTooManyRequests, []string{"60", "60"}, enrollmentcoord.ExchangeAmbiguous},
		{"bad request", http.StatusBadRequest, nil, enrollmentcoord.ExchangeRejected},
		{"unauthorized", http.StatusUnauthorized, nil, enrollmentcoord.ExchangeAmbiguous},
		{"server failure", http.StatusInternalServerError, nil, enrollmentcoord.ExchangeAmbiguous},
		{"unexpected", http.StatusNoContent, nil, enrollmentcoord.ExchangeAmbiguous},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				for _, value := range test.retryAfter {
					response.Header().Add("Retry-After", value)
				}
				response.WriteHeader(test.status)
				_, _ = response.Write(bytes.Repeat([]byte("private-canary"), MaxResponseBytes))
			}))
			defer server.Close()
			result, err := newOwnedEnrollment(t, server).Exchange(context.Background(), validEnrollmentRequestFixture())
			if err != nil || result.Outcome != test.want || result.ServerID != "" || len(result.ConnectorSecret) != 0 {
				t.Fatalf("Exchange() = %#v, %v", result, err)
			}
		})
	}
}

func TestEnrollmentSuccessDecoderRejectsHostileFramingAndMismatch(t *testing.T) {
	serverID := transportBinding.ServerID
	valid := fmt.Sprintf(`{"server_id":%q,"connector_secret":%q}`, serverID, transportCredential)
	tests := []struct {
		name        string
		body        string
		contentType []string
		encoding    string
	}{
		{"missing content type", valid, nil, ""},
		{"wrong content type", valid, []string{"text/plain"}, ""},
		{"extra content type", valid, []string{"application/json", "application/json"}, ""},
		{"wrong charset", valid, []string{"application/json; charset=latin1"}, ""},
		{"unknown content type parameter", valid, []string{"application/json; boundary=x"}, ""},
		{"encoded", valid, []string{"application/json"}, "gzip"},
		{"array", `[]`, []string{"application/json"}, ""},
		{"missing server", fmt.Sprintf(`{"connector_secret":%q}`, transportCredential), []string{"application/json"}, ""},
		{"missing credential", fmt.Sprintf(`{"server_id":%q}`, serverID), []string{"application/json"}, ""},
		{"wrong server type", fmt.Sprintf(`{"server_id":7,"connector_secret":%q}`, transportCredential), []string{"application/json"}, ""},
		{"wrong credential type", fmt.Sprintf(`{"server_id":%q,"connector_secret":7}`, serverID), []string{"application/json"}, ""},
		{"duplicate known", fmt.Sprintf(`{"server_id":%q,"server_id":%q,"connector_secret":%q}`, serverID, serverID, transportCredential), []string{"application/json"}, ""},
		{"escaped duplicate known", fmt.Sprintf(`{"server_id":%q,"\u0073erver_id":%q,"connector_secret":%q}`, serverID, serverID, transportCredential), []string{"application/json"}, ""},
		{"duplicate unknown", fmt.Sprintf(`{"server_id":%q,"connector_secret":%q,"future":1,"future":2}`, serverID, transportCredential), []string{"application/json"}, ""},
		{"trailing", valid + `{}`, []string{"application/json"}, ""},
		{"bom", "\ufeff" + valid, []string{"application/json"}, ""},
		{"mismatched server", fmt.Sprintf(`{"server_id":"srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","connector_secret":%q}`, transportCredential), []string{"application/json"}, ""},
		{"short credential", fmt.Sprintf(`{"server_id":%q,"connector_secret":"hlc_short"}`, serverID), []string{"application/json"}, ""},
		{"oversized", strings.Repeat(" ", MaxResponseBytes) + valid, []string{"application/json"}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				for _, value := range test.contentType {
					response.Header().Add("Content-Type", value)
				}
				if test.encoding != "" {
					response.Header().Set("Content-Encoding", test.encoding)
				}
				_, _ = io.WriteString(response, test.body)
			}))
			defer server.Close()
			result, err := newOwnedEnrollment(t, server).Exchange(context.Background(), validEnrollmentRequestFixture())
			if err != nil || result.Outcome != enrollmentcoord.ExchangeAmbiguous || result.ServerID != "" || len(result.ConnectorSecret) != 0 {
				t.Fatalf("Exchange() = %#v, %v", result, err)
			}
		})
	}
}

func TestContentEncodingPresenceRejectsBothSnapshotAndEnrollmentResponses(t *testing.T) {
	header := http.Header{
		"Content-Type":     []string{"application/json"},
		"Content-Encoding": []string{"", ""},
	}

	snapshot, err := New("https://platform.example.com", &fakeCredentials{credential: transportCredential})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header.Clone(),
			Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
				`{"server_id":%q,"sequence":1}`, transportBinding.ServerID,
			))),
		}, nil
	})
	result, err := snapshot.Send(context.Background(), uploadRequestForEncodingTest(t))
	if err != nil || result.Outcome != uploadstate.Retry {
		t.Fatalf("snapshot Send() = %#v, %v", result, err)
	}

	enrollment, err := NewEnrollment("https://platform.example.com")
	if err != nil {
		t.Fatal(err)
	}
	enrollment.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header.Clone(),
			Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
				`{"server_id":%q,"connector_secret":%q}`, transportBinding.ServerID, transportCredential,
			))),
		}, nil
	})
	exchange, err := enrollment.Exchange(context.Background(), validEnrollmentRequestFixture())
	if err != nil || exchange.Outcome != enrollmentcoord.ExchangeAmbiguous {
		t.Fatalf("enrollment Exchange() = %#v, %v", exchange, err)
	}
}

func TestEnrollmentOriginTLSProxyRedirectHeaderAndDeadlinePolicy(t *testing.T) {
	for _, origin := range []string{
		"", "http://platform.example.com", "https://platform.example.com/", "https://PLATFORM.example.com",
		"https://user@platform.example.com", "https://platform.example.com?x=1", "https://platform.example.com#x",
		"https://platform.example.com:", "https://platform.example.com:0", "https://bad_host.example.com",
	} {
		if _, err := NewEnrollment(origin); err != ErrConfig {
			t.Fatalf("origin %q returned %v", origin, err)
		}
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(response, `{"server_id":%q,"connector_secret":%q}`, transportBinding.ServerID, transportCredential)
	}))
	defer server.Close()
	production, err := NewEnrollment(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = production.Exchange(context.Background(), validEnrollmentRequestFixture()); err != ErrUnavailable {
		t.Fatalf("untrusted test certificate returned %v", err)
	}

	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("https_proxy", "http://127.0.0.1:1")
	trusted := newOwnedEnrollment(t, server)
	if result, err := trusted.Exchange(context.Background(), validEnrollmentRequestFixture()); err != nil || result.Outcome != enrollmentcoord.ExchangeAccepted {
		t.Fatalf("environment proxy affected transport: %#v, %v", result, err)
	}

	var redirected atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
	defer target.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, target.URL+"/capture", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	result, err := newOwnedEnrollment(t, redirect).Exchange(context.Background(), validEnrollmentRequestFixture())
	if err != nil || result.Outcome != enrollmentcoord.ExchangeAmbiguous || redirected.Load() != 0 {
		t.Fatalf("redirect policy failed: %#v, %v, target=%d", result, err, redirected.Load())
	}

	headerServer := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("X-Oversized", strings.Repeat("x", MaxResponseHeaderBytes*4))
		response.WriteHeader(http.StatusOK)
	}))
	defer headerServer.Close()
	if _, err := newOwnedEnrollment(t, headerServer).Exchange(context.Background(), validEnrollmentRequestFixture()); err != ErrUnavailable {
		t.Fatalf("oversized header returned %v", err)
	}

	release := make(chan struct{})
	deadlineServer := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		select {
		case <-request.Context().Done():
		case <-release:
		}
	}))
	deadlineTransport := newOwnedEnrollment(t, deadlineServer)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := deadlineTransport.Exchange(ctx, validEnrollmentRequestFixture()); err != ErrUnavailable {
		t.Fatalf("deadline returned %v", err)
	}
	close(release)
	deadlineServer.Close()
	if deadlineTransport.client.Timeout != SendTimeout {
		t.Fatalf("direct timeout = %v", deadlineTransport.client.Timeout)
	}

	fixedError, err := NewEnrollment("https://platform.example.com")
	if err != nil {
		t.Fatal(err)
	}
	fixedError.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("secret-canary-transport")
	})
	if _, err := fixedError.Exchange(context.Background(), validEnrollmentRequestFixture()); err != ErrUnavailable ||
		strings.Contains(err.Error(), "canary") {
		t.Fatalf("transport error leaked: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func uploadRequestForEncodingTest(t *testing.T) uploadstate.Request {
	t.Helper()
	return uploadstate.Request{Binding: transportBinding, Sequence: 1, Body: validTransportBody(t)}
}
