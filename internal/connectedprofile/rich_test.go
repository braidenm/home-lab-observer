package connectedprofile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func richPolicyFixture() RichProcessesPolicy {
	return RichProcessesPolicy{
		Version: "observer-connected-rich-policy/v1", Profile: RichProcessesProfile,
		ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32),
		CollectorUID: 61001, UploaderUID: 61002, UploaderGID: 61003, SharedGID: 61004,
		DefinitionSHA256: RichProcessesDefinitionSHA256, ProcessesEnabled: true,
	}
}

func richConfigFixture(t *testing.T) RichProcessesConfig {
	t.Helper()
	p := richPolicyFixture()
	digest, err := DigestRichProcessesPolicy(p)
	if err != nil {
		t.Fatal(err)
	}
	return RichProcessesConfig{
		Version: RichProcessesVersion, State: "INSTALLED_READY", Profile: RichProcessesProfile,
		ServerID: p.ServerID, ConnectorID: p.ConnectorID, CollectorUID: p.CollectorUID,
		UploaderUID: p.UploaderUID, UploaderGID: p.UploaderGID, SharedGID: p.SharedGID,
		DefinitionSHA256: RichProcessesDefinitionSHA256, PolicySHA256: hex.EncodeToString(digest[:]),
		ArtifactSHA256: strings.Repeat("c", 64), PolicyGeneration: 1, Addresses: []string{"1.1.1.1", "2606:4700:4700::1111"},
	}
}

func TestRichProcessesDefinitionAndPolicyAreExact(t *testing.T) {
	sum := sha256.Sum256([]byte(RichProcessesDefinition))
	if got := hex.EncodeToString(sum[:]); got != RichProcessesDefinitionSHA256 {
		t.Fatalf("definition digest = %s", got)
	}
	for _, required := range []string{`"collection_deadline_seconds":10`, `"network_collection":"disabled"`, `"collector_network":false`} {
		if !strings.Contains(RichProcessesDefinition, required) {
			t.Fatalf("definition missing %s", required)
		}
	}
	p := richPolicyFixture()
	constructed, err := NewRichProcessesPolicy(p.ServerID, p.ConnectorID, p.CollectorUID, p.UploaderUID, p.UploaderGID, p.SharedGID)
	if err != nil || constructed != p {
		t.Fatalf("fixed policy constructor = %#v, %v", constructed, err)
	}
	b, err := EncodeRichProcessesPolicy(p)
	if err != nil || len(b) == 0 || !bytes.Contains(b, []byte(`"containers_enabled":false`)) {
		t.Fatalf("policy = %s, %v", b, err)
	}
	decoded, err := DecodeRichProcessesPolicy(b)
	if err != nil || decoded != p {
		t.Fatalf("decoded = %#v, %v", decoded, err)
	}
	for name, mutate := range map[string]func(*RichProcessesPolicy){
		"profile":    func(v *RichProcessesPolicy) { v.Profile = "RICH_PROCESSES_DOCKER_V1" },
		"definition": func(v *RichProcessesPolicy) { v.DefinitionSHA256 = strings.Repeat("d", 64) },
		"processes":  func(v *RichProcessesPolicy) { v.ProcessesEnabled = false },
		"containers": func(v *RichProcessesPolicy) { v.ContainersEnabled = true },
		"identity":   func(v *RichProcessesPolicy) { v.ConnectorID = "agent_invalid" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := p
			mutate(&bad)
			if _, err := EncodeRichProcessesPolicy(bad); err != ErrUnsafe {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if _, err := DecodeRichProcessesPolicy(append(b, '\n')); err != ErrUnsafe {
		t.Fatalf("noncanonical policy error = %v", err)
	}
}

func TestRichProcessesConfigBindsPolicyButNotOperationalRefresh(t *testing.T) {
	p := richPolicyFixture()
	c := richConfigFixture(t)
	b, err := EncodeRichProcessesConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRichProcessesConfig(b)
	if err != nil || decoded.PolicySHA256 != c.PolicySHA256 || VerifyRichProcessesBinding(decoded, p) != nil {
		t.Fatalf("decoded = %#v, %v", decoded, err)
	}
	refreshed := c
	refreshed.ArtifactSHA256 = strings.Repeat("d", 64)
	refreshed.PolicyGeneration++
	refreshed.Addresses = []string{"8.8.8.8"}
	if VerifyRichProcessesBinding(refreshed, p) != nil {
		t.Fatal("operational refresh changed immutable policy binding")
	}
	for name, mutate := range map[string]func(*RichProcessesConfig){
		"definition": func(v *RichProcessesConfig) { v.DefinitionSHA256 = strings.Repeat("e", 64) },
		"profile":    func(v *RichProcessesConfig) { v.Profile = "RICH_PROCESSES_DOCKER_V1" },
		"addresses":  func(v *RichProcessesConfig) { v.Addresses = []string{"192.0.2.1"} },
		"ids":        func(v *RichProcessesConfig) { v.UploaderUID = v.CollectorUID },
	} {
		t.Run(name, func(t *testing.T) {
			bad := c
			mutate(&bad)
			if _, err := EncodeRichProcessesConfig(bad); err != ErrUnsafe {
				t.Fatalf("error = %v", err)
			}
		})
	}
	wrongDigest := c
	wrongDigest.PolicySHA256 = strings.Repeat("e", 64)
	if _, err := EncodeRichProcessesConfig(wrongDigest); err != nil || VerifyRichProcessesBinding(wrongDigest, p) != ErrUnsafe {
		t.Fatalf("policy digest was not treated as a binding: %v", err)
	}
	wrong := p
	wrong.ServerID = "srv_" + strings.Repeat("f", 32)
	if VerifyRichProcessesBinding(c, wrong) != ErrUnsafe {
		t.Fatal("mismatched policy identity admitted")
	}
}

func TestNumericConfigBytesRemainIndependent(t *testing.T) {
	numeric := Config{Version: Version, State: "INSTALLED_READY", ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32), CollectorUID: 1, UploaderUID: 2, UploaderGID: 3, SharedGID: 4, ArtifactSHA256: strings.Repeat("c", 64), PolicyGeneration: 1, Addresses: []string{"1.1.1.1"}}
	b, err := Encode(numeric)
	if err != nil || bytes.Contains(b, []byte("collection_profile")) || bytes.Contains(b, []byte("collection_policy")) {
		t.Fatalf("numeric config changed: %s, %v", b, err)
	}
}
