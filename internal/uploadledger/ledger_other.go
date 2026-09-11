//go:build !linux

package uploadledger

import (
	"context"

	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

type Ledger struct{}

func Provision(context.Context, string, uploadstate.Binding) (*Ledger, error) {
	return nil, ErrUnsupported
}
func OpenExisting(context.Context, string, uploadstate.Binding) (*Ledger, error) {
	return nil, ErrUnsupported
}
func (*Ledger) Load(context.Context) (uploadstate.Record, error) {
	return uploadstate.Record{}, ErrUnsupported
}
func (*Ledger) Commit(context.Context, uploadstate.Record, uploadstate.Record) error {
	return ErrUnsupported
}
func (*Ledger) Close() error { return nil }
