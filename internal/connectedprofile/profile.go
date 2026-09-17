// Package connectedprofile defines fixed, non-secret installed worker contracts.
// It imports no collector, network client, ledger, or credential implementation.
package connectedprofile

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"net/netip"
	"regexp"
	"slices"
)

const (
	Version          = "observer-connected-install/v1"
	Origin           = "https://app.braidenmiller.com"
	Hostname         = "app.braidenmiller.com"
	ConfigPath       = "/etc/home-lab-observer-connected/installed.json"
	ConfigDirectory  = "/etc/home-lab-observer-connected"
	StateDirectory   = "/var/lib/home-lab-observer-connected"
	ReleaseDirectory = "/opt/home-lab-observer-connected/releases"
	MaxConfigBytes   = 4096
)

var ErrUnsafe = errors.New("connected_profile_unsafe")
var serverPattern = regexp.MustCompile(`^srv_[a-f0-9]{32}$`)
var connectorPattern = regexp.MustCompile(`^agent_[a-f0-9]{32}$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func ValidServerID(value string) bool    { return serverPattern.MatchString(value) }
func ValidConnectorID(value string) bool { return connectorPattern.MatchString(value) }

type Config struct {
	Version          string   `json:"version"`
	State            string   `json:"state"`
	ServerID         string   `json:"server_id"`
	ConnectorID      string   `json:"connector_id"`
	CollectorUID     uint32   `json:"collector_uid"`
	UploaderUID      uint32   `json:"uploader_uid"`
	UploaderGID      uint32   `json:"uploader_gid"`
	SharedGID        uint32   `json:"shared_gid"`
	ArtifactSHA256   string   `json:"artifact_sha256"`
	PolicyGeneration uint64   `json:"policy_generation"`
	Addresses        []string `json:"addresses"`
}

func (c Config) Validate() error {
	if c.CollectorUID == math.MaxUint32 || c.UploaderUID == math.MaxUint32 || c.UploaderGID == math.MaxUint32 || c.SharedGID == math.MaxUint32 {
		return ErrUnsafe
	}
	if c.Version != Version || c.State != "INSTALLED_READY" || !serverPattern.MatchString(c.ServerID) || !connectorPattern.MatchString(c.ConnectorID) || c.CollectorUID == 0 || c.UploaderUID == 0 || c.UploaderGID == 0 || c.SharedGID == 0 || c.SharedGID == c.UploaderGID || c.CollectorUID == c.UploaderUID || !digestPattern.MatchString(c.ArtifactSHA256) || c.PolicyGeneration == 0 || len(c.Addresses) == 0 || len(c.Addresses) > 8 || !slices.IsSorted(c.Addresses) {
		return ErrUnsafe
	}
	for i, raw := range c.Addresses {
		a, err := netip.ParseAddr(raw)
		if err != nil || a.String() != raw || !PublicAddress(a) || (i > 0 && raw == c.Addresses[i-1]) {
			return ErrUnsafe
		}
	}
	return nil
}

func Decode(data []byte) (Config, error) {
	var c Config
	if len(data) == 0 || len(data) > MaxConfigBytes || json.Unmarshal(data, &c) != nil || c.Validate() != nil {
		return Config{}, ErrUnsafe
	}
	b, _ := json.Marshal(c)
	if !bytes.Equal(b, data) {
		return Config{}, ErrUnsafe
	}
	return c, nil
}

func Encode(c Config) ([]byte, error) {
	if c.Validate() != nil {
		return nil, ErrUnsafe
	}
	return json.Marshal(c)
}

// PublicAddress is deliberately narrower than IsGlobalUnicast: special-use
// ranges are not accepted for the fixed public control plane canary.
func PublicAddress(a netip.Addr) bool {
	if !a.IsValid() || a.Zone() != "" || a.Is4In6() || !a.IsGlobalUnicast() || a.IsPrivate() {
		return false
	}
	for _, raw := range []string{"0.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "::/96", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20"} {
		if netip.MustParsePrefix(raw).Contains(a) {
			return false
		}
	}
	return a.Is4() || netip.MustParsePrefix("2000::/3").Contains(a)
}
