package logprocess

import (
	"errors"
	"os"
	"runtime"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
	"github.com/braidenm/home-lab-observer/internal/logprotocol"
)

var (
	ErrHelperUnavailable = errors.New("LOG_HELPER_UNAVAILABLE")
	ErrHelperMismatch    = errors.New("LOG_HELPER_MISMATCH")
)

const maxHelperBytes = 200 << 20 // Same bounded binary cap as release packaging.
const maxIdentityPathBytes = 32768
const maxIdentityDepth = 128

func resolveHelper(build logprotocol.Build, linuxSHA256 string) (commandSpec, error) {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		return commandSpec{}, ErrHelperUnavailable
	}
	if build.OS != runtime.GOOS || build.Arch != runtime.GOARCH {
		return commandSpec{}, ErrHelperMismatch
	}
	// Reuse the closed build grammar without exposing another public validator.
	if _, err := logprotocol.EncodeRequest(build, logobs.ReadRequest{Source: logobs.SourceSystem, QueryStartedAt: time.Unix(1, 0).UTC()}); err != nil {
		return commandSpec{}, ErrHelperMismatch
	}
	path, err := os.Executable()
	if err != nil {
		return commandSpec{}, ErrHelperUnavailable
	}
	return resolvePlatformHelper(path, linuxSHA256)
}

func helperEnvironment() []string { return []string{"LANG=C", "LC_ALL=C", "TZ=UTC"} }
