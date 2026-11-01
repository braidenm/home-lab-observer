//go:build !linux

package actionengine

import (
	"context"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

type Engine struct{}

func Open(context.Context, SocketWitness) (*Engine, error) { return nil, ErrUnsupported }
func (*Engine) Inspect(context.Context, string) (actioncore.State, error) {
	return actioncore.UnknownState, ErrUnsupported
}
func (*Engine) Start(context.Context, string) error          { return ErrUnsupported }
func (*Engine) Stop(context.Context, string, uint8) error    { return ErrUnsupported }
func (*Engine) Restart(context.Context, string, uint8) error { return ErrUnsupported }
func (*Engine) Close() error                                 { return nil }
