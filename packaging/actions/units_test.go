package actions

import (
	"net/netip"
	"os"
	"strings"
	"testing"
)

func TestRenderUnitsUsesOnlyBoundedHostAddresses(t *testing.T) {
	digest := strings.Repeat("a", 64)
	host := netip.MustParseAddr("192.0.2.10")
	worker, broker, err := RenderUnits(digest, 998, []netip.Addr{host, netip.MustParseAddr("2001:db8::2")})
	if err != nil || !strings.Contains(worker, "IPAddressAllow=192.0.2.10/32 2001:db8::2/128\n") ||
		!strings.Contains(broker, "SupplementaryGroups=home-lab-observer-action-ipc 998\n") ||
		strings.Contains(worker+broker, "@MANIFEST_SHA256@") || strings.Contains(worker+broker, "@WORKER_EGRESS@") ||
		strings.Contains(worker+broker, "@ENGINE_GID@") {
		t.Fatal("invalid rendered units")
	}
	for _, hosts := range [][]netip.Addr{nil, {host, host}, {netip.IPv4Unspecified()}, {netip.MustParseAddr("fe80::1%eth0")}} {
		if _, _, err := RenderUnits(digest, 998, hosts); err == nil {
			t.Fatal("accepted invalid egress")
		}
	}
}

func TestActionUnitsSeparateAuthority(t *testing.T) {
	for _, role := range []string{"worker", "broker"} {
		t.Run(role, func(t *testing.T) {
			raw, err := os.ReadFile("home-lab-observer-action-" + role + ".service.tmpl")
			if err != nil {
				t.Fatal(err)
			}
			unit := string(raw)
			want := []string{"Type=exec", "Restart=no", "NoNewPrivileges=yes", "PrivateUsers=no",
				"CapabilityBoundingSet=", "AmbientCapabilities=", "LimitCORE=0", "FileDescriptorStoreMax=0",
				"NoExecPaths=/", "ExecPaths=/bin/observer-action-" + role}
			forbidden := []string{"[Install]", "ExecStartPre=", "ExecStartPost=", "ExecReload=", "LoadCredential="}
			if role == "broker" {
				want = append(want, "PrivateNetwork=yes", "RestrictAddressFamilies=AF_UNIX", "TimeoutStopSec=80s")
				forbidden = append(forbidden, "worker.json", "/var/lib/home-lab-observer-actions/worker", "AF_INET")
			} else {
				want = append(want, "IPAddressDeny=any", "IPAddressAllow=@WORKER_EGRESS@", "TimeoutStopSec=10s")
				forbidden = append(forbidden, "docker.sock", "engine.json", "policy.json", "/journal")
			}
			for _, line := range want {
				if !strings.Contains("\n"+unit+"\n", "\n"+line+"\n") {
					t.Errorf("missing %q", line)
				}
			}
			for _, token := range forbidden {
				if strings.Contains(unit, token) {
					t.Errorf("unexpected %q", token)
				}
			}
		})
	}
}
