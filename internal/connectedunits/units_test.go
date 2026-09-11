package connectedunits

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func fixture() connectedprofile.Config {
	return connectedprofile.Config{Version: connectedprofile.Version, State: "INSTALLED_READY", ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32), CollectorUID: 61001, UploaderUID: 61002, UploaderGID: 61012, SharedGID: 61011, ArtifactSHA256: strings.Repeat("c", 64), PolicyGeneration: 1, Addresses: []string{"1.1.1.1", "2606:4700:4700::1111"}}
}

func TestReviewedProfiles(t *testing.T) {
	c := fixture()
	u, err := RenderUnits(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{string(u.Collector), string(u.Uploader)} {
		for _, required := range []string{"Type=exec\n", "NoNewPrivileges=yes\n", "CapabilityBoundingSet=\n", "AmbientCapabilities=\n", "PrivateIPC=yes\n", "SystemCallArchitectures=native\n", "@ipc", "socketpair", "io_uring_setup", "io_uring_enter", "io_uring_register", "NoExecPaths=/\n", "MemoryMax=128M\n", "TasksMax=64\n", "TimeoutStopSec=15s\n", "KillMode=control-group\n", "RestartPreventExitStatus=20 21 22\n", "RestartSec=60s\n", "StandardOutput=null\n", "StandardError=null\n"} {
			if !strings.Contains(text, required) {
				t.Fatalf("missing policy %s", required)
			}
		}
		if strings.Contains(text, "{{") || strings.Contains(text, "hlc_") || strings.Contains(text, "hle_") {
			t.Fatal("unexpanded or secret property")
		}
	}
	collector := string(u.Collector)
	for _, required := range []string{"PrivateNetwork=yes\n", "RestrictAddressFamilies=none\n", "ProtectProc=invisible\n", "User=61001\n", "Group=61011\n"} {
		if !strings.Contains(collector, required) {
			t.Fatal("collector policy")
		}
	}
	uploader := string(u.Uploader)
	for _, required := range []string{"RootDirectory=/var/lib/home-lab-observer-connected/uploader-root\n", "MountAPIVFS=no\n", "RestrictAddressFamilies=AF_INET AF_INET6\n", "IPAddressDeny=any\n", "IPAddressAllow=1.1.1.1/32\n", "IPAddressAllow=2606:4700:4700::1111/128\n", "LoadCredential=connector.json:/etc/home-lab-observer-connected/credentials/connector.json\n", "Group=61012\n", "SupplementaryGroups=61011\n"} {
		if !strings.Contains(uploader, required) {
			t.Fatal("uploader policy")
		}
	}
	for _, forbidden := range []string{"ProtectProc=", "ProtectKernelTunables=", "ProtectControlGroups=", "/proc", "/sys", "ExecStartPre=", "/bin/sh"} {
		if strings.Contains(uploader, forbidden) {
			t.Fatal("uploader implicit authority")
		}
	}
	input := EnrollmentInput{c.UploaderUID, c.UploaderGID, c.SharedGID, c.ArtifactSHA256, c.Addresses}
	for _, mode := range []EnrollmentMode{Enroll, ValidateEnrollment} {
		command, err := RenderEnrollmentProperties(input, mode)
		if err != nil {
			t.Fatal(err)
		}
		if command.Executable != "/bin/observer-connected-uploader" || len(command.Args) != 1 || command.Args[0] != string(mode) {
			t.Fatal("command contract")
		}
		text := strings.Join(command.Properties, "\n")
		for _, forbidden := range []string{"LoadCredential=", "/state/ledger", "/handoff", "StandardInputText=", "StandardOutput=journal", "INSTALLED_READY"} {
			if strings.Contains(text, forbidden) {
				t.Fatal("enrollment authority")
			}
		}
		if !strings.Contains(text, "/state/enrollment") || !strings.Contains(text, "RuntimeMaxSec=45s") {
			t.Fatal("enrollment bounds")
		}
	}
	ledger, err := RenderEnrollmentProperties(input, ValidateLedger)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(ledger.Properties, "\n")
	if !strings.Contains(text, "PrivateNetwork=yes") || !strings.Contains(text, "RestrictAddressFamilies=none") || !strings.Contains(text, "/state/ledger") || strings.Contains(text, "/state/enrollment") || strings.Contains(text, "IPAddressAllow=") {
		t.Fatal("ledger validation profile")
	}
}

func TestInvalidInputsAndDetachedResources(t *testing.T) {
	for _, change := range []func(*connectedprofile.Config){func(c *connectedprofile.Config) { c.ArtifactSHA256 = "x\nExecStart=/bin/sh" }, func(c *connectedprofile.Config) { c.UploaderUID = 0 }, func(c *connectedprofile.Config) { c.SharedGID = ^uint32(0) }, func(c *connectedprofile.Config) { c.Addresses = []string{"127.0.0.1"} }, func(c *connectedprofile.Config) { c.Addresses = []string{"1.1.1.1\nIPAddressAllow=any"} }, func(c *connectedprofile.Config) { c.Addresses = append(c.Addresses, c.Addresses[0]) }} {
		c := fixture()
		change(&c)
		if _, err := RenderUnits(c); err != ErrInvalid {
			t.Fatal("invalid profile accepted")
		}
	}
	c := fixture()
	input := EnrollmentInput{c.UploaderUID, c.UploaderGID, c.SharedGID, c.ArtifactSHA256, c.Addresses}
	if _, err := RenderEnrollmentProperties(input, "shell"); err != ErrInvalid {
		t.Fatal("arbitrary mode")
	}
	first := Resources()
	for name, data := range first {
		if len(data) == 0 {
			t.Fatal("empty resource")
		}
		data[0] = '!'
		if Resources()[name][0] == '!' {
			t.Fatal("resource alias")
		}
	}
	var schema map[string]any
	if json.Unmarshal(Resources()["installed-config.schema.json"], &schema) != nil {
		t.Fatal("schema syntax")
	}
}
