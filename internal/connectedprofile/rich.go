package connectedprofile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/netip"
	"slices"
)

const (
	RichProcessesVersion          = "observer-connected-rich-install/v1"
	RichProcessesProfile          = "RICH_PROCESSES_V1"
	RichProcessesConfigDirectory  = "/etc/home-lab-observer-connected-rich"
	RichProcessesConfigPath       = RichProcessesConfigDirectory + "/installed.json"
	RichProcessesPolicyPath       = RichProcessesConfigDirectory + "/collection-policy.json"
	RichProcessesStateDirectory   = "/var/lib/home-lab-observer-connected-rich"
	RichProcessesReleaseDirectory = "/opt/home-lab-observer-connected-rich/releases"
	RichProcessesCollectorName    = "hlo-connected-rich-collector"
	RichProcessesUploaderName     = "hlo-connected-rich-uploader"
	RichProcessesSharedName       = "hlo-connected-rich-read"
	RichProcessesMaxPolicyBytes   = 4096

	// RichProcessesDefinition is the immutable collection and sandbox contract.
	// It deliberately excludes artifact identity, resolved network addresses and
	// policy generation so compatible delivery refresh cannot change collection.
	RichProcessesDefinition       = `{"version":"observer-connected-rich-definition/v1","profile":"RICH_PROCESSES_V1","combined_schema":"home-lab-connected-observation/v1","combined_profile":"numeric-host-workloads/v1","host_schema":"home-lab-native-host-metrics/v1","workload_schema":"home-lab-workload-inventory/v1","workload_profile":"process-container/v1","processes_enabled":true,"containers_enabled":false,"collection_deadline_seconds":10,"network_collection":"disabled","max_body_bytes":294912,"ledger_user_version":2,"ledger_profile_kind":1,"ledger_db_bytes":524288,"ledger_page_bytes":4096,"ledger_total_bytes":2097152,"handoff_schema":"observer-connected-rich-handoff/v1","handoff_record_bytes":295424,"handoff_slots":2,"max_process_rows":200,"max_metadata_items":4096,"denied_process_fields":["command_line","environment","open_files","network_endpoints"],"workers":["rich-collector","rich-uploader","rich-install"],"principals":["hlo-connected-rich-collector","hlo-connected-rich-uploader","hlo-connected-rich-read"],"collector_proc_visibility":"default","collector_ptrace":false,"collector_network":false,"collector_credentials":false,"uploader_proc_visibility":"invisible","uploader_network":"allowlisted_tls","uploader_credentials":true,"config_root":"/etc/home-lab-observer-connected-rich","state_root":"/var/lib/home-lab-observer-connected-rich","release_root":"/opt/home-lab-observer-connected-rich/releases","collector_unit":"home-lab-observer-connected-rich-collector.service","uploader_unit":"home-lab-observer-connected-rich-uploader.service","automatic_boot":false,"automatic_restart":false}`
	RichProcessesDefinitionSHA256 = "438442e2675b19fc7f1f181e7eaef80d45f6c87b807710970664affbbc6e1150"
)

type RichProcessesConfig struct {
	Version          string   `json:"version"`
	State            string   `json:"state"`
	Profile          string   `json:"collection_profile"`
	ServerID         string   `json:"server_id"`
	ConnectorID      string   `json:"connector_id"`
	CollectorUID     uint32   `json:"collector_uid"`
	UploaderUID      uint32   `json:"uploader_uid"`
	UploaderGID      uint32   `json:"uploader_gid"`
	SharedGID        uint32   `json:"shared_gid"`
	DefinitionSHA256 string   `json:"collection_definition_sha256"`
	PolicySHA256     string   `json:"collection_policy_sha256"`
	ArtifactSHA256   string   `json:"artifact_sha256"`
	PolicyGeneration uint64   `json:"policy_generation"`
	Addresses        []string `json:"addresses"`
}

type RichProcessesPolicy struct {
	Version           string `json:"version"`
	Profile           string `json:"collection_profile"`
	ServerID          string `json:"server_id"`
	ConnectorID       string `json:"connector_id"`
	CollectorUID      uint32 `json:"collector_uid"`
	UploaderUID       uint32 `json:"uploader_uid"`
	UploaderGID       uint32 `json:"uploader_gid"`
	SharedGID         uint32 `json:"shared_gid"`
	DefinitionSHA256  string `json:"collection_definition_sha256"`
	ProcessesEnabled  bool   `json:"processes_enabled"`
	ContainersEnabled bool   `json:"containers_enabled"`
}

// NewRichProcessesPolicy builds the one code-owned process-only policy after
// enrollment has generated the connector identity. It exposes no profile or
// group selector and therefore cannot widen the installed definition.
func NewRichProcessesPolicy(serverID, connectorID string, collectorUID, uploaderUID, uploaderGID, sharedGID uint32) (RichProcessesPolicy, error) {
	p := RichProcessesPolicy{
		Version: "observer-connected-rich-policy/v1", Profile: RichProcessesProfile,
		ServerID: serverID, ConnectorID: connectorID,
		CollectorUID: collectorUID, UploaderUID: uploaderUID, UploaderGID: uploaderGID, SharedGID: sharedGID,
		DefinitionSHA256: RichProcessesDefinitionSHA256, ProcessesEnabled: true,
	}
	if p.Validate() != nil {
		return RichProcessesPolicy{}, ErrUnsafe
	}
	return p, nil
}

func validRichIDs(collector, uploader, uploaderGID, sharedGID uint32) bool {
	return collector != 0 && uploader != 0 && uploaderGID != 0 && sharedGID != 0 &&
		collector != math.MaxUint32 && uploader != math.MaxUint32 && uploaderGID != math.MaxUint32 && sharedGID != math.MaxUint32 &&
		collector != uploader && uploaderGID != sharedGID
}

func validRichAddresses(addresses []string) bool {
	if len(addresses) == 0 || len(addresses) > 8 || !slices.IsSorted(addresses) {
		return false
	}
	for i, raw := range addresses {
		a, err := netip.ParseAddr(raw)
		if err != nil || a.String() != raw || !PublicAddress(a) || (i > 0 && raw == addresses[i-1]) {
			return false
		}
	}
	return true
}

func (c RichProcessesConfig) Validate() error {
	if c.Version != RichProcessesVersion || c.State != "INSTALLED_READY" || c.Profile != RichProcessesProfile ||
		!ValidServerID(c.ServerID) || !ValidConnectorID(c.ConnectorID) ||
		!validRichIDs(c.CollectorUID, c.UploaderUID, c.UploaderGID, c.SharedGID) ||
		c.DefinitionSHA256 != RichProcessesDefinitionSHA256 || !digestPattern.MatchString(c.PolicySHA256) ||
		!digestPattern.MatchString(c.ArtifactSHA256) || c.PolicyGeneration == 0 || !validRichAddresses(c.Addresses) {
		return ErrUnsafe
	}
	return nil
}

func (p RichProcessesPolicy) Validate() error {
	if p.Version != "observer-connected-rich-policy/v1" || p.Profile != RichProcessesProfile ||
		!ValidServerID(p.ServerID) || !ValidConnectorID(p.ConnectorID) ||
		!validRichIDs(p.CollectorUID, p.UploaderUID, p.UploaderGID, p.SharedGID) ||
		p.DefinitionSHA256 != RichProcessesDefinitionSHA256 || !p.ProcessesEnabled || p.ContainersEnabled {
		return ErrUnsafe
	}
	return nil
}

func EncodeRichProcessesConfig(c RichProcessesConfig) ([]byte, error) {
	if c.Validate() != nil {
		return nil, ErrUnsafe
	}
	return json.Marshal(c)
}

func DecodeRichProcessesConfig(data []byte) (RichProcessesConfig, error) {
	var c RichProcessesConfig
	if len(data) == 0 || len(data) > MaxConfigBytes || json.Unmarshal(data, &c) != nil || c.Validate() != nil {
		return RichProcessesConfig{}, ErrUnsafe
	}
	canonical, _ := json.Marshal(c)
	if !bytes.Equal(data, canonical) {
		return RichProcessesConfig{}, ErrUnsafe
	}
	return c, nil
}

func EncodeRichProcessesPolicy(p RichProcessesPolicy) ([]byte, error) {
	if p.Validate() != nil {
		return nil, ErrUnsafe
	}
	return json.Marshal(p)
}

func DecodeRichProcessesPolicy(data []byte) (RichProcessesPolicy, error) {
	var p RichProcessesPolicy
	if len(data) == 0 || len(data) > RichProcessesMaxPolicyBytes || json.Unmarshal(data, &p) != nil || p.Validate() != nil {
		return RichProcessesPolicy{}, ErrUnsafe
	}
	canonical, _ := json.Marshal(p)
	if !bytes.Equal(data, canonical) {
		return RichProcessesPolicy{}, ErrUnsafe
	}
	return p, nil
}

func DigestRichProcessesPolicy(p RichProcessesPolicy) ([32]byte, error) {
	data, err := EncodeRichProcessesPolicy(p)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(data), nil
}

func VerifyRichProcessesBinding(c RichProcessesConfig, p RichProcessesPolicy) error {
	if c.Validate() != nil || p.Validate() != nil || c.ServerID != p.ServerID || c.ConnectorID != p.ConnectorID ||
		c.CollectorUID != p.CollectorUID || c.UploaderUID != p.UploaderUID || c.UploaderGID != p.UploaderGID || c.SharedGID != p.SharedGID {
		return ErrUnsafe
	}
	digest, err := DigestRichProcessesPolicy(p)
	if err != nil || hex.EncodeToString(digest[:]) != c.PolicySHA256 {
		return ErrUnsafe
	}
	return nil
}

func init() {
	if sum := sha256.Sum256([]byte(RichProcessesDefinition)); hex.EncodeToString(sum[:]) != RichProcessesDefinitionSHA256 {
		panic("connected rich definition digest mismatch")
	}
}
