// Package platformtransport sends bounded snapshots and performs one-shot
// enrollment exchange against the fixed Platform Demo origin. It owns HTTP
// policy and wire credentials; it does not own persistence, scheduling,
// application retries, or activation.
package platformtransport

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

const (
	MaxResponseBytes       = 16 * 1024
	MaxResponseHeaderBytes = 16 * 1024
	SendTimeout            = uploadstate.RequestTimeout
)

var (
	ErrConfig      = errors.New("platform_transport_invalid_config")
	ErrRequest     = errors.New("platform_transport_invalid_request")
	ErrCredential  = errors.New("platform_transport_credential_unavailable")
	ErrUnavailable = errors.New("platform_transport_unavailable")

	serverPattern     = regexp.MustCompile(`^srv_[a-f0-9]{32}$`)
	connectorPattern  = regexp.MustCompile(`^agent_[a-f0-9]{32}$`)
	credentialPattern = regexp.MustCompile(`^hlc_[A-Za-z0-9_-]{43}$`)
)

// CredentialProvider returns the credential scoped to the complete expected
// binding. Implementations must not return a credential for a different binding.
type CredentialProvider interface {
	Credential(context.Context, uploadstate.Binding) (string, error)
}

type Transport struct {
	origin      url.URL
	credentials CredentialProvider
	client      *http.Client
}

// New constructs the production transport. Trust roots and all HTTP behavior
// are deliberately not caller-configurable.
func New(origin string, credentials CredentialProvider) (*Transport, error) {
	return newTransport(origin, credentials, nil)
}

// newTransport differs from New only by accepting owned test-server roots.
// It is package-private so production callers cannot weaken platform trust.
func newTransport(origin string, credentials CredentialProvider, roots *x509.CertPool) (*Transport, error) {
	parsed, ok := parseOrigin(origin)
	if !ok || credentials == nil {
		return nil, ErrConfig
	}
	return &Transport{
		origin:      *parsed,
		credentials: credentials,
		client:      newHTTPClient(roots),
	}, nil
}

func newHTTPClient(roots *x509.CertPool) *http.Client {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if roots != nil {
		tlsConfig.RootCAs = roots
	}
	httpTransport := &http.Transport{
		Proxy:                  nil,
		DialContext:            (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           2,
		MaxIdleConnsPerHost:    1,
		MaxConnsPerHost:        2,
		IdleConnTimeout:        30 * time.Second,
		TLSHandshakeTimeout:    2 * time.Second,
		ResponseHeaderTimeout:  4 * time.Second,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: MaxResponseHeaderBytes,
		DisableCompression:     true,
		TLSClientConfig:        tlsConfig,
	}
	return &http.Client{
		Transport: httpTransport,
		Timeout:   SendTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (t *Transport) Send(ctx context.Context, request uploadstate.Request) (uploadstate.Response, error) {
	if ctx == nil || !validRequestEnvelope(request) {
		return uploadstate.Response{}, ErrRequest
	}
	request.Body = bytes.Clone(request.Body)
	if remoteprojection.Validate(request.Body, request.Binding.ServerID) != nil {
		return uploadstate.Response{}, ErrRequest
	}
	requestContext, cancel := context.WithTimeout(ctx, SendTimeout)
	defer cancel()

	credential, err := t.credentials.Credential(requestContext, request.Binding)
	if err != nil || !credentialPattern.MatchString(credential) {
		return uploadstate.Response{}, ErrCredential
	}

	target := t.origin
	target.Path = "/v1/connectors/home-lab/servers/" + request.Binding.ServerID + "/snapshot"
	httpRequest, err := http.NewRequestWithContext(requestContext, http.MethodPut, target.String(), bytes.NewReader(request.Body))
	if err != nil {
		return uploadstate.Response{}, ErrRequest
	}
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Connector "+credential)
	httpRequest.Header.Set("X-Connector-Sequence", strconv.FormatInt(request.Sequence, 10))
	httpRequest.Header.Set("User-Agent", "")

	response, err := t.client.Do(httpRequest)
	if err != nil {
		return uploadstate.Response{}, ErrUnavailable
	}
	defer response.Body.Close()
	body, bounded := readBounded(response.Body)
	outcome := statusOutcome(response.StatusCode)
	if response.StatusCode != http.StatusOK {
		return uploadstate.Response{Outcome: outcome}, nil
	}
	if !bounded || response.ContentLength > MaxResponseBytes || !noContentEncoding(response.Header) || !jsonContentType(response.Header) {
		return uploadstate.Response{Outcome: uploadstate.Retry}, nil
	}
	serverID, sequence, ok := decodeAcknowledgement(body)
	if !ok || serverID != request.Binding.ServerID || sequence != request.Sequence {
		return uploadstate.Response{Outcome: uploadstate.Retry}, nil
	}
	return uploadstate.Response{Outcome: uploadstate.Acknowledged, ServerID: serverID, Sequence: sequence}, nil
}

// CloseIdleConnections releases pooled idle connections. It does not cancel
// or wait for an active Send call.
func (t *Transport) CloseIdleConnections() {
	if t != nil && t.client != nil {
		t.client.CloseIdleConnections()
	}
}

func validRequestEnvelope(request uploadstate.Request) bool {
	return serverPattern.MatchString(request.Binding.ServerID) &&
		connectorPattern.MatchString(request.Binding.ConnectorID) &&
		request.Sequence > 0 && len(request.Body) > 0 && len(request.Body) <= remoteprojection.MaxBytes
}

func statusOutcome(status int) uploadstate.Outcome {
	switch status {
	case http.StatusOK:
		return uploadstate.Acknowledged
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge:
		return uploadstate.Rejected
	case http.StatusUnauthorized:
		return uploadstate.CredentialRejected
	case http.StatusConflict:
		return uploadstate.Conflict
	case http.StatusTooManyRequests:
		return uploadstate.RateLimited
	default:
		return uploadstate.Retry
	}
}

func readBounded(reader io.Reader) ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(reader, MaxResponseBytes+1))
	return body, err == nil && len(body) <= MaxResponseBytes
}

func jsonContentType(header http.Header) bool {
	values := header.Values("Content-Type")
	if len(values) != 1 {
		return false
	}
	mediaType, parameters, err := mime.ParseMediaType(values[0])
	if err != nil || mediaType != "application/json" {
		return false
	}
	if len(parameters) == 0 {
		return true
	}
	if len(parameters) != 1 {
		return false
	}
	charset, present := parameters["charset"]
	return present && strings.EqualFold(charset, "utf-8")
}

func noContentEncoding(header http.Header) bool {
	return len(header.Values("Content-Encoding")) == 0
}

func decodeAcknowledgement(body []byte) (string, int64, bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return "", 0, false
	}
	seen := make(map[string]struct{})
	var serverID string
	var sequence int64
	for decoder.More() {
		keyToken, err := decoder.Token()
		key, isString := keyToken.(string)
		if err != nil || !isString {
			return "", 0, false
		}
		if _, duplicate := seen[key]; duplicate {
			return "", 0, false
		}
		seen[key] = struct{}{}
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil {
			return "", 0, false
		}
		switch key {
		case "server_id":
			if json.Unmarshal(raw, &serverID) != nil {
				return "", 0, false
			}
		case "sequence":
			if json.Unmarshal(raw, &sequence) != nil {
				return "", 0, false
			}
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return "", 0, false
	}
	if _, err = decoder.Token(); err != io.EOF {
		return "", 0, false
	}
	_, hasServer := seen["server_id"]
	_, hasSequence := seen["sequence"]
	return serverID, sequence, hasServer && hasSequence && serverPattern.MatchString(serverID) && sequence > 0
}

func parseOrigin(raw string) (*url.URL, bool) {
	if raw == "" || len(raw) > 2048 || strings.ContainsAny(raw, "?#") {
		return nil, false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.Opaque != "" || parsed.RawPath != "" || parsed.Host != strings.ToLower(parsed.Host) || parsed.String() != raw {
		return nil, false
	}
	host := parsed.Hostname()
	if host == "" || strings.Contains(host, "%") || strings.HasSuffix(parsed.Host, ":") ||
		(strings.HasPrefix(parsed.Host, "[") && net.ParseIP(host) == nil) || !validHost(host) {
		return nil, false
	}
	if port := parsed.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return nil, false
		}
	}
	return &url.URL{Scheme: "https", Host: parsed.Host}, true
}

func validHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) > 253 || strings.HasSuffix(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}
