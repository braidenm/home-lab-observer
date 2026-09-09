//go:build windows

package containerobs

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/Microsoft/go-winio"
)

func nativeTransport(endpoint localEndpoint) (*http.Transport, error) {
	if endpoint.scheme != "npipe" {
		return nil, errors.New("unix docker endpoints are not supported on Windows")
	}
	return &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return winio.DialPipeContext(ctx, endpoint.address)
		},
	}, nil
}
