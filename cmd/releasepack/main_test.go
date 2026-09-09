package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunHelpAndRequiredFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--help"}, &stdout, &stderr); code != 0 || !strings.Contains(stderr.String(), "version string") {
		t.Fatalf("help code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(nil, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "are required") {
		t.Fatalf("missing flags code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
