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
		items := []observation.Filesystem{}
		return observation.Snapshot{ObservedAt: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC), Filesystems: observation.Section[[]observation.Filesystem]{State: observation.Available, Data: &items}}
	})
	if code != 0 || !strings.Contains(out.String(), `"schema_version":"observer-current-snapshot/v1"`) {
		t.Fatalf("code=%d out=%s err=%s", code, out.String(), errOut.String())
	}
}

func TestRunHelpIsSuccessful(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"help"}, {"collect-once", "--help"}} {
		var out, errOut strings.Builder
		if code := run(args, &out, &errOut, nil); code != 0 || !strings.Contains(out.String(), "usage: observer collect-once") {
			t.Fatalf("args=%v code=%d out=%q err=%q", args, code, out.String(), errOut.String())
		}
	}
}

func TestRunFailedSnapshotIsEmittedWithFailureExit(t *testing.T) {
	var out, errOut strings.Builder
	code := run([]string{"collect-once"}, &out, &errOut, func(context.Context) observation.Snapshot {
		return observation.Snapshot{ObservedAt: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)}
	})
	if code != 1 || !strings.Contains(out.String(), `"collection_state":"FAILED"`) {
		t.Fatalf("code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
}

func TestRunRejectsInvalidCommand(t *testing.T) {
	var out, errOut strings.Builder
	if code := run([]string{"unknown"}, &out, &errOut, nil); code != 2 {
		t.Fatalf("code=%d", code)
	}
}

func TestServeHelpAndUnsafeBind(t *testing.T) {
	for _, test := range []struct {
		args []string
		want int
	}{
		{[]string{"serve", "--help"}, 0},
		{[]string{"serve", "--listen", "0.0.0.0:9847"}, 2},
		{[]string{"serve", "--listen", "localhost:9847"}, 2},
		{[]string{"serve", "--unknown"}, 2},
		{[]string{"serve", "--docker-endpoint", "tcp://127.0.0.1:2375"}, 2},
		{[]string{"serve", "--docker-endpoint", "ssh://owner@example.invalid"}, 2},
	} {
		var out, errOut strings.Builder
		if got := run(test.args, &out, &errOut, nil); got != test.want {
			t.Fatalf("args=%v exit=%d want=%d", test.args, got, test.want)
		}
	}
}
