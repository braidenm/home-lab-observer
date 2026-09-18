//go:build !linux || (!amd64 && !arm64)

package ledgeridentity

import (
	"context"
	"os"
)

func Inspect(context.Context, *os.File, *os.File) (Witness, error) {
	return Witness{}, ErrUnavailable
}
