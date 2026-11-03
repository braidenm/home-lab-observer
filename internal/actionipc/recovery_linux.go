//go:build linux

package actionipc

import (
	"context"
	"net"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

// ReadRecoveryContinuation separates broker ACK processing from frame transfer.
// The authenticated worker retains and rechecks its ORIGINAL native relay90s
// context and exact next-operation graph. This reader grants no query authority.
func ReadRecoveryContinuation(ctx context.Context, c *net.UnixConn) (actionprotocol.Envelope, error) {
	return readRecoveryContinuation(ctx, c, 5*time.Second, nil)
}

func readRecoveryContinuation(ctx context.Context, c *net.UnixConn, transferCap time.Duration, observe func(bool)) (actionprotocol.Envelope, error) {
	e, err := readPhaseMessage(ctx, c, transferCap, observe)
	if err != nil || e.Version != actionprotocol.WorkerVersion || e.Kind != "RECOVERY_QUERY" ||
		(e.Ordinal != "2" && e.Ordinal != "3" && e.Ordinal != "4" && e.Ordinal != "5") {
		return actionprotocol.Envelope{}, ErrFrame
	}
	return e, nil
}
