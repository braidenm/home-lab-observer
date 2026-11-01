//go:build !linux

package actionjournal

import (
	"context"
	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

type Journal struct{}

func Provision(context.Context, string, Binding) (*Journal, error)    { return nil, ErrUnsupported }
func OpenExisting(context.Context, string, Binding) (*Journal, error) { return nil, ErrUnsupported }
func (*Journal) Acquire(context.Context, actioncore.Key) (actioncore.Attempt, error) {
	return nil, ErrUnsupported
}
func (*Journal) Prune(context.Context) error { return ErrUnsupported }
func (*Journal) Close() error                { return nil }
