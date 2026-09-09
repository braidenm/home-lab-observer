package containerobs

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type localEndpoint struct {
	scheme  string
	address string
}

func parseLocalEndpoint(raw string) (localEndpoint, error) {
	if raw == "" || raw != strings.TrimSpace(raw) {
		return localEndpoint{}, errors.New("docker endpoint is empty or contains surrounding whitespace")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return localEndpoint{}, errors.New("docker endpoint is malformed")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "unix":
		if parsed.Host != "" || !path.IsAbs(parsed.Path) || parsed.Path == "/" || strings.ContainsRune(parsed.Path, '\x00') {
			return localEndpoint{}, errors.New("unix docker endpoint must be an absolute local socket path")
		}
		return localEndpoint{scheme: "unix", address: parsed.Path}, nil
	case "npipe":
		if parsed.Host != "" || !strings.HasPrefix(parsed.Path, "//./pipe/") {
			return localEndpoint{}, errors.New("named-pipe docker endpoint must be local")
		}
		name := strings.TrimPrefix(parsed.Path, "//./pipe/")
		if name == "" || strings.ContainsAny(name, "/\\\x00") {
			return localEndpoint{}, errors.New("named-pipe docker endpoint is malformed")
		}
		return localEndpoint{scheme: "npipe", address: `\\.\pipe\` + name}, nil
	default:
		return localEndpoint{}, fmt.Errorf("docker endpoint scheme %q is not supported", parsed.Scheme)
	}
}

func newLocalClient(raw string) (*http.Client, *http.Transport, error) {
	endpoint, err := parseLocalEndpoint(raw)
	if err != nil {
		return nil, nil, err
	}
	transport, err := nativeTransport(endpoint)
	if err != nil {
		return nil, nil, err
	}
	transport.Proxy = nil
	transport.DisableCompression = true
	transport.MaxConnsPerHost = maxStatsFanout
	transport.MaxIdleConnsPerHost = maxStatsFanout
	transport.IdleConnTimeout = 30 * time.Second
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return client, transport, nil
}
