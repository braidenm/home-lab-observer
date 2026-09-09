package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
)

func TestRunCollectOnce(t *testing.T) {
	var out, errOut strings.Builder
	code := run([]string{"collect-once", "--processes=false"}, &out, &errOut, func(context.Context) observation.Snapshot {
		return observation.Snapshot{ObservedAt: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC), Quality: observation.Quality{State: observation.Failed}}
	})
	if code != 0 || !strings.Contains(out.String(), `"schema_version":"observer-current-snapshot/v1"`) {
		t.Fatalf("code=%d out=%s err=%s", code, out.String(), errOut.String())
	}
}

func TestRunRejectsInvalidCommand(t *testing.T) {
	var out, errOut strings.Builder
	if code := run([]string{"serve"}, &out, &errOut, nil); code != 2 {
		t.Fatalf("code=%d", code)
	}
}
