// Package uploadstate implements pure, credential-free upload admission decisions.
// It contains no HTTP, filesystem, scheduler or collector implementation.
package uploadstate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"sync"
	"time"

	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
)

var (
	ErrConfig        = errors.New("upload_invalid_config")
	ErrRecovery      = errors.New("upload_recovery_required")
	ErrCanceled      = errors.New("upload_canceled")
	ErrBusy          = errors.New("upload_busy")
	ErrSnapshot      = errors.New("upload_invalid_snapshot")
	serverPattern    = regexp.MustCompile(`^srv_[a-f0-9]{32}$`)
	connectorPattern = regexp.MustCompile(`^agent_[a-f0-9]{32}$`)
)

const MaxAge = 2 * time.Minute
const MaxFuture = 30 * time.Second
const RequestTimeout = 5 * time.Second

type Outcome string

const (
	Idle               Outcome = "IDLE"
	SourceUnavailable  Outcome = "SOURCE_UNAVAILABLE"
	Retry              Outcome = "RETRY"
	Acknowledged       Outcome = "ACKNOWLEDGED"
	Expired            Outcome = "EXPIRED_DELIVERY_UNKNOWN"
	ClockSkew          Outcome = "CLOCK_SKEW"
	CredentialRejected Outcome = "CREDENTIAL_REJECTED"
	Conflict           Outcome = "CONFLICT"
	Rejected           Outcome = "REJECTED"
	Exhausted          Outcome = "SEQUENCE_EXHAUSTED"
)

type Binding struct{ ServerID, ConnectorID string }
type Pending struct {
	Sequence    int64
	Body        []byte
	Digest      [32]byte
	CollectedAt time.Time
}
type Record struct {
	Binding   Binding
	Watermark int64
	HasAck    bool
	LastAck   [32]byte
	Pending   *Pending
	Stopped   Outcome
}
type Request struct {
	Binding  Binding
	Sequence int64
	Body     []byte
}

// ACK correlation is checked here even when the adapter reports Acknowledged.
type Response struct {
	Outcome  Outcome
	ServerID string
	Sequence int64
}
type Source interface {
	Read(context.Context, string) ([]byte, error)
}
type Clock interface{ Now() time.Time }
type Transport interface {
	Send(context.Context, Request) (Response, error)
}

// Load must distinguish lost state from a provisioned initial record. Commit
// atomically compares the full expected record and durably replaces it. Neither
// operation may retain/mutate returned or supplied buffers after returning.
type Ledger interface {
	Load(context.Context) (Record, error)
	Commit(context.Context, Record, Record) error
}

type Machine struct {
	mu        sync.Mutex
	binding   Binding
	source    Source
	clock     Clock
	transport Transport
	ledger    Ledger
	record    *Record
	recovery  bool
}

func New(binding Binding, source Source, clock Clock, transport Transport, ledger Ledger) (*Machine, error) {
	if !validBinding(binding) || source == nil || clock == nil || transport == nil || ledger == nil {
		return nil, ErrConfig
	}
	return &Machine{binding: binding, source: source, clock: clock, transport: transport, ledger: ledger}, nil
}

// Step has no internal retry loop. Concurrent callers fail quickly; scheduling
// and backoff belong to the future worker. All externally returned errors are fixed.
func (m *Machine) Step(ctx context.Context) (Outcome, error) {
	if !m.mu.TryLock() {
		return "", ErrBusy
	}
	defer m.mu.Unlock()
	if m.recovery {
		return "", ErrRecovery
	}
	if ctx.Err() != nil {
		return "", ErrCanceled
	}
	if m.record == nil {
		r, err := m.ledger.Load(ctx)
		if err != nil || ValidateRecord(r, m.binding) != nil {
			m.recovery = true
			return "", ErrRecovery
		}
		r = clone(r)
		m.record = &r
	}
	if m.record.Stopped != "" {
		return m.record.Stopped, nil
	}
	now := m.clock.Now().UTC()
	if now.IsZero() || now.Year() < 1 || now.Year() > 9999 {
		return ClockSkew, nil
	}
	retired := false
	if p := m.record.Pending; p != nil {
		if p.CollectedAt.After(now.Add(MaxFuture)) {
			return ClockSkew, nil
		}
		if now.Sub(p.CollectedAt) > MaxAge {
			next := clone(*m.record)
			next.Pending = nil
			if err := m.commit(ctx, next); err != nil {
				return "", err
			}
			retired = true
		}
	}
	if m.record.Pending == nil {
		if ctx.Err() != nil {
			return "", ErrCanceled
		}
		body, err := m.source.Read(ctx, m.binding.ServerID)
		if ctx.Err() != nil {
			return "", ErrCanceled
		}
		if err != nil {
			if retired {
				return Expired, nil
			}
			return SourceUnavailable, nil
		}
		if len(body) > remoteprojection.MaxBytes {
			return "", ErrSnapshot
		}
		body = bytes.Clone(body)
		now = m.clock.Now().UTC()
		if now.IsZero() || now.Year() < 1 || now.Year() > 9999 {
			return ClockSkew, nil
		}
		at, ok := collectionTime(body, m.binding.ServerID)
		if !ok {
			return "", ErrSnapshot
		}
		if at.After(now.Add(MaxFuture)) {
			return ClockSkew, nil
		}
		if now.Sub(at) > MaxAge {
			return Expired, nil
		}
		digest := sha256.Sum256(body)
		if m.record.HasAck && digest == m.record.LastAck {
			return Idle, nil
		}
		if m.record.Watermark == math.MaxInt64 {
			return m.stop(ctx, Exhausted)
		}
		next := clone(*m.record)
		next.Watermark++
		next.Pending = &Pending{Sequence: next.Watermark, Body: body, Digest: digest, CollectedAt: at}
		if err := m.commit(ctx, next); err != nil {
			return "", err
		}
	}
	if ctx.Err() != nil {
		return "", ErrCanceled
	}
	p := m.record.Pending
	now = m.clock.Now().UTC()
	if now.IsZero() || now.Year() < 1 || now.Year() > 9999 || p.CollectedAt.After(now.Add(MaxFuture)) {
		return ClockSkew, nil
	}
	if now.Sub(p.CollectedAt) > MaxAge {
		return Expired, nil
	}
	requestContext, cancelRequest := context.WithTimeout(ctx, RequestTimeout)
	response, err := m.transport.Send(requestContext, Request{Binding: m.binding, Sequence: p.Sequence, Body: bytes.Clone(p.Body)})
	requestExpired := requestContext.Err() != nil
	cancelRequest()
	if ctx.Err() != nil {
		return "", ErrCanceled
	}
	if err != nil || requestExpired {
		return Retry, nil
	}
	switch response.Outcome {
	case Acknowledged:
		if response.ServerID != m.binding.ServerID || response.Sequence != p.Sequence {
			return Retry, nil
		}
		next := clone(*m.record)
		next.HasAck = true
		next.LastAck = p.Digest
		next.Pending = nil
		if err := m.commit(ctx, next); err != nil {
			return "", err
		}
		return Acknowledged, nil
	case CredentialRejected, Conflict, Rejected:
		return m.stop(ctx, response.Outcome)
	default:
		return Retry, nil
	}
}

func (m *Machine) stop(ctx context.Context, reason Outcome) (Outcome, error) {
	next := clone(*m.record)
	next.Stopped = reason
	if err := m.commit(ctx, next); err != nil {
		return "", err
	}
	return reason, nil
}
func (m *Machine) commit(ctx context.Context, next Record) error {
	if ctx.Err() != nil {
		return ErrCanceled
	}
	if m.ledger.Commit(ctx, clone(*m.record), clone(next)) != nil {
		m.recovery = true
		return ErrRecovery
	}
	next = clone(next)
	m.record = &next
	return nil
}
func validBinding(b Binding) bool {
	return serverPattern.MatchString(b.ServerID) && connectorPattern.MatchString(b.ConnectorID)
}
func collectionTime(body []byte, server string) (time.Time, bool) {
	if remoteprojection.Validate(body, server) != nil {
		return time.Time{}, false
	}
	var doc struct {
		CollectedAt string `json:"collected_at"`
	}
	if json.Unmarshal(body, &doc) != nil {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339Nano, doc.CollectedAt)
	return at, err == nil
}

// ValidateRecord checks the complete logical ledger invariants against a trusted
// expected binding. It does not authenticate state or detect restored backups.
func ValidateRecord(r Record, expected Binding) error {
	if !validBinding(expected) || !validRecord(r, expected) {
		return ErrRecovery
	}
	return nil
}

// CloneRecord detaches the pending body without validating it. Bound and validate
// untrusted records before copying them.
func CloneRecord(r Record) Record { return clone(r) }

func validRecord(r Record, b Binding) bool {
	if r.Binding != b || r.Watermark < 0 || (!r.HasAck && r.LastAck != [32]byte{}) || (r.HasAck && r.Watermark == 0) {
		return false
	}
	if r.Stopped != "" && r.Stopped != CredentialRejected && r.Stopped != Conflict && r.Stopped != Rejected && r.Stopped != Exhausted {
		return false
	}
	if r.Stopped == Exhausted && (r.Watermark != math.MaxInt64 || r.Pending != nil) {
		return false
	}
	if p := r.Pending; p != nil {
		at, ok := collectionTime(p.Body, b.ServerID)
		if !ok || p.Sequence <= 0 || p.Sequence != r.Watermark || !at.Equal(p.CollectedAt) || sha256.Sum256(p.Body) != p.Digest {
			return false
		}
	}
	return true
}
func clone(r Record) Record {
	if r.Pending != nil {
		p := *r.Pending
		p.Body = bytes.Clone(p.Body)
		r.Pending = &p
	}
	return r
}
