//go:build !linux || (!amd64 && !arm64)

package ledgeridentity

import (
	"context"
	"testing"
)

func TestUnsupported(t *testing.T) {
	if w, err := Inspect(context.Background(), nil, nil); err != ErrUnavailable || w != (Witness{}) {
		t.Fatal("unsupported inspection accepted")
	}
}
