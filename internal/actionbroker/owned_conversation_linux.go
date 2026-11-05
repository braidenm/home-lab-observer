//go:build linux

package actionbroker

import (
	"context"
	"net"
)

// Actual runnable worker ingress: authenticate before entering, consume BEGIN
// before a quiesced/no-query clean close, then choose a mandatory owned scope.
// A greeting never allocates an execution/control reservation or root authority.
func (c *executionConsumer) serveOwned(h *ownedWorker, conn *net.UnixConn) error {
	if conn == nil {
		return ErrRecoveryRequired
	}
	defer conn.Close()
	if c == nil || c.state == nil || h == nil || h.registry == nil || h.registry.fenced != &c.state.executionClosed || readWorkerBeginning(h.context, conn) != nil {
		return ErrRecoveryRequired
	}
	ctx, err := h.nonRelay()
	if err != nil {
		// Close AFTER BEGIN without a reservation once execution admission is fenced.
		return nil
	}
	joined := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { conn.Close(); close(joined) })
	defer func() {
		if !stop() {
			<-joined
		}
	}()
	if c.currentSource(ctx) != nil {
		return ErrRecoveryRequired
	}
	inv, err := c.state.selectWorkerInvocation(ctx, conn)
	if err != nil {
		return err
	}
	if c.currentSource(ctx) != nil {
		if inv != nil {
			c.abandon(inv)
		}
		return ErrRecoveryRequired
	}
	if inv == nil {
		return nil
	}
	return c.execute(ctx, inv)
}
