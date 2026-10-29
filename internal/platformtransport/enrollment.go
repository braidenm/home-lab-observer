package platformtransport

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sync"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/enrollmentcoord"
)

const (
	enrollmentPath    = "/v1/connectors/home-lab/enrollments:exchange"
	serverIDLength    = 36
	connectorIDLength = 38
	enrollmentLength  = 47
	credentialLength  = 47
	retryAfterSeconds = "60"
)

var (
	enrollmentPattern = regexp.MustCompile(`^hle_[A-Za-z0-9_-]{43}$`)
)

// EnrollmentTransport performs one bounded exchange against the fixed
// Platform origin. It contains no persistence, retry, sleep, or activation.
type EnrollmentTransport struct {
	origin url.URL
	client *http.Client
	rich   bool
}

var _ enrollmentcoord.Exchanger = (*EnrollmentTransport)(nil)

// NewEnrollment constructs the production exchange transport. Trust roots and
// all HTTP behavior are deliberately not caller-configurable.
func NewEnrollment(origin string) (*EnrollmentTransport, error) {
	return newEnrollmentTransport(origin, nil, false)
}

// NewRichProcessesEnrollment requires explicit server approval echoes for the
// one fixed rich profile. A legacy server that ignores the additive request
// fields is ambiguous, never an accepted rich enrollment.
func NewRichProcessesEnrollment(origin string) (*EnrollmentTransport, error) {
	return newEnrollmentTransport(origin, nil, true)
}

// newEnrollmentTransport differs only by accepting an owned test-server root.
func newEnrollmentTransport(origin string, roots *x509.CertPool, rich bool) (*EnrollmentTransport, error) {
	parsed, ok := parseOrigin(origin)
	if !ok {
		return nil, ErrConfig
	}
	return &EnrollmentTransport{origin: *parsed, client: newHTTPClient(roots), rich: rich}, nil
}

func (t *EnrollmentTransport) Exchange(
	ctx context.Context,
	request enrollmentcoord.ExchangeRequest,
) (enrollmentcoord.ExchangeResponse, error) {
	if ctx == nil || !validEnrollmentRequest(request) {
		return enrollmentcoord.ExchangeResponse{}, ErrRequest
	}

	requestBody := newOneShotBody(enrollmentRequestBody(request, t.rich))
	defer requestBody.Close()
	requestContext, cancel := context.WithTimeout(ctx, SendTimeout)
	defer cancel()
	target := t.origin
	target.Path = enrollmentPath
	httpRequest, err := http.NewRequestWithContext(requestContext, http.MethodPost, target.String(), requestBody)
	if err != nil {
		return enrollmentcoord.ExchangeResponse{}, ErrRequest
	}
	httpRequest.ContentLength = int64(requestBody.size())
	httpRequest.GetBody = nil
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("User-Agent", "")

	response, err := t.client.Do(httpRequest)
	if err != nil {
		return enrollmentcoord.ExchangeResponse{}, ErrUnavailable
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusTooManyRequests:
		if values := response.Header.Values("Retry-After"); len(values) == 1 && values[0] == retryAfterSeconds {
			return enrollmentcoord.ExchangeResponse{Outcome: enrollmentcoord.ExchangeRateLimited}, nil
		}
		return enrollmentcoord.ExchangeResponse{Outcome: enrollmentcoord.ExchangeAmbiguous}, nil
	case http.StatusBadRequest:
		return enrollmentcoord.ExchangeResponse{Outcome: enrollmentcoord.ExchangeRejected}, nil
	case http.StatusOK:
		// Decoded below.
	default:
		return enrollmentcoord.ExchangeResponse{Outcome: enrollmentcoord.ExchangeAmbiguous}, nil
	}

	body, bounded := readBounded(response.Body)
	defer clear(body)
	if !bounded || response.ContentLength > MaxResponseBytes || !noContentEncoding(response.Header) ||
		!jsonContentType(response.Header) {
		return enrollmentcoord.ExchangeResponse{Outcome: enrollmentcoord.ExchangeAmbiguous}, nil
	}
	serverID, credential, ok := decodeEnrollmentResponse(body, request.Binding.ServerID, t.rich)
	if !ok {
		clear(credential)
		return enrollmentcoord.ExchangeResponse{Outcome: enrollmentcoord.ExchangeAmbiguous}, nil
	}
	return enrollmentcoord.ExchangeResponse{
		Outcome:         enrollmentcoord.ExchangeAccepted,
		ServerID:        serverID,
		ConnectorSecret: credential,
	}, nil
}

// CloseIdleConnections releases pooled idle connections. It does not cancel
// or wait for an active Exchange call.
func (t *EnrollmentTransport) CloseIdleConnections() {
	if t != nil && t.client != nil {
		t.client.CloseIdleConnections()
	}
}

func validEnrollmentRequest(request enrollmentcoord.ExchangeRequest) bool {
	return len(request.Binding.ServerID) == serverIDLength &&
		len(request.Binding.ConnectorID) == connectorIDLength &&
		len(request.EnrollmentGrant) == enrollmentLength &&
		serverPattern.MatchString(request.Binding.ServerID) &&
		connectorPattern.MatchString(request.Binding.ConnectorID) &&
		enrollmentPattern.Match(request.EnrollmentGrant)
}

func enrollmentRequestBody(request enrollmentcoord.ExchangeRequest, rich bool) []byte {
	body := make([]byte, 0, 128)
	body = append(body, `{"enrollment_secret":"`...)
	body = append(body, request.EnrollmentGrant...)
	body = append(body, `","connector_instance_id":"`...)
	body = append(body, request.Binding.ConnectorID...)
	if rich {
		body = append(body, `","collection_profile":"`...)
		body = append(body, connectedprofile.RichProcessesProfile...)
		body = append(body, `","collection_definition_sha256":"`...)
		body = append(body, connectedprofile.RichProcessesDefinitionSHA256...)
	}
	body = append(body, `"}`...)
	return body
}

func decodeEnrollmentResponse(body []byte, expectedServerID string, rich bool) (string, []byte, bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return "", nil, false
	}
	seen := make(map[string]struct{})
	var serverID, credential, profile, definition string
	for decoder.More() {
		keyToken, err := decoder.Token()
		key, isString := keyToken.(string)
		if err != nil || !isString {
			return "", nil, false
		}
		if _, duplicate := seen[key]; duplicate {
			return "", nil, false
		}
		seen[key] = struct{}{}
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil {
			return "", nil, false
		}
		switch key {
		case "server_id":
			if json.Unmarshal(raw, &serverID) != nil {
				return "", nil, false
			}
		case "connector_secret":
			if json.Unmarshal(raw, &credential) != nil {
				return "", nil, false
			}
		case "collection_profile":
			if rich && json.Unmarshal(raw, &profile) != nil {
				return "", nil, false
			}
		case "collection_definition_sha256":
			if rich && json.Unmarshal(raw, &definition) != nil {
				return "", nil, false
			}
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') {
		return "", nil, false
	}
	if _, err = decoder.Token(); err != io.EOF {
		return "", nil, false
	}
	_, hasServer := seen["server_id"]
	_, hasCredential := seen["connector_secret"]
	_, hasProfile := seen["collection_profile"]
	_, hasDefinition := seen["collection_definition_sha256"]
	if !hasServer || !hasCredential || len(serverID) != serverIDLength || serverID != expectedServerID ||
		!serverPattern.MatchString(serverID) || len(credential) != credentialLength ||
		!credentialPattern.MatchString(credential) ||
		(rich && (!hasProfile || !hasDefinition || profile != connectedprofile.RichProcessesProfile || definition != connectedprofile.RichProcessesDefinitionSHA256)) {
		return "", nil, false
	}
	return serverID, []byte(credential), true
}

type oneShotBody struct {
	mu     sync.Mutex
	buffer []byte
	offset int
	closed bool
}

func newOneShotBody(buffer []byte) *oneShotBody {
	return &oneShotBody{buffer: buffer}
}

func (b *oneShotBody) Read(destination []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, io.ErrClosedPipe
	}
	if b.offset == len(b.buffer) {
		return 0, io.EOF
	}
	count := copy(destination, b.buffer[b.offset:])
	b.offset += count
	return count, nil
}

func (b *oneShotBody) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	clear(b.buffer)
	b.buffer = nil
	b.closed = true
	return nil
}

func (b *oneShotBody) size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.buffer)
}
