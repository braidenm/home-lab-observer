package actionengine

import (
	"context"

	"github.com/braidenm/home-lab-observer/internal/actionbudget"
	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

// GuardedEngine is the paired broker's timing-enforced adapter. Timing tokens
// supply neither remote authority nor local approval; the broker checks both.
// The underlying legacy adapter is intentionally not exposed by this type.
type GuardedEngine struct{ engine *Engine }

var _ actioncore.Engine = (*GuardedEngine)(nil)

type mutationContextKey struct{}
type mutationContext struct {
	context   context.Context
	target    string
	operation actioncore.Operation
}

func (g *GuardedEngine) Inspect(ctx context.Context, id string) (actioncore.State, error) {
	if g == nil || g.engine == nil {
		return actioncore.UnknownState, ErrUnavailable
	}
	return g.engine.Inspect(ctx, id)
}

func (g *GuardedEngine) Start(ctx context.Context, id string) error {
	return g.mutate(ctx, id, actioncore.Start, 0)
}

func (g *GuardedEngine) Stop(ctx context.Context, id string, grace uint8) error {
	return g.mutate(ctx, id, actioncore.Stop, grace)
}

func (g *GuardedEngine) Restart(ctx context.Context, id string, grace uint8) error {
	return g.mutate(ctx, id, actioncore.Restart, grace)
}

func (g *GuardedEngine) mutate(ctx context.Context, id string, operation actioncore.Operation, grace uint8) error {
	if g == nil || g.engine == nil || actionbudget.CheckMutation(ctx, id, operation) != nil {
		return ErrUnavailable
	}
	// Retain the original operation context: net/http may detach the dial's
	// cancellation while acquiring a connection, but must not detach this check.
	bound := context.WithValue(ctx, mutationContextKey{}, mutationContext{ctx, id, operation})
	var err error
	switch operation {
	case actioncore.Start:
		err = g.engine.Start(bound, id)
	case actioncore.Stop:
		err = g.engine.Stop(bound, id, grace)
	case actioncore.Restart:
		err = g.engine.Restart(bound, id, grace)
	default:
		return ErrInvalid
	}
	if err != nil || actionbudget.CheckMutation(ctx, id, operation) != nil {
		return ErrUnavailable
	}
	return nil
}

func (g *GuardedEngine) Close() error {
	if g == nil || g.engine == nil {
		return nil
	}
	return g.engine.Close()
}
