//go:build linux

package ledgerwitness

import (
	"context"

	"github.com/braidenm/home-lab-observer/internal/uploadledger"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

// Inspect opens only the fixed installed ledger. It does not create missing state.
// Inspection runs as the dedicated uploader; its root coordinator must already
// hold the installation lease and have joined workers before invoking it.
func Inspect(ctx context.Context, binding uploadstate.Binding) ([32]byte, error) {
	return inspect(ctx, "/state/ledger", binding)
}

// The private path seam permits isolated D1 fixtures, not configurable production I/O.
func inspect(ctx context.Context, path string, binding uploadstate.Binding) ([32]byte, error) {
	if ctx == nil || ctx.Err() != nil {
		return [32]byte{}, ErrUnsafe
	}
	ledger, err := uploadledger.OpenExisting(ctx, path, binding)
	if err != nil {
		return [32]byte{}, ErrUnsafe
	}
	return inspectOpened(ctx, ledger, binding)
}

// Deliberately excludes Commit: inspection has no logical write capability.
type readLedger interface {
	Load(context.Context) (uploadstate.Record, error)
	Close() error
}

func inspectOpened(ctx context.Context, ledger readLedger, binding uploadstate.Binding) ([32]byte, error) {
	record, loadErr := ledger.Load(ctx)
	closeErr := ledger.Close()
	defer func() {
		if record.Pending != nil {
			clear(record.Pending.Body)
		}
	}()
	if loadErr != nil || closeErr != nil || ctx.Err() != nil {
		return [32]byte{}, ErrUnsafe
	}
	witness, err := Fingerprint(record, binding)
	if err != nil || ctx.Err() != nil {
		return [32]byte{}, ErrUnsafe
	}
	return witness, nil
}
