//go:build linux

package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedinstall"
)

func TestLifecycleDiagnosticsAndExit(t *testing.T) {
	for _, operation := range []string{"start", "stop", "status", "restart", "enable", "other.service"} {
		t.Run(operation, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			var output, diagnostic bytes.Buffer
			if runLifecycle(ctx, operation, &output, &diagnostic) != 22 || output.Len() != 0 || diagnostic.Len() == 0 {
				t.Fatal("refusal lost exit/diagnostic boundary")
			}
		})
	}
	var diagnostic bytes.Buffer
	if lifecycleFailure(fmt.Errorf("private detail: %w", connectedinstall.ErrRecovery), &diagnostic) != 22 || !strings.HasPrefix(diagnostic.String(), "RECOVERY_REQUIRED:") || strings.Contains(diagnostic.String(), "private detail") {
		t.Fatal("recovery diagnostic leaked or lost classification")
	}
}
