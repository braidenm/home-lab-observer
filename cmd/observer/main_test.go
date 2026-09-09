package main

import (
	"context"
	"strings"
	"testing"
)

func TestRunCollectOnce(t *testing.T) {
	var out, errOut strings.Builder
	code := run([]string{"collect-once", "--processes=false"}, &out, &errOut, func(context.Context) any { return map[string]string{"schema_version": "test/v1"} })
	if code != 0 || !strings.Contains(out.String(), `"schema_version":"test/v1"`) {
		t.Fatalf("code=%d out=%s err=%s", code, out.String(), errOut.String())
	}
}

func TestRunRejectsInvalidCommand(t *testing.T) {
	var out, errOut strings.Builder
	if code := run([]string{"serve"}, &out, &errOut, nil); code != 2 {
		t.Fatalf("code=%d", code)
	}
}
