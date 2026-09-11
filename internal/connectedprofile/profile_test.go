package connectedprofile

import (
	"bytes"
	"net/netip"
	"strings"
	"testing"
)

func testConfig() Config {
	return Config{Version: Version, State: "INSTALLED_READY", ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32), CollectorUID: 60101, UploaderUID: 60102, UploaderGID: 60102, SharedGID: 60103, ArtifactSHA256: strings.Repeat("c", 64), PolicyGeneration: 1, Addresses: []string{"93.184.216.34"}}
}
func TestClosedCanonicalInstalledAuthority(t *testing.T) {
	c := testConfig()
	b, err := Encode(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(b); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{nil, append(bytes.Clone(b), '\n'), bytes.Replace(b, []byte(`"state":`), []byte(`"unexpected":true,"state":`), 1), bytes.Replace(b, []byte(`"state":`), []byte(`"state":"BAD","state":`), 1), bytes.Repeat([]byte{' '}, MaxConfigBytes+1)} {
		if _, err := Decode(bad); err != ErrUnsafe {
			t.Fatal("noncanonical metadata accepted")
		}
	}
	for _, change := range []func(*Config){func(c *Config) { c.CollectorUID = 0 }, func(c *Config) { c.UploaderUID = c.CollectorUID }, func(c *Config) { c.UploaderGID = c.SharedGID }, func(c *Config) { c.State = "PREPARING" }, func(c *Config) { c.PolicyGeneration = 0 }, func(c *Config) { c.Addresses = []string{"127.0.0.1"} }, func(c *Config) { c.Addresses = []string{"93.184.216.34", "93.184.216.34"} }, func(c *Config) { c.Addresses = nil }} {
		c := testConfig()
		change(&c)
		if c.Validate() != ErrUnsafe {
			t.Fatal("invalid authority accepted")
		}
	}
}
func TestPublicEndpointBoundary(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.1.2.3", "192.168.1.2", "100.64.0.1", "169.254.169.254", "192.0.2.1", "198.18.0.1", "203.0.113.1", "240.1.1.1", "224.0.0.1", "::1", "::ffff:93.184.216.34", "fe80::1", "fc00::1", "2001:db8::1", "2002::1", "64:ff9b::1", "3fff::1"} {
		if PublicAddress(netip.MustParseAddr(raw)) {
			t.Fatalf("special destination accepted: %s", raw)
		}
	}
	for _, raw := range []string{"93.184.216.34", "2606:4700::1111"} {
		if !PublicAddress(netip.MustParseAddr(raw)) {
			t.Fatal("public address refused")
		}
	}
}
