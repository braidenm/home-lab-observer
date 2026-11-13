//go:build linux

package actionbroker

import (
	"context"
	"errors"
	"testing"
)

func TestProvisionJournalRefusesCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ProvisionJournal(ctx); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("expected closed provisioning refusal, got %v", err)
	}
}
