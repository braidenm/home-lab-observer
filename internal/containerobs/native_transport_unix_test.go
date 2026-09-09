//go:build !windows

package containerobs

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestUnixSocketTransport(t *testing.T) {
	exerciseNativeTransport(t, func(t *testing.T) (string, net.Listener) {
		t.Helper()
		directory, err := os.MkdirTemp("", "hlo-")
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, "s")
		t.Cleanup(func() { _ = os.Remove(path); _ = os.Remove(directory) })
		listener, err := net.Listen("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		return "unix://" + path, listener
	})
}
