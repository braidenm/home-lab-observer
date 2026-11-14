package actions

import (
	"os"
	"strings"
	"testing"
)

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
