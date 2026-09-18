//go:build linux

package connectedprocess

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestSelfAuditIncludesOwnTemporaryDescriptors(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	identity, err := Audit(ctx, os.Getpid(), executable)
	if err != nil || identity.PID != os.Getpid() || identity.StartTimeTicks == 0 {
		t.Fatal("same-process audit refused", err)
	}
}
