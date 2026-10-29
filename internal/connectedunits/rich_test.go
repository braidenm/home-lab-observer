package connectedunits

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func TestRichCollectorMasksBothProfilesPrivateState(t *testing.T) {
	u, err := RenderRichProcessesUnits(richFixture())
	if err != nil {
		t.Fatal(err)
	}
	var actual []string
	properties := 0
	for _, line := range strings.Split(string(u.Collector), "\n") {
		if paths, ok := strings.CutPrefix(line, "InaccessiblePaths="); ok {
			properties++
			actual = strings.Fields(paths)
		}
	}
	var expected []string
	for _, root := range []string{"home-lab-observer-connected", "home-lab-observer-connected-rich"} {
		expected = append(expected, "-/etc/"+root+"/credentials")
		for _, name := range []string{"ledger", "uploader-root", "enrollment", "uploader-status"} {
			expected = append(expected, "-/var/lib/"+root+"/"+name)
		}
	}
	slices.Sort(actual)
	slices.Sort(expected)
	if properties != 1 || !slices.Equal(actual, expected) {
		t.Fatal("rich collector does not mask both profiles' private state")
	}
}

func richFixture() connectedprofile.RichProcessesConfig {
	policy := connectedprofile.RichProcessesPolicy{Version: "observer-connected-rich-policy/v1", Profile: connectedprofile.RichProcessesProfile, ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32), CollectorUID: 62001, UploaderUID: 62002, UploaderGID: 62003, SharedGID: 62004, DefinitionSHA256: connectedprofile.RichProcessesDefinitionSHA256, ProcessesEnabled: true}
	digest, _ := connectedprofile.DigestRichProcessesPolicy(policy)
	return connectedprofile.RichProcessesConfig{Version: connectedprofile.RichProcessesVersion, State: "INSTALLED_READY", Profile: connectedprofile.RichProcessesProfile, ServerID: policy.ServerID, ConnectorID: policy.ConnectorID, CollectorUID: policy.CollectorUID, UploaderUID: policy.UploaderUID, UploaderGID: policy.UploaderGID, SharedGID: policy.SharedGID, DefinitionSHA256: connectedprofile.RichProcessesDefinitionSHA256, PolicySHA256: hex.EncodeToString(digest[:]), ArtifactSHA256: strings.Repeat("c", 64), PolicyGeneration: 1, Addresses: []string{"1.1.1.1"}}
}

func TestRichProcessesUnitsAreSeparateAndClosed(t *testing.T) {
	c := richFixture()
	u, err := RenderRichProcessesUnits(c)
	if err != nil {
		t.Fatal(err)
	}
	collector, uploader := string(u.Collector), string(u.Uploader)
	for _, required := range []string{"observer-connected-rich-collector", "ProtectProc=default\n", "ProcSubset=all\n", "PrivateNetwork=yes\n", " socket socketpair ", "ReadOnlyPaths=/etc/home-lab-observer-connected-rich/installed.json /etc/home-lab-observer-connected-rich/collection-policy.json\n", "Restart=no\n"} {
		if !strings.Contains(collector, required) {
			t.Fatalf("collector missing %q", required)
		}
	}
	for _, forbidden := range []string{"observer-connected-collector\n", "ProtectProc=invisible", "docker.sock", "CAP_SYS_PTRACE", "AmbientCapabilities=CAP"} {
		if strings.Contains(collector, forbidden) {
			t.Fatalf("collector contains %q", forbidden)
		}
	}
	for _, required := range []string{"observer-connected-rich-uploader", "RootDirectory=/var/lib/home-lab-observer-connected-rich/uploader-root\n", "collection-policy.json", "LoadCredential=connector.json:/etc/home-lab-observer-connected-rich/credentials/connector.json\n", "IPAddressDeny=any\n", "IPAddressAllow=1.1.1.1/32\n", "Restart=no\n"} {
		if !strings.Contains(uploader, required) {
			t.Fatalf("uploader missing %q", required)
		}
	}
	for _, text := range []string{collector, uploader} {
		if strings.Contains(text, "{{") || strings.Contains(text, "docker.sock") || strings.Contains(text, "home-lab-observer-connected/releases") {
			t.Fatal("rich unit leaked template, Docker, or numeric root")
		}
	}
	if bytes, _ := RenderUnits(fixture()); strings.Contains(string(bytes.Collector), "connected-rich") || strings.Contains(string(bytes.Uploader), "connected-rich") {
		t.Fatal("numeric units changed to rich paths")
	}
}

func TestRichProcessesEnrollmentIsExact(t *testing.T) {
	c := richFixture()
	input := RichProcessesEnrollmentInput{c.UploaderUID, c.UploaderGID, c.SharedGID, c.ArtifactSHA256, c.Addresses}
	for _, mode := range []EnrollmentMode{Enroll, ValidateEnrollment, ValidateLedger} {
		command, err := RenderRichProcessesEnrollmentProperties(input, mode)
		if err != nil {
			t.Fatal(err)
		}
		if command.Executable != "/bin/observer-connected-rich-uploader" || len(command.Args) != 1 || command.Args[0] != string(mode) {
			t.Fatal("rich enrollment command changed")
		}
		text := strings.Join(command.Properties, "\n")
		if strings.Contains(text, "home-lab-observer-connected/") || strings.Contains(text, "observer-connected-uploader") {
			t.Fatal("numeric path admitted in rich enrollment")
		}
		if mode == Enroll {
			if !strings.Contains(text, "PrivateNetwork=no") || !strings.Contains(text, "IPAddressAllow=1.1.1.1/32") {
				t.Fatal("online enrollment policy")
			}
		} else if !strings.Contains(text, "PrivateNetwork=yes") || !strings.Contains(text, " socket socketpair ") || strings.Contains(text, "IPAddressAllow=") {
			t.Fatal("offline enrollment policy")
		}
	}
	if _, err := RenderRichProcessesEnrollmentProperties(input, ValidateExistingLedger); err != ErrInvalid {
		t.Fatal("unsupported rich existing-ledger witness rendered")
	}
}

func TestRichResourcesAreDetachedAndMatchDefinition(t *testing.T) {
	resources := RichProcessesResources()
	if len(resources) != 5 || len(Resources()) != 4 {
		t.Fatalf("resource counts = %d, %d", len(resources), len(Resources()))
	}
	for name, data := range resources {
		if len(data) == 0 {
			t.Fatal("empty rich resource")
		}
		copy := RichProcessesResources()[name]
		data[0] ^= 1
		if copy[0] == data[0] {
			t.Fatal("resource alias")
		}
	}
	for _, name := range []string{"rich-installed-config.schema.json", "rich-collection-policy.schema.json"} {
		var schema map[string]any
		if json.Unmarshal(RichProcessesResources()[name], &schema) != nil || !strings.Contains(string(RichProcessesResources()[name]), connectedprofile.RichProcessesDefinitionSHA256) {
			t.Fatalf("invalid rich schema %s", name)
		}
	}
	definition := sha256.Sum256([]byte(connectedprofile.RichProcessesDefinition))
	if hex.EncodeToString(definition[:]) != connectedprofile.RichProcessesDefinitionSHA256 {
		t.Fatal("definition changed")
	}
}
