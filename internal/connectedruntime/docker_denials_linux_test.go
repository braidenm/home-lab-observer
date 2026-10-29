//go:build linux

package connectedruntime

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestDockerCollectorCannotReadEitherRootActivationChannel(t *testing.T) {
	paths := []string{"/run/docker.sock", "/var/run/docker.sock", "/engine/docker.sock", "/var/lib/home-lab-observer-connected-rich/reader-root", "/var/lib/home-lab-observer-connected-rich/reader-status", "/run/home-lab-observer-connected-rich/activation", "/run/home-lab-observer-connected-rich/reader-activation"}
	for _, denial := range []error{os.ErrPermission, os.ErrNotExist} {
		seen := map[string]bool{}
		if checkDockerCollectorSecrets(func(path string) (io.Closer, error) { seen[path] = true; return nil, denial }) != nil || len(seen) != len(paths) {
			t.Fatal("fixed Docker private paths not all checked")
		}
		for _, path := range paths {
			if !seen[path] {
				t.Fatal("fixed activation/socket path omitted")
			}
		}
	}
	for _, exposed := range paths {
		if checkDockerCollectorSecrets(func(path string) (io.Closer, error) {
			if path == exposed {
				return io.NopCloser(strings.NewReader("")), nil
			}
			return nil, os.ErrPermission
		}) != ErrUnsafe {
			t.Fatal("readable Docker authority accepted")
		}
	}
	if checkDockerCollectorSecrets(func(string) (io.Closer, error) { return nil, errors.New("synthetic-private") }) != ErrUnsafe {
		t.Fatal("unknown access error treated as denial")
	}
}
