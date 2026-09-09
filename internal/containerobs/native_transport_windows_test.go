//go:build windows

package containerobs

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
)

func TestNamedPipeTransport(t *testing.T) {
	exerciseNativeTransport(t, func(t *testing.T) (string, net.Listener) {
		t.Helper()
		name := fmt.Sprintf("home-lab-observer-test-%d", time.Now().UnixNano())
		listener, err := winio.ListenPipe(`\\.\pipe\`+name, nil)
		if err != nil {
			t.Fatal(err)
		}
		return "npipe:////./pipe/" + name, listener
	})
}
