package actions

import (
	"net/netip"
	"strings"
	"testing"
)

func TestBootstrapPropertiesRestrictAllThreeOperations(t *testing.T) {
	digest := strings.Repeat("a", 64)
	addresses := []netip.Addr{netip.MustParseAddr("192.0.2.10")}
	for _, verb := range []string{"seed-enrollment", "exchange", "verify-enrollment", "provision-journal"} {
		needsHTTPS := verb == "exchange" || verb == "verify-enrollment"
		props, err := BootstrapProperties(digest, verb, 998, addresses)
		if err != nil {
			t.Fatal(err)
		}
		raw := strings.Join(props, "\n")
		for _, unexpected := range []string{"docker.sock", "/run/home-lab-observer-actions/worker", "ExecStart=", "StandardInput="} {
			if strings.Contains(raw, unexpected) {
				t.Fatalf("%s has unexpected %s", verb, unexpected)
			}
		}
		if !strings.Contains(raw, "ExecPaths=/bin/observer-action-setup\n") ||
			!strings.Contains(raw, "RuntimeMaxSec=30s") ||
			strings.Contains(raw, "PrivateNetwork=yes") != !needsHTTPS ||
			strings.Contains(raw, "IPAddressDeny=any") != needsHTTPS ||
			strings.Contains(raw, "worker.json") != (verb != "provision-journal") ||
			strings.Contains(raw, "/journal") != (verb == "provision-journal") {
			t.Fatalf("%s role or network policy mismatch", verb)
		}
	}
	if _, err := BootstrapProperties(digest, "start", 998, addresses); err == nil {
		t.Fatal("runtime start is not a bootstrap operation")
	}
}
