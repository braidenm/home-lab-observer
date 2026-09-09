//go:build !windows

package containerobs

import (
	"net"
	"path/filepath"
	"testing"
)

func TestUnixSocketTransport(t *testing.T) {
	exerciseNativeTransport(t, func(t *testing.T) (string, net.Listener) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "docker.sock")
		listener, err := net.Listen("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		return "unix://" + filepath.ToSlash(path), listener
	})
}
