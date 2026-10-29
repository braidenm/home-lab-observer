//go:build linux

package connectedruntime

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRichCollectorSecretViewFailsClosed(t *testing.T) {
	for _, denied := range []error{os.ErrNotExist, os.ErrPermission} {
		calls := 0
		if checkRichCollectorSecrets(func(path string) (io.Closer, error) {
			calls++
			if !strings.Contains(path, "home-lab-observer-connected-rich/") {
				t.Fatal("path escaped fixed rich roots")
			}
			return nil, denied
		}) != nil || calls != 5 {
			t.Fatal("denied fixed secret paths refused")
		}
	}
	for _, failure := range []error{nil, errors.New("synthetic I/O failure")} {
		if checkRichCollectorSecrets(func(string) (io.Closer, error) { return io.NopCloser(strings.NewReader("")), failure }) != ErrUnsafe {
			t.Fatal("secret visibility or unknown failure accepted")
		}
	}
}
