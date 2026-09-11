//go:build !linux

package enrollmentstore

import (
	"context"

	"github.com/braidenm/home-lab-observer/internal/enrollmentcoord"
	"github.com/braidenm/home-lab-observer/internal/uploadledger"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

type Setup struct{}
type Ready struct{}

func OpenNew(context.Context, string) (*Setup, error) { return nil, ErrUnsupported }
func OpenReady(context.Context, string, uploadstate.Binding) (*Ready, error) {
	return nil, ErrUnsupported
}
func (*Setup) Begin(context.Context, uploadstate.Binding) error         { return ErrUnsupported }
func (*Setup) Create(context.Context, enrollmentcoord.Credential) error { return ErrUnsupported }
func (*Setup) Provision(context.Context, uploadstate.Binding) error     { return ErrUnsupported }
func (*Setup) MarkReady(context.Context, uploadstate.Binding) error     { return ErrUnsupported }
func (*Setup) Close() error                                             { return nil }
func (*Ready) Credential() (enrollmentcoord.Credential, error) {
	return enrollmentcoord.Credential{}, ErrUnsupported
}
func (*Ready) Ledger() *uploadledger.Ledger { return nil }
func (*Ready) Close() error                 { return nil }
