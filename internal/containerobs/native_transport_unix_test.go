//go:build !windows

package containerobs

import (
	"net"
	"os"
	"testing"
)

func TestUnixSocketTransport(t *testing.T) {
	exerciseNativeTransport(t, func(t *testing.T) (string, net.Listener) {
		t.Helper()
		placeholder, err := os.CreateTemp("", "hlo-*.sock")
		if err != nil {
			t.Fatal(err)
		}
		path := placeholder.Name()
		if err := placeholder.Close(); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		listener, err := net.Listen("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(path) })
		return "unix://" + path, listener
	})
}
