package connectedprofile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
)

const (
	RichProcessesDockerVersion = "observer-connected-rich-docker-install/v1"
	RichProcessesDockerProfile = "RICH_PROCESSES_DOCKER_V1"
	// Rich profiles deliberately share one exclusive installation location.
	RichProcessesDockerConfigDirectory       = RichProcessesConfigDirectory
	RichProcessesDockerConfigPath            = RichProcessesConfigPath
	RichProcessesDockerPolicyPath            = RichProcessesPolicyPath
	RichProcessesDockerStateDirectory        = RichProcessesStateDirectory
	RichProcessesDockerReleaseDirectory      = RichProcessesReleaseDirectory
	RichProcessesDockerCollectorName         = "hlo-rich-docker-collector"
	RichProcessesDockerUploaderName          = "hlo-rich-docker-uploader"
	RichProcessesDockerReaderName            = "hlo-rich-docker-reader"
	RichProcessesDockerSharedName            = "hlo-rich-docker-read"
	RichProcessesDockerFrameName             = "hlo-rich-docker-frame"
	RichProcessesDockerReaderRoot            = RichProcessesDockerStateDirectory + "/reader-root"
	RichProcessesDockerFrameDirectory        = RichProcessesDockerStateDirectory + "/docker-frame"
	RichProcessesDockerReaderStatusDirectory = RichProcessesDockerStateDirectory + "/reader-status"
	RichProcessesDockerMaxPolicyBytes        = RichProcessesMaxPolicyBytes

	// This unreleased definition is frozen only with the reviewed resources and
	// successful installed-artifact acceptance; it is not enrollment approval.
	RichProcessesDockerDefinition       = `{"version":"observer-connected-rich-docker-definition/v1","profile":"RICH_PROCESSES_DOCKER_V1","combined_schema":"home-lab-connected-observation/v1","combined_profile":"numeric-host-workloads/v1","host_schema":"home-lab-native-host-metrics/v1","workload_schema":"home-lab-workload-inventory/v1","workload_profile":"process-container/v1","processes_enabled":true,"containers_enabled":true,"collection_deadline_seconds":10,"network_collection":"disabled","max_body_bytes":294912,"ledger_user_version":3,"ledger_profile_kind":2,"ledger_db_bytes":524288,"ledger_page_bytes":4096,"ledger_total_bytes":2097152,"handoff_schema":"observer-connected-rich-handoff/v1","handoff_record_bytes":295424,"handoff_slots":2,"max_process_rows":200,"max_container_rows":500,"max_metadata_items":4096,"denied_process_fields":["command_line","environment","open_files","network_endpoints"],"denied_container_fields":["command","environment","labels","mounts","ports","networks"],"workers":["rich-docker-collector","rich-docker-reader","rich-docker-uploader","rich-docker-install"],"principals":["hlo-rich-docker-collector","hlo-rich-docker-uploader","hlo-rich-docker-reader","hlo-rich-docker-read","hlo-rich-docker-frame"],"collector_proc_visibility":"default","collector_ptrace":false,"collector_network":false,"collector_credentials":false,"collector_docker_socket":false,"reader_cycle_seconds":15,"reader_deadline_seconds":4,"frame_max_age_seconds":45,"frame_future_skew_seconds":5,"reader_memory_max_bytes":134217728,"reader_tasks_max":32,"reader_cpu_percent":25,"reader_gomaxprocs":2,"reader_go_memory_bytes":67108864,"reader_stop_seconds":15,"reader_socket":"/engine/docker.sock","reader_socket_origin":"/run/docker.sock","reader_socket_mode":"0660","reader_socket_group":"approved_service_only","reader_socket_replacement":"explicit_stop_start","reader_startup_domain":"observer-connected-docker-reader-startup/v1","reader_startup_role":"rich-docker-reader","collector_activation_channels":"denied","reader_startup_request":"observer-connected-activation-request/v1","reader_startup_response":"observer-connected-activation-response/v1","reader_startup_commit":"observer-connected-activation-commit/v1","reader_primary_group":"frame_gid","reader_supplementary_groups":["socket_gid"],"collector_supplementary_groups":["frame_gid"],"reader_activation_source":"/run/home-lab-observer-connected-rich/reader-activation","reader_activation_directory_owner":"root:frame_gid","reader_activation_directory_mode":"0750","reader_activation_record_owner":"root:frame_gid","reader_activation_record_mode":"0640","reader_activation_mount":"/activation","reader_request_port_fields":"inert_1024_never_probed","reader_network":"unix_only","reader_credentials":false,"reader_proc_visibility":"no_host_mount","reader_stdout":"null","reader_stderr":"null","frame_schema":"observer-connected-docker-frame/v1","frame_section_bytes":130560,"frame_metadata_bytes":512,"frame_record_bytes":131072,"frame_slots":2,"uploader_proc_visibility":"invisible","uploader_network":"allowlisted_tls","uploader_credentials":true,"uploader_docker_socket":false,"uploader_frame_access":false,"config_root":"/etc/home-lab-observer-connected-rich","state_root":"/var/lib/home-lab-observer-connected-rich","release_root":"/opt/home-lab-observer-connected-rich/releases","collector_unit":"home-lab-observer-connected-rich-docker-collector.service","reader_unit":"home-lab-observer-connected-rich-docker-reader.service","uploader_unit":"home-lab-observer-connected-rich-docker-uploader.service","automatic_boot":false,"automatic_restart":false}`
	RichProcessesDockerDefinitionSHA256 = "86f39c7ba444f5ef4560ba82cbf660a17c0d7810b6e49443cb509d9e85036f37"
)

type RichProcessesDockerConfig struct {
	Version          string   `json:"version"`
	State            string   `json:"state"`
	Profile          string   `json:"collection_profile"`
	ServerID         string   `json:"server_id"`
	ConnectorID      string   `json:"connector_id"`
	CollectorUID     uint32   `json:"collector_uid"`
	UploaderUID      uint32   `json:"uploader_uid"`
	UploaderGID      uint32   `json:"uploader_gid"`
	SharedGID        uint32   `json:"shared_gid"`
	ReaderUID        uint32   `json:"reader_uid"`
	FrameGID         uint32   `json:"frame_gid"`
	SocketGID        uint32   `json:"socket_gid"`
	DefinitionSHA256 string   `json:"collection_definition_sha256"`
	PolicySHA256     string   `json:"collection_policy_sha256"`
	ArtifactSHA256   string   `json:"artifact_sha256"`
	PolicyGeneration uint64   `json:"policy_generation"`
	Addresses        []string `json:"addresses"`
}

type RichProcessesDockerPolicy struct {
	Version           string `json:"version"`
	Profile           string `json:"collection_profile"`
	ServerID          string `json:"server_id"`
	ConnectorID       string `json:"connector_id"`
	CollectorUID      uint32 `json:"collector_uid"`
	UploaderUID       uint32 `json:"uploader_uid"`
	UploaderGID       uint32 `json:"uploader_gid"`
	SharedGID         uint32 `json:"shared_gid"`
	ReaderUID         uint32 `json:"reader_uid"`
	FrameGID          uint32 `json:"frame_gid"`
	SocketGID         uint32 `json:"socket_gid"`
	DefinitionSHA256  string `json:"collection_definition_sha256"`
	ProcessesEnabled  bool   `json:"processes_enabled"`
	ContainersEnabled bool   `json:"containers_enabled"`
}

func NewRichProcessesDockerPolicy(server, connector string, collectorUID, uploaderUID, uploaderGID, sharedGID, readerUID, frameGID, socketGID uint32) (RichProcessesDockerPolicy, error) {
	p := RichProcessesDockerPolicy{Version: "observer-connected-rich-docker-policy/v1", Profile: RichProcessesDockerProfile,
		ServerID: server, ConnectorID: connector, CollectorUID: collectorUID, UploaderUID: uploaderUID, UploaderGID: uploaderGID,
		SharedGID: sharedGID, ReaderUID: readerUID, FrameGID: frameGID, SocketGID: socketGID,
		DefinitionSHA256: RichProcessesDockerDefinitionSHA256, ProcessesEnabled: true, ContainersEnabled: true}
	if p.Validate() != nil {
		return RichProcessesDockerPolicy{}, ErrUnsafe
	}
	return p, nil
}

func validDockerIDs(collector, uploader, reader, uploaderGID, shared, frame, socket uint32) bool {
	for _, v := range []uint32{collector, uploader, reader, uploaderGID, shared, frame, socket} {
		if v == 0 || v == math.MaxUint32 {
			return false
		}
	}
	return collector != uploader && collector != reader && uploader != reader && uploaderGID != shared && uploaderGID != frame && uploaderGID != socket && shared != frame && shared != socket && frame != socket
}

func (c RichProcessesDockerConfig) Validate() error {
	if c.Version != RichProcessesDockerVersion || c.State != "INSTALLED_READY" || c.Profile != RichProcessesDockerProfile || !ValidServerID(c.ServerID) || !ValidConnectorID(c.ConnectorID) ||
		!validDockerIDs(c.CollectorUID, c.UploaderUID, c.ReaderUID, c.UploaderGID, c.SharedGID, c.FrameGID, c.SocketGID) || c.DefinitionSHA256 != RichProcessesDockerDefinitionSHA256 ||
		!digestPattern.MatchString(c.PolicySHA256) || !digestPattern.MatchString(c.ArtifactSHA256) || c.PolicyGeneration == 0 || !validRichAddresses(c.Addresses) {
		return ErrUnsafe
	}
	return nil
}

func (p RichProcessesDockerPolicy) Validate() error {
	if p.Version != "observer-connected-rich-docker-policy/v1" || p.Profile != RichProcessesDockerProfile || !ValidServerID(p.ServerID) || !ValidConnectorID(p.ConnectorID) ||
		!validDockerIDs(p.CollectorUID, p.UploaderUID, p.ReaderUID, p.UploaderGID, p.SharedGID, p.FrameGID, p.SocketGID) || p.DefinitionSHA256 != RichProcessesDockerDefinitionSHA256 || !p.ProcessesEnabled || !p.ContainersEnabled {
		return ErrUnsafe
	}
	return nil
}

func EncodeRichProcessesDockerConfig(c RichProcessesDockerConfig) ([]byte, error) {
	if c.Validate() != nil {
		return nil, ErrUnsafe
	}
	return json.Marshal(c)
}
func DecodeRichProcessesDockerConfig(b []byte) (RichProcessesDockerConfig, error) {
	var c RichProcessesDockerConfig
	if len(b) == 0 || len(b) > MaxConfigBytes || json.Unmarshal(b, &c) != nil || c.Validate() != nil {
		return RichProcessesDockerConfig{}, ErrUnsafe
	}
	canonical, _ := json.Marshal(c)
	if !bytes.Equal(b, canonical) {
		return RichProcessesDockerConfig{}, ErrUnsafe
	}
	return c, nil
}
func EncodeRichProcessesDockerPolicy(p RichProcessesDockerPolicy) ([]byte, error) {
	if p.Validate() != nil {
		return nil, ErrUnsafe
	}
	return json.Marshal(p)
}
func DecodeRichProcessesDockerPolicy(b []byte) (RichProcessesDockerPolicy, error) {
	var p RichProcessesDockerPolicy
	if len(b) == 0 || len(b) > RichProcessesDockerMaxPolicyBytes || json.Unmarshal(b, &p) != nil || p.Validate() != nil {
		return RichProcessesDockerPolicy{}, ErrUnsafe
	}
	canonical, _ := json.Marshal(p)
	if !bytes.Equal(b, canonical) {
		return RichProcessesDockerPolicy{}, ErrUnsafe
	}
	return p, nil
}
func DigestRichProcessesDockerPolicy(p RichProcessesDockerPolicy) ([32]byte, error) {
	b, err := EncodeRichProcessesDockerPolicy(p)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(b), nil
}
func VerifyRichProcessesDockerBinding(c RichProcessesDockerConfig, p RichProcessesDockerPolicy) error {
	if c.Validate() != nil || p.Validate() != nil || c.ServerID != p.ServerID || c.ConnectorID != p.ConnectorID || c.CollectorUID != p.CollectorUID || c.UploaderUID != p.UploaderUID || c.UploaderGID != p.UploaderGID || c.SharedGID != p.SharedGID || c.ReaderUID != p.ReaderUID || c.FrameGID != p.FrameGID || c.SocketGID != p.SocketGID {
		return ErrUnsafe
	}
	d, err := DigestRichProcessesDockerPolicy(p)
	if err != nil || hex.EncodeToString(d[:]) != c.PolicySHA256 {
		return ErrUnsafe
	}
	return nil
}

func init() {
	sum := sha256.Sum256([]byte(RichProcessesDockerDefinition))
	if hex.EncodeToString(sum[:]) != RichProcessesDockerDefinitionSHA256 {
		panic("connected Docker definition digest mismatch")
	}
}
