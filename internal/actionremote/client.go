// Package actionremote owns only the fixed-origin typed action HTTP bridge.
// It neither grants permission, retries work nor owns broker policy or storage.
package actionremote

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

var (
	ErrInvalid          = errors.New("action_remote_invalid")
	ErrUnavailable      = errors.New("action_remote_unavailable")
	ErrCredential       = errors.New("action_remote_credential_unavailable")
	ErrDenied           = errors.New("action_remote_denied")
	ErrConflict         = errors.New("action_remote_conflict")
	ErrBusy             = errors.New("action_remote_busy")
	ErrClaimUnconfirmed = errors.New("action_remote_claim_unconfirmed")
)

const requestTimeout = 5 * time.Second

type Binding struct{ ServerID, OwnerUserID, ConnectorInstanceID string }

type Credential struct{ secret string }

func (Credential) String() string   { return "ActionCredential([redacted])" }
func (Credential) GoString() string { return "ActionCredential([redacted])" }

func NewCredential(raw string) (Credential, error) {
	if len(raw) != 81 || !id(raw[:37], "hlac_") || raw[37] != '.' {
		return Credential{}, ErrCredential
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw[38:])
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != raw[38:] {
		return Credential{}, ErrCredential
	}
	return Credential{raw}, nil
}

// CredentialProvider must honor context cancellation/deadlines and return only
// the exact requested binding's active secret. Close joins providers as well as
// HTTP calls; it cannot forcibly stop an uncooperative implementation.
type CredentialProvider interface {
	Credential(context.Context, Binding) (Credential, error)
}

type Client struct {
	origin      url.URL
	binding     Binding
	credentials CredentialProvider
	client      *http.Client
	mu          sync.Mutex
	closed      bool
	lifetime    context.Context
	cancel      context.CancelFunc
	active      sync.WaitGroup
}

func id(value, prefix string) bool {
	if len(value) != len(prefix)+32 || !strings.HasPrefix(value, prefix) {
		return false
	}
	for _, c := range value[len(prefix):] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func validBinding(b Binding) bool {
	return id(b.ServerID, "srv_") && id(b.OwnerUserID, "usr_") && id(b.ConnectorInstanceID, "agent_")
}

// New pins one explicitly configured HTTPS origin and uses system roots only.
// No caller-supplied client, proxy, headers, redirect or trust bypass is exposed.
func New(origin string, binding Binding, credentials CredentialProvider) (*Client, error) {
	return newClient(origin, binding, credentials, nil)
}

func newClient(origin string, binding Binding, credentials CredentialProvider, roots *x509.CertPool) (*Client, error) {
	u, err := url.Parse(origin)
	if err != nil || len(origin) > 2048 || !validBinding(binding) || credentials == nil || u.Scheme != "https" ||
		u.Host == "" || u.Host != strings.ToLower(u.Host) || u.User != nil || u.Path != "" || u.RawPath != "" ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.String() != origin ||
		strings.ContainsAny(origin, "?#") || !validHost(u.Hostname()) {
		return nil, ErrInvalid
	}
	if port := u.Port(); port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
			return nil, ErrInvalid
		}
	}
	if strings.HasSuffix(u.Host, ":") {
		return nil, ErrInvalid
	}
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	transport := &http.Transport{Proxy: nil, Protocols: protocols, DisableKeepAlives: true, DisableCompression: true,
		DialContext: (&net.Dialer{Timeout: 2 * time.Second}).DialContext, MaxConnsPerHost: 3,
		TLSHandshakeTimeout: 2 * time.Second, ResponseHeaderTimeout: 4 * time.Second, MaxResponseHeaderBytes: 16384,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}}
	c := &Client{origin: *u, binding: binding, credentials: credentials, client: &http.Client{Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	c.lifetime, c.cancel = context.WithCancel(context.Background())
	return c, nil
}

func validHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

// request is private and called only with code-owned paths/methods by typed
// operations. Fresh HTTP/1 connections and nil GetBody prevent mutation replay.
func (c *Client) request(parent context.Context, method, suffix string, body []byte) (int, []byte, error) {
	return c.requestCorrelated(parent, method, suffix, body, "")
}

func (c *Client) requestCorrelated(parent context.Context, method, suffix string, body []byte, correlationID string) (int, []byte, error) {
	return c.requestTyped(parent, method, suffix, body, correlationID, nil)
}

func (c *Client) requestTyped(parent context.Context, method, suffix string, body []byte, correlationID string, setup *lifecycleAuth) (int, []byte, error) {
	if c == nil || c.client == nil || c.credentials == nil || c.lifetime == nil || parent == nil || parent.Err() != nil || len(body) > actionprotocol.MaxEnvelopeBytes {
		return 0, nil, ErrUnavailable
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return 0, nil, ErrUnavailable
	}
	c.active.Add(1)
	c.mu.Unlock()
	defer c.active.Done()
	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	stop := context.AfterFunc(c.lifetime, cancel)
	defer func() { stop(); cancel() }()
	var authorization string
	if setup == nil {
		credential, err := c.credentials.Credential(ctx, c.binding)
		if err != nil {
			return 0, nil, ErrCredential
		}
		if _, err = NewCredential(credential.secret); err != nil {
			return 0, nil, ErrCredential
		}
		authorization = "ActionConnector " + credential.secret
	} else {
		var err error
		authorization, err = setup.header()
		if err != nil {
			return 0, nil, ErrCredential
		}
	}
	target := c.origin
	target.Path = "/v1/connectors/home-lab/servers/" + c.binding.ServerID + "/container-actions" + suffix
	if setup != nil {
		target.Path = "/v1/connectors/home-lab/servers/" + c.binding.ServerID + "/action-credentials" + suffix
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return 0, nil, ErrInvalid
	}
	req.GetBody = nil
	req.Header.Set("Authorization", authorization)
	req.Header.Set("X-Home-Lab-Connector-Instance", c.binding.ConnectorInstanceID)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "")
	if correlationID == "" {
		var correlation [16]byte
		if _, err = rand.Read(correlation[:]); err != nil {
			return 0, nil, ErrUnavailable
		}
		correlationID = hex.EncodeToString(correlation[:])
	}
	req.Header.Set("X-Correlation-Id", correlationID)
	response, err := c.client.Do(req)
	if err != nil {
		return 0, nil, ErrUnavailable
	}
	defer response.Body.Close()
	if ctx.Err() != nil {
		return 0, nil, ErrUnavailable
	}
	switch response.StatusCode {
	case http.StatusBadRequest:
		if setup != nil {
			return 0, nil, ErrInvalid
		}
		return 0, nil, ErrUnavailable
	case http.StatusUnauthorized:
		return 0, nil, ErrCredential
	case http.StatusForbidden:
		return 0, nil, ErrDenied
	case http.StatusConflict:
		return 0, nil, ErrConflict
	case http.StatusTooManyRequests:
		return 0, nil, ErrBusy
	case http.StatusNoContent:
		if response.ContentLength > 0 {
			return 0, nil, ErrUnavailable
		}
		raw, e := io.ReadAll(io.LimitReader(response.Body, 1))
		if e != nil || len(raw) != 0 || ctx.Err() != nil {
			return 0, nil, ErrUnavailable
		}
		return http.StatusNoContent, nil, nil
	case http.StatusOK:
	case http.StatusCreated:
		if setup == nil {
			return 0, nil, ErrUnavailable
		}
	default:
		return 0, nil, ErrUnavailable
	}
	contentType, parameters, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || len(response.Header.Values("Content-Type")) != 1 || contentType != "application/json" || len(parameters) > 1 || len(parameters) == 1 && !strings.EqualFold(parameters["charset"], "utf-8") ||
		len(response.Header.Values("Content-Encoding")) != 0 || response.ContentLength > actionprotocol.MaxEnvelopeBytes {
		return 0, nil, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, actionprotocol.MaxEnvelopeBytes+1))
	if err != nil || ctx.Err() != nil || len(raw) > actionprotocol.MaxEnvelopeBytes {
		return 0, nil, ErrUnavailable
	}
	return response.StatusCode, raw, nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	c.closed = true
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()
	c.active.Wait()
	if c.client != nil {
		c.client.CloseIdleConnections()
	}
	return nil
}
