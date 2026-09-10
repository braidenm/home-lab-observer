//go:build !linux

package journalruntime

import "testing"

func TestHardenUnavailableOutsideLinux(t *testing.T) {
	if Harden() != ErrUnavailable {
		t.Fatal("non-Linux hardening must be unavailable")
	}
}
