//go:build linux

package actionworker

import (
	"context"
	"errors"
	"net"

	"github.com/braidenm/home-lab-observer/internal/actionprofile"
)

var errBrokerAbsent = actionprofile.ErrBrokerAbsent

// Secret-process hardening stays in the worker. The shared profile capability
// owns only fixed path/kernel-peer admission, never credentials or authority.
func dialBroker(ctx context.Context, expected actionprofile.Identity) (*net.UnixConn, error) {
	i, err := workerIdentity(ctx)
	if err != nil || i != expected {
		return nil, ErrUnsafe
	}
	c, err := actionprofile.DialWorkerBroker(ctx, expected)
	if err != nil {
		if errors.Is(err, actionprofile.ErrBrokerAbsent) {
			return nil, errBrokerAbsent
		}
		return nil, ErrUnsafe
	}
	current, err := workerIdentity(ctx)
	if err != nil || current != expected {
		c.Close()
		return nil, ErrUnsafe
	}
	return c, nil
}
