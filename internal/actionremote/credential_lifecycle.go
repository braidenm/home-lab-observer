package actionremote

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

type CredentialState string

const (
	CredentialActive   CredentialState = "ACTIVE"
	CredentialPrepared CredentialState = "PREPARED"
	CredentialInactive CredentialState = "INACTIVE"
)

type CredentialMetadata struct {
	CredentialID string
	Revision     int64
	State        CredentialState
	ExpiresAt    time.Time
}

type EnrollmentSecret struct{ secret string }

func (EnrollmentSecret) String() string               { return "ActionEnrollment([redacted])" }
func (EnrollmentSecret) GoString() string             { return "ActionEnrollment([redacted])" }
func (EnrollmentSecret) MarshalJSON() ([]byte, error) { return nil, ErrCredential }

func NewEnrollmentSecret(raw string) (EnrollmentSecret, error) {
	if len(raw) != 81 || !id(raw[:37], "hlae_") || raw[37] != '.' {
		return EnrollmentSecret{}, ErrCredential
	}
	b, err := base64.RawURLEncoding.DecodeString(raw[38:])
	if err != nil || len(b) != 32 || base64.RawURLEncoding.EncodeToString(b) != raw[38:] {
		return EnrollmentSecret{}, ErrCredential
	}
	return EnrollmentSecret{raw}, nil
}

// Only trusted worker file translation consumes AuthorizationSecret. No implicit
// serialization, formatting, diagnostic or durable domain event may expose it.
type CredentialDisclosure struct {
	AuthorizationSecret string `json:"-"`
	Metadata            CredentialMetadata
}

func (CredentialDisclosure) String() string               { return "ActionCredentialDisclosure([redacted])" }
func (CredentialDisclosure) GoString() string             { return "ActionCredentialDisclosure([redacted])" }
func (CredentialDisclosure) MarshalJSON() ([]byte, error) { return nil, ErrCredential }

type SetupClient struct{ base *Client }
type setupProvider struct{}

func (setupProvider) Credential(context.Context, Binding) (Credential, error) {
	return Credential{}, ErrCredential
}

func NewSetupClient(origin string, binding Binding) (*SetupClient, error) {
	return newSetupClient(origin, binding, nil)
}
func newSetupClient(origin string, binding Binding, roots *x509.CertPool) (*SetupClient, error) {
	c, err := newClient(origin, binding, setupProvider{}, roots)
	if err != nil {
		return nil, err
	}
	return &SetupClient{c}, nil
}
func (c *SetupClient) Close() error {
	if c == nil {
		return nil
	}
	return c.base.Close()
}

// Private closed choice, never a public scheme/path/header override.
type lifecycleAuth struct {
	enrollment *EnrollmentSecret
	action     *Credential
}

func (a *lifecycleAuth) header() (string, error) {
	if a == nil {
		return "", ErrCredential
	}
	if a.enrollment != nil && a.action == nil {
		v, err := NewEnrollmentSecret(a.enrollment.secret)
		if err == nil {
			return "ActionEnrollment " + v.secret, nil
		}
	}
	if a.action != nil && a.enrollment == nil {
		v, err := NewCredential(a.action.secret)
		if err == nil {
			return "ActionConnector " + v.secret, nil
		}
	}
	return "", ErrCredential
}
func (c *SetupClient) request(ctx context.Context, method, suffix string, auth *lifecycleAuth) (int, []byte, error) {
	if c == nil || c.base == nil {
		return 0, nil, ErrUnavailable
	}
	return c.base.requestTyped(ctx, method, suffix, nil, "", auth)
}
func (c *SetupClient) Exchange(ctx context.Context, s EnrollmentSecret) (CredentialDisclosure, error) {
	status, raw, err := c.request(ctx, "POST", "/exchange", &lifecycleAuth{enrollment: &s})
	if err != nil {
		return CredentialDisclosure{}, err
	}
	if status != 201 {
		return CredentialDisclosure{}, ErrUnavailable
	}
	return decodeCredentialDisclosure(raw, CredentialActive)
}
func (c *SetupClient) Prepare(ctx context.Context, s Credential) (CredentialDisclosure, error) {
	status, raw, err := c.request(ctx, "POST", "/prepare", &lifecycleAuth{action: &s})
	if err != nil {
		return CredentialDisclosure{}, err
	}
	if status != 201 {
		return CredentialDisclosure{}, ErrUnavailable
	}
	d, err := decodeCredentialDisclosure(raw, CredentialPrepared)
	if err != nil || d.Metadata.CredentialID == s.secret[:37] {
		return CredentialDisclosure{}, ErrUnavailable
	}
	return d, nil
}
func (c *SetupClient) Activate(ctx context.Context, s Credential) (CredentialMetadata, error) {
	return c.metadata(ctx, "POST", "/activate", s, true)
}
func (c *SetupClient) Status(ctx context.Context, s Credential) (CredentialMetadata, error) {
	return c.metadata(ctx, "GET", "/status", s, false)
}
func (c *SetupClient) metadata(ctx context.Context, method, suffix string, s Credential, active bool) (CredentialMetadata, error) {
	status, raw, err := c.request(ctx, method, suffix, &lifecycleAuth{action: &s})
	if err != nil {
		return CredentialMetadata{}, err
	}
	if status != 200 {
		return CredentialMetadata{}, ErrUnavailable
	}
	m, err := decodeCredentialMetadata(raw)
	if err != nil || active && m.State != CredentialActive || m.State != CredentialInactive && m.CredentialID != s.secret[:37] {
		return CredentialMetadata{}, ErrUnavailable
	}
	return m, nil
}

var setupInteger = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
var setupInstant = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,6})?Z$`)

func setupObject(raw []byte, fields ...string) (map[string]json.RawMessage, error) {
	if !utf8.Valid(raw) || len(raw) == 0 || len(raw) > actionprotocol.MaxEnvelopeBytes {
		return nil, ErrUnavailable
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if setupJSON(d, 0) != nil {
		return nil, ErrUnavailable
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, ErrUnavailable
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil || len(values) != len(fields) {
		return nil, ErrUnavailable
	}
	for _, field := range fields {
		if _, ok := values[field]; !ok {
			return nil, ErrUnavailable
		}
	}
	return values, nil
}
func setupJSON(d *json.Decoder, depth int) error {
	if depth > 8 {
		return ErrUnavailable
	}
	token, err := d.Token()
	if err != nil {
		return ErrUnavailable
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			name, ok := key.(string)
			if err != nil || !ok || seen[name] {
				return ErrUnavailable
			}
			seen[name] = true
			if setupJSON(d, depth+1) != nil {
				return ErrUnavailable
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return ErrUnavailable
		}
	case json.Delim('['):
		for d.More() {
			if setupJSON(d, depth+1) != nil {
				return ErrUnavailable
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return ErrUnavailable
		}
	case json.Delim('}'), json.Delim(']'):
		return ErrUnavailable
	}
	return nil
}
func setupString(raw json.RawMessage) (string, error) {
	var s string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &s) != nil {
		return "", ErrUnavailable
	}
	return s, nil
}
func decodeCredentialMetadata(raw []byte) (CredentialMetadata, error) {
	v, err := setupObject(raw, "credential_id", "revision", "state", "expires_at")
	if err != nil {
		return CredentialMetadata{}, err
	}
	state, e1 := setupString(v["state"])
	number := string(v["revision"])
	revision, e2 := strconv.ParseInt(number, 10, 64)
	if e1 != nil || e2 != nil || !setupInteger.MatchString(number) {
		return CredentialMetadata{}, ErrUnavailable
	}
	if state == string(CredentialInactive) {
		if revision != 0 || string(v["credential_id"]) != "null" || string(v["expires_at"]) != "null" {
			return CredentialMetadata{}, ErrUnavailable
		}
		return CredentialMetadata{State: CredentialInactive}, nil
	}
	credential, e3 := setupString(v["credential_id"])
	expires, e4 := setupString(v["expires_at"])
	at, e5 := time.Parse(time.RFC3339Nano, expires)
	if revision <= 0 || state != string(CredentialActive) && state != string(CredentialPrepared) || e3 != nil || !id(credential, "hlac_") || e4 != nil || !setupInstant.MatchString(expires) || e5 != nil || at.Year() < 1 || at.Nanosecond()%1000 != 0 {
		return CredentialMetadata{}, ErrUnavailable
	}
	return CredentialMetadata{credential, revision, CredentialState(state), at}, nil
}
func decodeCredentialDisclosure(raw []byte, state CredentialState) (CredentialDisclosure, error) {
	v, err := setupObject(raw, "authorization_secret", "metadata")
	if err != nil {
		return CredentialDisclosure{}, err
	}
	secret, e1 := setupString(v["authorization_secret"])
	_, e2 := NewCredential(secret)
	metadata, e3 := decodeCredentialMetadata(v["metadata"])
	if e1 != nil || e2 != nil || e3 != nil || metadata.State != state || metadata.CredentialID != secret[:37] {
		return CredentialDisclosure{}, ErrUnavailable
	}
	return CredentialDisclosure{secret, metadata}, nil
}
