package connectedunits

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func dockerFixture(t *testing.T) connectedprofile.RichProcessesDockerConfig {
	t.Helper()
	p, err := connectedprofile.NewRichProcessesDockerPolicy("srv_"+strings.Repeat("a", 32), "agent_"+strings.Repeat("b", 32), 62001, 62002, 62003, 62004, 62005, 62006, 998)
	if err != nil {
		t.Fatal(err)
	}
	d, err := connectedprofile.DigestRichProcessesDockerPolicy(p)
	if err != nil {
		t.Fatal(err)
	}
	return connectedprofile.RichProcessesDockerConfig{Version: connectedprofile.RichProcessesDockerVersion, State: "INSTALLED_READY", Profile: p.Profile, ServerID: p.ServerID, ConnectorID: p.ConnectorID, CollectorUID: p.CollectorUID, UploaderUID: p.UploaderUID, UploaderGID: p.UploaderGID, SharedGID: p.SharedGID, ReaderUID: p.ReaderUID, FrameGID: p.FrameGID, SocketGID: p.SocketGID, DefinitionSHA256: p.DefinitionSHA256, PolicySHA256: hex.EncodeToString(d[:]), ArtifactSHA256: strings.Repeat("c", 64), PolicyGeneration: 1, Addresses: []string{"1.1.1.1"}}
}
func TestDockerFixedResourceBoundaries(t *testing.T) {
	c := dockerFixture(t)
	u, err := RenderRichProcessesDockerUnits(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range [][]byte{u.Collector, u.Reader, u.Uploader} {
		s := string(b)
		for _, want := range []string{"NoNewPrivileges=yes\n", "CapabilityBoundingSet=\n", "AmbientCapabilities=\n", "KillMode=control-group\n", "Restart=no\n", "TimeoutStopSec=15s\n", "StandardOutput=null\n", "StandardError=null\n"} {
			if !strings.Contains(s, want) {
				t.Fatal("missing fixed worker boundary")
			}
		}
		if strings.Contains(s, "{{") {
			t.Fatal("unexpanded resource")
		}
	}
	r := string(u.Reader)
	for _, want := range []string{"User=62005\n", "Group=62006\n", "SupplementaryGroups=998\n", "RestrictAddressFamilies=AF_UNIX\n", "PrivateNetwork=yes\n", "RootDirectory=" + connectedprofile.RichProcessesDockerReaderRoot + "\n", "MountAPIVFS=no\n", "MemoryMax=128M\n", "TasksMax=32\n", "CPUQuota=25%\n", "GOMAXPROCS=2\n", "GOMEMLIMIT=64MiB\n", "/run/docker.sock:/engine/docker.sock", "/docker-frame:/frame", "/reader-status:/state/status"} {
		if !strings.Contains(r, want) {
			t.Fatal("reader boundary missing")
		}
	}
	if !strings.Contains(r, "BindReadOnlyPaths=/run/home-lab-observer-connected-rich/reader-activation:/activation\n") {
		t.Fatal("reader startup barrier missing")
	}
	for _, forbidden := range []string{"LoadCredential=", "/handoff", "/ledger", "connected-rich/activation:", "AF_INET", "/proc", "/bin/sh", "ProtectProc=", "ProtectKernelTunables=", "ProtectControlGroups="} {
		if strings.Contains(r, forbidden) {
			t.Fatal("reader gained unintended authority")
		}
	}
	collector := string(u.Collector)
	for _, path := range []string{"-/run/home-lab-observer-connected-rich/activation", "-/run/home-lab-observer-connected-rich/reader-activation"} {
		if !strings.Contains(collector, path) {
			t.Fatal("collector can read startup authority")
		}
	}
	if !strings.Contains(collector, "SupplementaryGroups=62006\n") || !strings.Contains(collector, " socket socketpair ") || !strings.Contains(collector, "ReadOnlyPaths="+connectedprofile.RichProcessesDockerConfigPath+" "+connectedprofile.RichProcessesDockerPolicyPath+" "+connectedprofile.RichProcessesDockerFrameDirectory) {
		t.Fatal("collector frame read boundary")
	}
	up := string(u.Uploader)
	for _, forbidden := range []string{"/docker-frame", "/engine/", "docker.sock", "SupplementaryGroups=998", "SupplementaryGroups=62006"} {
		if strings.Contains(up, forbidden) {
			t.Fatal("uploader gained Docker authority")
		}
	}
	if !strings.Contains(up, "SupplementaryGroups=62004\n") || !strings.Contains(up, "IPAddressAllow=1.1.1.1/32\n") {
		t.Fatal("uploader boundary missing")
	}
	resources := RichProcessesDockerResources()
	if len(resources) != 6 {
		t.Fatal("resource allowlist changed")
	}
	for name, b := range resources {
		if strings.HasSuffix(name, ".json") {
			var schema map[string]any
			if json.Unmarshal(b, &schema) != nil {
				t.Fatal("schema syntax")
			}
			properties := schema["properties"].(map[string]any)
			if properties["collection_definition_sha256"].(map[string]any)["const"] != connectedprofile.RichProcessesDockerDefinitionSHA256 {
				t.Fatal("schema definition drift")
			}
		}
		b[0] = '!'
		if RichProcessesDockerResources()[name][0] == '!' {
			t.Fatal("resource alias")
		}
	}
}
func TestDockerEnrollmentHasNoReaderAuthority(t *testing.T) {
	c := dockerFixture(t)
	in := RichProcessesDockerEnrollmentInput{c.UploaderUID, c.UploaderGID, c.SharedGID, c.ArtifactSHA256, c.Addresses}
	for _, mode := range []EnrollmentMode{Enroll, ValidateEnrollment, ValidateLedger} {
		cmd, err := RenderRichProcessesDockerEnrollmentProperties(in, mode)
		if err != nil {
			t.Fatal(err)
		}
		s := strings.Join(cmd.Properties, "\n")
		if cmd.Executable != "/bin/observer-connected-rich-docker-uploader" || len(cmd.Args) != 1 || cmd.Args[0] != string(mode) {
			t.Fatal("enrollment fixed executable")
		}
		for _, forbidden := range []string{"docker.sock", "/frame", "LoadCredential=", "/activation", "reader-root"} {
			if strings.Contains(s, forbidden) {
				t.Fatal("enrollment gained reader authority")
			}
		}
		if mode != Enroll && (strings.Contains(s, "IPAddressAllow=") || !strings.Contains(s, " socket socketpair ")) {
			t.Fatal("offline validation gained network")
		}
	}
	if _, err := RenderRichProcessesDockerEnrollmentProperties(in, ValidateExistingLedger); err != ErrInvalid {
		t.Fatal("unreviewed refresh admitted")
	}
	c.SocketGID = c.SharedGID
	if _, err := RenderRichProcessesDockerUnits(c); err != ErrInvalid {
		t.Fatal("invalid socket group rendered")
	}
}

func TestDockerReaderExplicitlyBindsOnlyItsInertSysScaffold(t *testing.T) {
	u, err := RenderRichProcessesDockerUnits(dockerFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	reader := string(u.Reader)
	for _, boundary := range []string{
		"PrivateNetwork=yes\n", "RestrictAddressFamilies=AF_UNIX\n", "MountAPIVFS=no\n",
		"NoNewPrivileges=yes\n", "CapabilityBoundingSet=\n", "AmbientCapabilities=\n",
		"NoExecPaths=/\n", "ExecPaths=/bin/observer-connected-rich-docker-reader\n",
		"MemoryMax=128M\n", "TasksMax=32\n", "CPUQuota=25%\n",
		"Environment=GOMAXPROCS=2\n", "Environment=GOMEMLIMIT=64MiB\n",
		"TimeoutStopSec=15s\n", "KillMode=control-group\n", "Restart=no\n",
	} {
		if !strings.Contains(reader, boundary) {
			t.Fatal("reader lost a fixed isolation or resource boundary")
		}
	}
	want := "BindReadOnlyPaths=" + connectedprofile.RichProcessesDockerReaderRoot + "/sys:/sys:norbind"
	bindings := 0
	for line := range strings.SplitSeq(reader, "\n") {
		if (strings.HasPrefix(line, "BindReadOnlyPaths=") || strings.HasPrefix(line, "BindPaths=")) && strings.Contains(line, ":/sys") {
			if line != want {
				t.Fatal("reader sys binding is not its mandatory read-only inert scaffold")
			}
			bindings++
		}
	}
	if bindings != 1 {
		t.Fatal("reader requires exactly one explicit read-only inert sys binding")
	}
}
