package connectedprofile

import (
	"strings"
	"testing"
)

func TestDockerReaderEnvironmentIsFixedAndDoesNotWidenOldWorkers(t *testing.T) {
	valid := []string{"GODEBUG=netdns=go", "GOMAXPROCS=2", "GOMEMLIMIT=64MiB", "INVOCATION_ID=synthetic"}
	if CheckRichProcessesDockerReaderEnvironment(valid) != nil || CheckEnvironment(valid) != ErrUnsafe {
		t.Fatal("reader resource controls changed old environment contract")
	}
	for _, bad := range [][]string{
		valid[:2], valid[1:], append(append([]string{}, valid...), "GOMAXPROCS=2"), append(append([]string{}, valid...), "GOMEMLIMIT=64MiB"),
		{"GODEBUG=netdns=go", "GOMAXPROCS=3", "GOMEMLIMIT=64MiB"}, {"GODEBUG=netdns=go", "GOMAXPROCS=2", "GOMEMLIMIT=128MiB"},
		append(append([]string{}, valid...), "HTTP_PROXY=https://example.invalid"), append(append([]string{}, valid...), "malformed"), append(append([]string{}, valid...), "HOME="+strings.Repeat("x", 4096)), append(append([]string{}, valid...), "GODEBUG=netdns=go"),
	} {
		if CheckRichProcessesDockerReaderEnvironment(bad) != ErrUnsafe {
			t.Fatal("unsafe reader environment admitted")
		}
	}
	tooMany := append([]string{}, valid...)
	for len(tooMany) < 33 {
		tooMany = append(tooMany, "HOME=synthetic")
	}
	if CheckRichProcessesDockerReaderEnvironment(tooMany) != ErrUnsafe {
		t.Fatal("environment entry bound widened")
	}
}
