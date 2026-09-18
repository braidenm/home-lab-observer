//go:build linux

package connectedenroll

import (
	"context"
	"io"

	"github.com/braidenm/home-lab-observer/internal/ledgerwitness"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

func validateExistingLedger(ctx context.Context, in io.Reader, out io.Writer) int {
	return validateExistingLedgerWith(ctx, in, out, principal, ledgerwitness.Inspect)
}

// Private seams test failure boundaries without native identities or /state writes.
func validateExistingLedgerWith(ctx context.Context, in io.Reader, out io.Writer, identity func(uint32, uint32, uint32) bool, inspect func(context.Context, uploadstate.Binding) ([32]byte, error)) int {
	if ctx == nil || ctx.Err() != nil || in == nil || out == nil {
		return 22
	}
	var expected ValidationInput
	if input(in, &expected) != nil || ctx.Err() != nil || !identity(expected.UploaderUID, expected.UploaderGID, expected.SharedGID) {
		return 22
	}
	binding := uploadstate.Binding{ServerID: expected.ServerID, ConnectorID: expected.ConnectorID}
	if uploadstate.ValidateRecord(uploadstate.Record{Binding: binding}, binding) != nil {
		return 22
	}
	if ctx.Err() != nil {
		return 22
	}
	fingerprint, err := inspect(ctx, binding)
	if err != nil || ctx.Err() != nil {
		return 22
	}
	result, err := ledgerwitness.EncodeResult(binding, fingerprint)
	defer clear(result)
	if err != nil || ctx.Err() != nil {
		return 22
	}
	n, err := out.Write(result)
	if err != nil || n != len(result) || ctx.Err() != nil {
		return 22
	}
	return 0
}
