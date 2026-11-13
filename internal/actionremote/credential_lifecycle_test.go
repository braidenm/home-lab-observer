package actionremote

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const enrollmentFixture = "hlae_11111111111111111111111111111111.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
const successorFixture = "hlac_22222222222222222222222222222222.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func setupMetadata(id string, revision, state, expires string) string {
	return `{"credential_id":` + id + `,"revision":` + revision + `,"state":"` + state + `","expires_at":` + expires + `}`
}

func TestCredentialLifecycleCodecKeepsLosslessRevisionAndActualInstantForms(t *testing.T) {
	for _, expiry := range []string{"2030-01-02T03:04:05Z", "2030-01-02T03:04:05.123Z", "2030-01-02T03:04:05.123456Z"} {
		raw := []byte(setupMetadata(`"hlac_22222222222222222222222222222222"`, "9223372036854775807", "ACTIVE", `"`+expiry+`"`))
		m, err := decodeCredentialMetadata(raw)
		if err != nil || m.Revision != int64(9223372036854775807) || m.State != CredentialActive || m.ExpiresAt.IsZero() {
			t.Fatal("lossless numeric revision/actual Instant form")
		}
	}
	valid := setupMetadata(`"hlac_22222222222222222222222222222222"`, "2", "PREPARED", `"2030-01-02T03:04:05.123456Z"`)
	for _, revision := range []string{`"2"`, "2.0", "2e0", "-1", "0", "9223372036854775808", "null"} {
		if _, err := decodeCredentialMetadata([]byte(strings.Replace(valid, `"revision":2`, `"revision":`+revision, 1))); err == nil {
			t.Fatal("noncanonical numeric revision")
		}
	}
	for _, changed := range []string{
		strings.Replace(valid, "123456Z", "1234567Z", 1),
		strings.Replace(valid, "05.123456Z", "05.123456+00:00", 1),
		strings.Replace(valid, "2030-", "0000-", 1),
		strings.Replace(valid, `"revision":2`, `"revision":2,"revision":2`, 1),
		strings.Replace(valid, `"state":`, `"State":`, 1),
		strings.Replace(valid, `"state":`, `"untrusted-secret-field":0,"state":`, 1), valid + `{}`,
	} {
		if _, err := decodeCredentialMetadata([]byte(changed)); err == nil {
			t.Fatal("malformed/unknown/duplicate metadata")
		}
	}
	if m, err := decodeCredentialMetadata([]byte(setupMetadata("null", "0", "INACTIVE", "null"))); err != nil || m.State != CredentialInactive || m.CredentialID != "" || !m.ExpiresAt.IsZero() {
		t.Fatal("exact inactive sentinel")
	}
	if _, err := NewEnrollmentSecret(syntheticSecret); err == nil {
		t.Fatal("ACTION secret accepted as bootstrap")
	}
}

func TestCredentialLifecycleActualTypedTLSJourneyAndRedaction(t *testing.T) {
	b, _, _ := controlFixture(t)
	var calls atomic.Int32
	old := "hlac_11111111111111111111111111111111"
	newID := "hlac_22222222222222222222222222222222"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Home-Lab-Connector-Instance") != b.ConnectorInstanceID || r.Header.Get("X-Correlation-Id") == "" || r.URL.RawQuery != "" {
			t.Error("closed binding/correlation")
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1))
		if len(body) != 0 {
			t.Error("lifecycle body was not absent")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Correlation-Id", r.Header.Get("X-Correlation-Id"))
		prefix := "/v1/connectors/home-lab/servers/" + b.ServerID + "/action-credentials"
		switch r.URL.Path {
		case prefix + "/exchange":
			if r.Method != "POST" || r.Header.Get("Authorization") != "ActionEnrollment "+enrollmentFixture {
				t.Error("exchange purpose")
			}
			w.WriteHeader(201)
			io.WriteString(w, `{"authorization_secret":"`+syntheticSecret+`","metadata":`+setupMetadata(`"`+old+`"`, "1", "ACTIVE", `"2030-02-01T00:00:00Z"`)+`}`)
		case prefix + "/prepare":
			if r.Method != "POST" || r.Header.Get("Authorization") != "ActionConnector "+syntheticSecret {
				t.Error("prepare active credential")
			}
			w.WriteHeader(201)
			io.WriteString(w, `{"authorization_secret":"`+successorFixture+`","metadata":`+setupMetadata(`"`+newID+`"`, "2", "PREPARED", `"2030-01-02T00:10:00Z"`)+`}`)
		case prefix + "/activate", prefix + "/status":
			if r.Method != map[bool]string{true: "GET", false: "POST"}[strings.HasSuffix(r.URL.Path, "/status")] || r.Header.Get("Authorization") != "ActionConnector "+successorFixture {
				t.Error("activate/status exact successor")
			}
			io.WriteString(w, setupMetadata(`"`+newID+`"`, "2", "ACTIVE", `"2030-02-01T00:00:00.123456Z"`))
		default:
			t.Error("arbitrary/old execution route")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	client, err := newSetupClient(server.URL, b, roots)
	if err != nil {
		t.Fatal("owned TLS setup client")
	}
	defer client.Close()
	enrollment, err := NewEnrollmentSecret(enrollmentFixture)
	if err != nil {
		t.Fatal("bootstrap fixture")
	}
	issued, err := client.Exchange(context.Background(), enrollment)
	if err != nil || issued.Metadata.CredentialID != old {
		t.Fatal("one-use exchange")
	}
	active, _ := NewCredential(issued.AuthorizationSecret)
	prepared, err := client.Prepare(context.Background(), active)
	if err != nil || prepared.Metadata.State != CredentialPrepared {
		t.Fatal("prepared handoff")
	}
	secret, _ := NewCredential(prepared.AuthorizationSecret)
	activated, err := client.Activate(context.Background(), secret)
	if err != nil || activated.State != CredentialActive || activated.CredentialID != newID {
		t.Fatal("activation")
	}
	status, err := client.Status(context.Background(), secret)
	if err != nil || status != activated || calls.Load() != 4 {
		t.Fatal("auth-only status/no retry")
	}
	for _, value := range []any{enrollment, issued, prepared} {
		if strings.Contains(fmt.Sprintf("%v %#v %+v", value, value, value), "AAAAAAAA") {
			t.Fatal("secret formatting leak")
		}
	}
	if _, err := json.Marshal(prepared); err == nil {
		t.Fatal("implicit secret serialization")
	}
}

func TestCredentialLifecycleLostMalformedAndDeniedRepliesNeverRetryOrDisclose(t *testing.T) {
	for _, mode := range []string{"lost", "invalid", "unavailable", "denied", "forbidden", "conflict", "busy", "wrong-id", "wrong-state", "duplicate", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			binding, _, _ := controlFixture(t)
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				io.Copy(io.Discard, r.Body)
				if mode == "lost" {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error("owned disconnect")
					} else {
						conn.Close()
					}
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("X-Correlation-Id", r.Header.Get("X-Correlation-Id"))
				code := map[string]int{"invalid": 400, "unavailable": 503, "denied": 401, "forbidden": 403, "conflict": 409, "busy": 429}[mode]
				if code != 0 {
					w.WriteHeader(code)
					io.WriteString(w, "secret-canary-"+enrollmentFixture)
					return
				}
				id, state := `"hlac_11111111111111111111111111111111"`, "ACTIVE"
				if mode == "wrong-id" {
					id = `"hlac_22222222222222222222222222222222"`
				}
				if mode == "wrong-state" {
					state = "PREPARED"
				}
				raw := `{"authorization_secret":"` + syntheticSecret + `","metadata":` + setupMetadata(id, "1", state, `"2030-02-01T00:00:00Z"`) + `}`
				if mode == "duplicate" {
					raw = strings.Replace(raw, `"revision":1`, `"revision":1,"revision":1`, 1)
				}
				if mode == "oversize" {
					raw += strings.Repeat(" ", 16385)
				}
				w.WriteHeader(201)
				io.WriteString(w, raw)
			}))
			defer server.Close()
			roots := x509.NewCertPool()
			roots.AddCert(server.Certificate())
			client, err := newSetupClient(server.URL, binding, roots)
			if err != nil {
				t.Fatal("owned setup")
			}
			defer client.Close()
			enrollment, _ := NewEnrollmentSecret(enrollmentFixture)
			result, err := client.Exchange(context.Background(), enrollment)
			if err == nil || result.AuthorizationSecret != "" || calls.Load() != 1 {
				t.Fatal("failed setup retried/disclosed")
			}
			if strings.Contains(fmt.Sprint(err), "canary") || strings.Contains(fmt.Sprint(err), "AAAAAAAA") {
				t.Fatal("raw failure escaped")
			}
			if mode == "invalid" && !errors.Is(err, ErrInvalid) || mode == "forbidden" && !errors.Is(err, ErrDenied) || mode == "unavailable" && !errors.Is(err, ErrUnavailable) || mode == "busy" && !errors.Is(err, ErrBusy) || mode == "denied" && !errors.Is(err, ErrCredential) || mode == "conflict" && !errors.Is(err, ErrConflict) {
				t.Fatal("finite failure classes collapsed")
			}
		})
	}
}

func TestCredentialLifecycleCloseCancelsAndJoinsActualRequest(t *testing.T) {
	binding, _, _ := controlFixture(t)
	entered := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	client, err := newSetupClient(server.URL, binding, roots)
	if err != nil {
		t.Fatal("owned client")
	}
	enrollment, _ := NewEnrollmentSecret(enrollmentFixture)
	done := make(chan error, 1)
	go func() { _, err := client.Exchange(context.Background(), enrollment); done <- err }()
	<-entered
	if client.Close() != nil || <-done == nil {
		t.Fatal("close failed to cancel/join")
	}
	if _, err := client.Exchange(context.Background(), enrollment); err == nil {
		t.Fatal("closed client used")
	}
}
