//go:build linux

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRichInstallerExposesOnlyFixedLifecycleActions(t *testing.T) {
	for _, operation := range []string{"restart", "enable", "--profile=numeric", "exec", ""} {
		var output, diagnostic bytes.Buffer
		if runLifecycle(context.Background(), operation, &output, &diagnostic) != 22 || output.Len() != 0 || !strings.HasPrefix(diagnostic.String(), "Usage: observer-connected-rich-docker-install") {
			t.Fatal("unsupported operation did not fail before host inspection")
		}
	}
}
