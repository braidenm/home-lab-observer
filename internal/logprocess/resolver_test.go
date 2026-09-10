package logprocess

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/logprotocol"
)

func TestResolverRejectsBuildMismatchBeforeFilesystem(t *testing.T) {
	for _, build := range []logprotocol.Build{
		{OS: "wrong", Arch: runtime.GOARCH},
		{OS: runtime.GOOS, Arch: "wrong"},
		{OS: runtime.GOOS, Arch: runtime.GOARCH, Version: "dev", Commit: strings.Repeat("a", 40)},
	} {
		expected := ErrHelperMismatch
		if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
			expected = ErrHelperUnavailable
		}
		if _, err := resolveHelper(build, ""); !errors.Is(err, expected) {
			t.Fatalf("unclosed build: %v", err)
		}
	}
}

func TestHelperEnvironmentNeverInheritsSecrets(t *testing.T) {
	for _, key := range []string{"LD_PRELOAD", "LD_LIBRARY_PATH", "PATH", "SYSTEMD_LOG_TARGET", "HOME", "USERPROFILE", "TOKEN", "HTTP_PROXY"} {
		t.Setenv(key, "synthetic-secret")
	}
	env := helperEnvironment()
	if strings.Join(env, ";") != "LANG=C;LC_ALL=C;TZ=UTC" {
		t.Fatalf("environment widened: %v", env)
	}
	env[0] = "changed"
	if helperEnvironment()[0] != "LANG=C" {
		t.Fatal("aliased environment")
	}
}
