//go:build !windows

package containerobs

import (
	"context"
	"errors"
	"net"
	"net/http"
)

func nativeTransport(endpoint localEndpoint) (*http.Transport, error) {
	if endpoint.scheme != "unix" {
		return nil, errors.New("named-pipe docker endpoints are only supported on Windows")
	}
	dialer := &net.Dialer{}
	return &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", endpoint.address)
		},
	}, nil
}
