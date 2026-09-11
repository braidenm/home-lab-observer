package enrollmentcoord

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

const (
	testServer     = "srv_0123456789abcdef0123456789abcdef"
	testGrant      = "hle_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq"
	testCredential = "hlc_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq"
)

var testConnector = "agent_000102030405060708090a0b0c0d0e0f"

type fakeGrantSource struct {
	mu       sync.Mutex
	grant    Grant
	err      error
	calls    int
	before   func()
	log      *[]string
	returned []byte
}

func (f *fakeGrantSource) Read(context.Context) (Grant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.log != nil {
		*f.log = append(*f.log, "grant")
	}
	if f.before != nil {
		f.before()
	}
	f.returned = append([]byte(nil), f.grant.Secret...)
	return Grant{ExpectedServerID: f.grant.ExpectedServerID, Secret: f.returned}, f.err
}

type fakeAttemptStore struct {
	beginErr, readyErr error
	beginCalls         int
	readyCalls         int
	beginBinding       uploadstate.Binding
	readyBinding       uploadstate.Binding
	afterBegin         func()
	afterReady         func()
	log                *[]string
}

func (f *fakeAttemptStore) Begin(_ context.Context, binding uploadstate.Binding) error {
	f.beginCalls++
	f.beginBinding = binding
	if f.log != nil {
		*f.log = append(*f.log, "begin")
	}
	if f.afterBegin != nil {
		f.afterBegin()
	}
	return f.beginErr
}

func (f *fakeAttemptStore) MarkReady(_ context.Context, binding uploadstate.Binding) error {
	f.readyCalls++
	f.readyBinding = binding
	if f.log != nil {
		*f.log = append(*f.log, "ready")
	}
	if f.afterReady != nil {
		f.afterReady()
	}
	return f.readyErr
}

type fakeExchanger struct {
	response ExchangeResponse
	err      error
	calls    int
	request  ExchangeRequest
	after    func()
	mutate   bool
	log      *[]string
	returned []byte
}

func (f *fakeExchanger) Exchange(_ context.Context, request ExchangeRequest) (ExchangeResponse, error) {
	f.calls++
	f.request = ExchangeRequest{Binding: request.Binding, EnrollmentGrant: append([]byte(nil), request.EnrollmentGrant...)}
	if f.log != nil {
		*f.log = append(*f.log, "exchange")
	}
	if f.mutate && len(request.EnrollmentGrant) != 0 {
		request.EnrollmentGrant[0] = 'x'
	}
	if f.after != nil {
		f.after()
	}
	f.returned = append([]byte(nil), f.response.ConnectorSecret...)
	return ExchangeResponse{
		Outcome:         f.response.Outcome,
		ServerID:        f.response.ServerID,
		ConnectorSecret: f.returned,
	}, f.err
}

type fakeCredentialStore struct {
	err        error
	calls      int
	credential Credential
	after      func()
	mutate     bool
	log        *[]string
}

func (f *fakeCredentialStore) Create(_ context.Context, credential Credential) error {
	f.calls++
	f.credential = Credential{Binding: credential.Binding, Secret: append([]byte(nil), credential.Secret...)}
	if f.log != nil {
		*f.log = append(*f.log, "credential")
	}
	if f.mutate && len(credential.Secret) != 0 {
		credential.Secret[0] = 'x'
	}
	if f.after != nil {
		f.after()
	}
	return f.err
}

type fakeLedgerProvisioner struct {
	err     error
	calls   int
	binding uploadstate.Binding
	after   func()
	log     *[]string
}

func (f *fakeLedgerProvisioner) Provision(_ context.Context, binding uploadstate.Binding) error {
	f.calls++
	f.binding = binding
	if f.log != nil {
		*f.log = append(*f.log, "ledger")
	}
	if f.after != nil {
		f.after()
	}
	return f.err
}

type harness struct {
	coordinator *Coordinator
	grants      *fakeGrantSource
	attempts    *fakeAttemptStore
	exchanger   *fakeExchanger
	credentials *fakeCredentialStore
	ledgers     *fakeLedgerProvisioner
	log         []string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{}
	h.grants = &fakeGrantSource{grant: Grant{ExpectedServerID: testServer, Secret: []byte(testGrant)}, log: &h.log}
	h.attempts = &fakeAttemptStore{log: &h.log}
	h.exchanger = &fakeExchanger{response: acceptedResponse(), log: &h.log}
	h.credentials = &fakeCredentialStore{log: &h.log}
	h.ledgers = &fakeLedgerProvisioner{log: &h.log}
	var err error
	h.coordinator, err = newCoordinator(
		h.grants,
		h.attempts,
		h.exchanger,
		h.credentials,
		h.ledgers,
		bytes.NewReader([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}),
	)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func acceptedResponse() ExchangeResponse {
	return ExchangeResponse{Outcome: ExchangeAccepted, ServerID: testServer, ConnectorSecret: []byte(testCredential)}
}

func TestRunOrdersOneShotEnrollmentAndDetachesSecrets(t *testing.T) {
	h := newHarness(t)
	h.exchanger.mutate = true
	h.credentials.mutate = true

	outcome, err := h.coordinator.Run(context.Background())
	if err != nil || outcome != Ready {
		t.Fatalf("Run() = %q, %v", outcome, err)
	}
	if got, want := strings.Join(h.log, ","), "grant,begin,exchange,credential,ledger,ready"; got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
	binding := uploadstate.Binding{ServerID: testServer, ConnectorID: testConnector}
	if h.attempts.beginBinding != binding || h.attempts.readyBinding != binding ||
		h.exchanger.request.Binding != binding || h.credentials.credential.Binding != binding || h.ledgers.binding != binding {
		t.Fatal("binding was not identical across enrollment boundaries")
	}
	if string(h.exchanger.request.EnrollmentGrant) != testGrant || string(h.credentials.credential.Secret) != testCredential {
		t.Fatal("a secret-bearing boundary received mutated bytes")
	}
	if string(h.grants.grant.Secret) != testGrant {
		t.Fatal("coordinator mutated grant source state")
	}
	if !allZero(h.grants.returned) || !allZero(h.exchanger.returned) {
		t.Fatal("coordinator-owned secret buffers were not cleared")
	}
	if outcome, err := h.coordinator.Run(context.Background()); err != ErrUsed || outcome != "" {
		t.Fatalf("second Run() = %q, %v", outcome, err)
	}
	if h.exchanger.calls != 1 {
		t.Fatalf("exchange calls = %d, want 1", h.exchanger.calls)
	}
}

func TestRunRejectsInputAndPreAttemptFailuresWithoutExchange(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*harness)
		wantErr error
	}{
		{"invalid server", func(h *harness) { h.grants.grant.ExpectedServerID = "agent_0123456789abcdef0123456789abcdef" }, ErrInput},
		{"oversized server", func(h *harness) { h.grants.grant.ExpectedServerID = strings.Repeat("s", 1<<16) }, ErrInput},
		{"invalid grant", func(h *harness) { h.grants.grant.Secret = []byte("hle_short") }, ErrInput},
		{"oversized grant", func(h *harness) { h.grants.grant.Secret = bytes.Repeat([]byte{'s'}, 1<<16) }, ErrInput},
		{"grant unavailable", func(h *harness) { h.grants.err = errors.New("canary-grant") }, ErrGrant},
		{"begin unavailable", func(h *harness) { h.attempts.beginErr = errors.New("canary-attempt") }, ErrAttempt},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t)
			test.mutate(h)
			outcome, err := h.coordinator.Run(context.Background())
			if outcome != "" || err != test.wantErr || strings.Contains(err.Error(), "canary") {
				t.Fatalf("Run() = %q, %v", outcome, err)
			}
			if h.exchanger.calls != 0 || h.credentials.calls != 0 || h.ledgers.calls != 0 || h.attempts.readyCalls != 0 {
				t.Fatal("pre-attempt failure reached a downstream boundary")
			}
			if !allZero(h.grants.returned) {
				t.Fatal("returned grant buffer was not cleared")
			}
		})
	}

	h := newHarness(t)
	h.coordinator.random = bytes.NewReader([]byte{1, 2})
	if outcome, err := h.coordinator.Run(context.Background()); outcome != "" || err != ErrRandom {
		t.Fatalf("short random Run() = %q, %v", outcome, err)
	}
	if h.attempts.beginCalls != 0 || h.exchanger.calls != 0 {
		t.Fatal("random failure created an attempt or exchange")
	}
}

func TestRunClassifiesClosedExchangeOutcomesWithoutDownstreamWork(t *testing.T) {
	tests := []struct {
		name     string
		response ExchangeResponse
		err      error
		want     Outcome
	}{
		{"rate limited", ExchangeResponse{Outcome: ExchangeRateLimited}, nil, Retryable},
		{"rejected", ExchangeResponse{Outcome: ExchangeRejected}, nil, Rejected},
		{"explicit ambiguous", ExchangeResponse{Outcome: ExchangeAmbiguous}, nil, RecoveryRequired},
		{"unknown", ExchangeResponse{Outcome: "FUTURE"}, nil, RecoveryRequired},
		{"adapter failure", ExchangeResponse{}, errors.New("canary-transport"), RecoveryRequired},
		{"mismatched accepted server", ExchangeResponse{Outcome: ExchangeAccepted, ServerID: "srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ConnectorSecret: []byte(testCredential)}, nil, RecoveryRequired},
		{"invalid accepted credential", ExchangeResponse{Outcome: ExchangeAccepted, ServerID: testServer, ConnectorSecret: []byte("hlc_short")}, nil, RecoveryRequired},
		{"oversized accepted credential", ExchangeResponse{Outcome: ExchangeAccepted, ServerID: testServer, ConnectorSecret: bytes.Repeat([]byte{'s'}, 1<<16)}, nil, RecoveryRequired},
		{"fields on rate limit", ExchangeResponse{Outcome: ExchangeRateLimited, ServerID: testServer}, nil, RecoveryRequired},
		{"fields on rejection", ExchangeResponse{Outcome: ExchangeRejected, ConnectorSecret: []byte(testCredential)}, nil, RecoveryRequired},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t)
			h.exchanger.response = test.response
			h.exchanger.err = test.err
			outcome, err := h.coordinator.Run(context.Background())
			if err != nil || outcome != test.want {
				t.Fatalf("Run() = %q, %v; want %q, nil", outcome, err, test.want)
			}
			if h.exchanger.calls != 1 || h.credentials.calls != 0 || h.ledgers.calls != 0 || h.attempts.readyCalls != 0 {
				t.Fatal("non-accepted exchange reached provisioning")
			}
			if !allZero(h.exchanger.returned) {
				t.Fatal("returned response credential buffer was not cleared")
			}
		})
	}
}

func TestRunFailsClosedAtEveryPostAttemptBoundary(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*harness)
		log    string
	}{
		{"credential", func(h *harness) { h.credentials.err = errors.New("canary-credential") }, "grant,begin,exchange,credential"},
		{"ledger", func(h *harness) { h.ledgers.err = errors.New("canary-ledger") }, "grant,begin,exchange,credential,ledger"},
		{"ready", func(h *harness) { h.attempts.readyErr = errors.New("canary-ready") }, "grant,begin,exchange,credential,ledger,ready"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t)
			test.mutate(h)
			outcome, err := h.coordinator.Run(context.Background())
			if err != nil || outcome != RecoveryRequired || strings.Contains(outcomeString(outcome, err), "canary") {
				t.Fatalf("Run() = %q, %v", outcome, err)
			}
			if got := strings.Join(h.log, ","); got != test.log {
				t.Fatalf("call order = %q, want %q", got, test.log)
			}
		})
	}
}

func TestRunCancellationBeforeAndAfterAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := newHarness(t)
	if outcome, err := h.coordinator.Run(ctx); outcome != "" || err != ErrCanceled || h.grants.calls != 0 {
		t.Fatalf("pre-run cancellation = %q, %v, calls=%d", outcome, err, h.grants.calls)
	}

	tests := []struct {
		name   string
		mutate func(*harness, context.CancelFunc)
		log    string
	}{
		{"after begin", func(h *harness, cancel context.CancelFunc) { h.attempts.afterBegin = cancel }, "grant,begin"},
		{"after accepted exchange", func(h *harness, cancel context.CancelFunc) { h.exchanger.after = cancel }, "grant,begin,exchange"},
		{"after credential", func(h *harness, cancel context.CancelFunc) { h.credentials.after = cancel }, "grant,begin,exchange,credential"},
		{"after ledger", func(h *harness, cancel context.CancelFunc) { h.ledgers.after = cancel }, "grant,begin,exchange,credential,ledger"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newHarness(t)
			ctx, cancel := context.WithCancel(context.Background())
			test.mutate(h, cancel)
			outcome, err := h.coordinator.Run(ctx)
			if err != nil || outcome != RecoveryRequired {
				t.Fatalf("Run() = %q, %v", outcome, err)
			}
			if got := strings.Join(h.log, ","); got != test.log {
				t.Fatalf("call order = %q, want %q", got, test.log)
			}
		})
	}
}

func TestRunRejectsNilContextWithoutConsumingCoordinator(t *testing.T) {
	h := newHarness(t)
	if outcome, err := h.coordinator.Run(nil); outcome != "" || err != ErrInput {
		t.Fatalf("nil-context Run() = %q, %v", outcome, err)
	}
	if len(h.log) != 0 {
		t.Fatal("nil context reached a dependency")
	}
	if outcome, err := h.coordinator.Run(context.Background()); outcome != Ready || err != nil {
		t.Fatalf("valid Run() after nil context = %q, %v", outcome, err)
	}
}

func TestRunConcurrentCallerFailsBusyAndNeverStartsSecondExchange(t *testing.T) {
	h := newHarness(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	h.grants.before = func() {
		close(entered)
		<-release
	}
	done := make(chan error, 1)
	go func() {
		outcome, err := h.coordinator.Run(context.Background())
		if err == nil && outcome != Ready {
			err = errors.New("unexpected_outcome")
		}
		done <- err
	}()
	<-entered
	if outcome, err := h.coordinator.Run(context.Background()); outcome != "" || err != ErrBusy {
		t.Fatalf("concurrent Run() = %q, %v", outcome, err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if h.exchanger.calls != 1 {
		t.Fatalf("exchange calls = %d, want 1", h.exchanger.calls)
	}
}

func TestNewRequiresEveryPort(t *testing.T) {
	h := newHarness(t)
	tests := []struct {
		grants      GrantSource
		attempts    AttemptStore
		exchanger   Exchanger
		credentials CredentialStore
		ledgers     LedgerProvisioner
		random      io.Reader
	}{
		{nil, h.attempts, h.exchanger, h.credentials, h.ledgers, bytes.NewReader(make([]byte, identityBytes))},
		{h.grants, nil, h.exchanger, h.credentials, h.ledgers, bytes.NewReader(make([]byte, identityBytes))},
		{h.grants, h.attempts, nil, h.credentials, h.ledgers, bytes.NewReader(make([]byte, identityBytes))},
		{h.grants, h.attempts, h.exchanger, nil, h.ledgers, bytes.NewReader(make([]byte, identityBytes))},
		{h.grants, h.attempts, h.exchanger, h.credentials, nil, bytes.NewReader(make([]byte, identityBytes))},
		{h.grants, h.attempts, h.exchanger, h.credentials, h.ledgers, nil},
	}
	for _, test := range tests {
		if coordinator, err := newCoordinator(test.grants, test.attempts, test.exchanger, test.credentials, test.ledgers, test.random); coordinator != nil || err != ErrConfig {
			t.Fatalf("newCoordinator() = %#v, %v", coordinator, err)
		}
	}
}

func outcomeString(outcome Outcome, err error) string {
	if err == nil {
		return string(outcome)
	}
	return string(outcome) + err.Error()
}

func allZero(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}
