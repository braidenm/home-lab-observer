//go:build linux

package connectedinstall

import (
	"strings"
	"testing"
)

func TestActionNetworkUsesSameAddressesForHostsAndEgress(t *testing.T) {
	digest := strings.Repeat("a", 64)
	result, err := renderActionNetwork(digest, 998, []string{"1.1.1.1", "2606:4700:4700::1111"})
	if err != nil || strings.Count(result.hosts, "\n") != 2 ||
		!strings.Contains(result.workerUnit, "IPAddressAllow=1.1.1.1/32 2606:4700:4700::1111/128\n") ||
		!strings.Contains(result.workerUnit, "/worker-hosts:/etc/hosts\n") ||
		strings.Contains(result.brokerUnit, "/worker-hosts") || strings.Contains(result.brokerUnit, "1.1.1.1") {
		t.Fatal("unexpected action network resources")
	}
	for _, addresses := range [][]string{nil, {"1.1.1.1\nIPAddressAllow=any"}, {"127.0.0.1"}, {"1.1.1.1", "1.1.1.1"}} {
		if _, err := renderActionNetwork(digest, 998, addresses); err == nil {
			t.Fatal("unsafe addresses accepted")
		}
	}
}
