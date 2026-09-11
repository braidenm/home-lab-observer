// Package enrollmentcoord implements pure, fail-closed enrollment ordering.
// It contains no HTTP, filesystem, service-manager, or worker activation code.
package enrollmentcoord

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"regexp"
	"sync"

	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

var (
	ErrConfig   = errors.New("enrollment_invalid_config")
	ErrCanceled = errors.New("enrollment_canceled")
	ErrBusy     = errors.New("enrollment_busy")
	ErrUsed     = errors.New("enrollment_already_used")
	ErrGrant    = errors.New("enrollment_grant_unavailable")
	ErrInput    = errors.New("enrollment_invalid_input")
	ErrRandom   = errors.New("enrollment_random_unavailable")
	ErrAttempt  = errors.New("enrollment_attempt_unavailable")

	serverPattern     = regexp.MustCompile(`^srv_[a-f0-9]{32}$`)
	connectorPattern  = regexp.MustCompile(`^agent_[a-f0-9]{32}$`)
	grantPattern      = regexp.MustCompile(`^hle_[A-Za-z0-9_-]{43}$`)
	credentialPattern = regexp.MustCompile(`^hlc_[A-Za-z0-9_-]{43}$`)
)

const (
	identityBytes    = 16
	serverIDLength   = 36
	grantLength      = 47
	credentialLength = 47
)

type Outcome string

const (
	Ready            Outcome = "READY"
	Retryable        Outcome = "RETRYABLE"
	Rejected         Outcome = "REJECTED"
	RecoveryRequired Outcome = "RECOVERY_REQUIRED"
)

type ExchangeOutcome string

const (
	ExchangeAccepted    ExchangeOutcome = "ACCEPTED"
	ExchangeRateLimited ExchangeOutcome = "RATE_LIMITED"
	ExchangeRejected    ExchangeOutcome = "REJECTED"
	ExchangeAmbiguous   ExchangeOutcome = "AMBIGUOUS"
)

// Grant is a detached, bounded enrollment input. Secret must be a one-use hle_
// credential. No implementation may log or retain it after Read returns.
type Grant struct {
	ExpectedServerID string
	Secret           []byte
}

// ExchangeRequest is the only secret-bearing request passed by the kernel.
// Exchanger must not retain or mutate it after Exchange returns.
type ExchangeRequest struct {
	Binding         uploadstate.Binding
	EnrollmentGrant []byte
}

// ExchangeResponse is a closed semantic response from a later HTTPS adapter.
// ConnectorSecret is required only for ExchangeAccepted. The adapter must not
// place raw response or transport details in errors.
type ExchangeResponse struct {
	Outcome         ExchangeOutcome
	ServerID        string
	ConnectorSecret []byte
}

type Credential struct {
	Binding uploadstate.Binding
	Secret  []byte
}

type GrantSource interface {
	Read(context.Context) (Grant, error)
}

// Begin must exclusively and durably create an ATTEMPTED marker for binding.
// MarkReady must durably change that same marker to READY and reject mismatch.
type AttemptStore interface {
	Begin(context.Context, uploadstate.Binding) error
	MarkReady(context.Context, uploadstate.Binding) error
}

type Exchanger interface {
	Exchange(context.Context, ExchangeRequest) (ExchangeResponse, error)
}

// Create must exclusively persist the credential under binding and must not
// retain or mutate the supplied slice after returning.
type CredentialStore interface {
	Create(context.Context, Credential) error
}

// Provision creates, validates, and closes a proven-new zero-watermark ledger.
type LedgerProvisioner interface {
	Provision(context.Context, uploadstate.Binding) error
}

type Coordinator struct {
	mu          sync.Mutex
	running     bool
	used        bool
	grants      GrantSource
	attempts    AttemptStore
	exchanger   Exchanger
	credentials CredentialStore
	ledgers     LedgerProvisioner
	random      io.Reader
}

func New(
	grants GrantSource,
	attempts AttemptStore,
	exchanger Exchanger,
	credentials CredentialStore,
	ledgers LedgerProvisioner,
) (*Coordinator, error) {
	return newCoordinator(grants, attempts, exchanger, credentials, ledgers, rand.Reader)
}

func newCoordinator(
	grants GrantSource,
	attempts AttemptStore,
	exchanger Exchanger,
	credentials CredentialStore,
	ledgers LedgerProvisioner,
	random io.Reader,
) (*Coordinator, error) {
	if grants == nil || attempts == nil || exchanger == nil || credentials == nil || ledgers == nil || random == nil {
		return nil, ErrConfig
	}
	return &Coordinator{
		grants:      grants,
		attempts:    attempts,
		exchanger:   exchanger,
		credentials: credentials,
		ledgers:     ledgers,
		random:      random,
	}, nil
}

// Run performs exactly one enrollment attempt. It never retries, sleeps,
// removes partial state, starts a worker, or exposes dependency errors.
func (c *Coordinator) Run(ctx context.Context) (Outcome, error) {
	if ctx == nil {
		return "", ErrInput
	}
	if err := c.start(); err != nil {
		return "", err
	}
	defer c.finish()

	if ctx.Err() != nil {
		return "", ErrCanceled
	}
	grant, err := c.grants.Read(ctx)
	if err != nil {
		clear(grant.Secret)
		return "", fixedPreAttemptError(ctx, ErrGrant)
	}
	if len(grant.ExpectedServerID) != serverIDLength || len(grant.Secret) != grantLength {
		clear(grant.Secret)
		return "", ErrInput
	}
	grantSecret := append([]byte(nil), grant.Secret...)
	clear(grant.Secret)
	defer clear(grantSecret)
	if !serverPattern.MatchString(grant.ExpectedServerID) || !grantPattern.Match(grantSecret) {
		return "", ErrInput
	}

	identity := make([]byte, identityBytes)
	if _, err := io.ReadFull(c.random, identity); err != nil {
		clear(identity)
		return "", fixedPreAttemptError(ctx, ErrRandom)
	}
	binding := uploadstate.Binding{
		ServerID:    grant.ExpectedServerID,
		ConnectorID: "agent_" + hex.EncodeToString(identity),
	}
	clear(identity)
	if !connectorPattern.MatchString(binding.ConnectorID) {
		return "", ErrRandom
	}

	if err := c.attempts.Begin(ctx, binding); err != nil {
		return "", fixedPreAttemptError(ctx, ErrAttempt)
	}
	if ctx.Err() != nil {
		return RecoveryRequired, nil
	}

	requestSecret := append([]byte(nil), grantSecret...)
	response, err := c.exchanger.Exchange(ctx, ExchangeRequest{
		Binding:         binding,
		EnrollmentGrant: requestSecret,
	})
	clear(requestSecret)
	if err != nil {
		clear(response.ConnectorSecret)
		return RecoveryRequired, nil
	}

	switch response.Outcome {
	case ExchangeRateLimited:
		if response.ServerID != "" || len(response.ConnectorSecret) != 0 {
			clear(response.ConnectorSecret)
			return RecoveryRequired, nil
		}
		return Retryable, nil
	case ExchangeRejected:
		if response.ServerID != "" || len(response.ConnectorSecret) != 0 {
			clear(response.ConnectorSecret)
			return RecoveryRequired, nil
		}
		return Rejected, nil
	case ExchangeAccepted:
		if response.ServerID != binding.ServerID || len(response.ConnectorSecret) != credentialLength {
			clear(response.ConnectorSecret)
			return RecoveryRequired, nil
		}
	case ExchangeAmbiguous:
		clear(response.ConnectorSecret)
		return RecoveryRequired, nil
	default:
		clear(response.ConnectorSecret)
		return RecoveryRequired, nil
	}
	responseSecret := append([]byte(nil), response.ConnectorSecret...)
	clear(response.ConnectorSecret)
	defer clear(responseSecret)
	if !credentialPattern.Match(responseSecret) {
		return RecoveryRequired, nil
	}

	if ctx.Err() != nil {
		return RecoveryRequired, nil
	}
	credentialSecret := append([]byte(nil), responseSecret...)
	if err := c.credentials.Create(ctx, Credential{Binding: binding, Secret: credentialSecret}); err != nil {
		clear(credentialSecret)
		return RecoveryRequired, nil
	}
	clear(credentialSecret)
	if ctx.Err() != nil {
		return RecoveryRequired, nil
	}
	if err := c.ledgers.Provision(ctx, binding); err != nil {
		return RecoveryRequired, nil
	}
	if ctx.Err() != nil {
		return RecoveryRequired, nil
	}
	if err := c.attempts.MarkReady(ctx, binding); err != nil {
		return RecoveryRequired, nil
	}
	return Ready, nil
}

func (c *Coordinator) start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return ErrBusy
	}
	if c.used {
		return ErrUsed
	}
	c.running = true
	c.used = true
	return nil
}

func (c *Coordinator) finish() {
	c.mu.Lock()
	c.running = false
	c.mu.Unlock()
}

func fixedPreAttemptError(ctx context.Context, fallback error) error {
	if ctx.Err() != nil {
		return ErrCanceled
	}
	return fallback
}
