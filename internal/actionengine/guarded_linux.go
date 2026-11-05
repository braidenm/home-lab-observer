//go:build linux

package actionengine

import (
	"context"
	"net"

	"github.com/braidenm/home-lab-observer/internal/actionbudget"
	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

// OpenGuarded preserves Open's fixed rootful socket and witness checks. No
// unguarded adapter escapes the paired broker's construction path.
func OpenGuarded(ctx context.Context, approved SocketWitness) (*GuardedEngine, error) {
	a, err := Open(ctx, approved)
	if err != nil {
		return nil, err
	}
	return guardEngine(a), nil
}

// Same production transport; only the existing private Unix fixture supplies
// a test-owned endpoint. This is not an exported arbitrary socket constructor.
func openGuardedEndpoint(ctx context.Context, endpoint socketEndpoint) (*GuardedEngine, error) {
	a, err := openEndpoint(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	return guardEngine(a), nil
}

func guardEngine(a *Engine) *GuardedEngine {
	dial := a.transport.DialContext
	a.transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		mutation, mutating := ctx.Value(mutationContextKey{}).(mutationContext)
		if mutating && actionbudget.CheckMutation(mutation.context, mutation.target, mutation.operation) != nil {
			return nil, ErrUnavailable
		}
		conn, err := dial(ctx, network, address)
		if err != nil || !mutating {
			return conn, err
		}
		return &guardedConnection{Conn: conn, context: mutation.context, target: mutation.target, operation: mutation.operation}, nil
	}
	return &GuardedEngine{engine: a}
}

type guardedConnection struct {
	net.Conn
	context   context.Context
	target    string
	operation actioncore.Operation
}

// A pre-write check cannot make the syscall atomic with a deadline or prevent
// later daemon processing. A failed post-write check is an uncertain outcome,
// never proof that the daemon did not receive the operation.
func (c *guardedConnection) Write(b []byte) (int, error) {
	if actionbudget.CheckMutation(c.context, c.target, c.operation) != nil {
		return 0, ErrUnavailable
	}
	n, err := c.Conn.Write(b)
	if actionbudget.CheckMutation(c.context, c.target, c.operation) != nil {
		return n, ErrUnavailable
	}
	return n, err
}
