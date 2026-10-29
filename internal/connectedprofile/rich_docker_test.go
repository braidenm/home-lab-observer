package connectedprofile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func dockerPolicyFixture(t *testing.T) RichProcessesDockerPolicy {
	t.Helper()
	p, err := NewRichProcessesDockerPolicy("srv_"+strings.Repeat("a", 32), "agent_"+strings.Repeat("b", 32), 62001, 62002, 62003, 62004, 62005, 62006, 998)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func dockerConfigFixture(t *testing.T) RichProcessesDockerConfig {
	t.Helper()
	p := dockerPolicyFixture(t)
	digest, err := DigestRichProcessesDockerPolicy(p)
	if err != nil {
		t.Fatal(err)
	}
	return RichProcessesDockerConfig{Version: RichProcessesDockerVersion, State: "INSTALLED_READY", Profile: RichProcessesDockerProfile,
		ServerID: p.ServerID, ConnectorID: p.ConnectorID, CollectorUID: p.CollectorUID, UploaderUID: p.UploaderUID,
		UploaderGID: p.UploaderGID, SharedGID: p.SharedGID, ReaderUID: p.ReaderUID, FrameGID: p.FrameGID, SocketGID: p.SocketGID,
		DefinitionSHA256: RichProcessesDockerDefinitionSHA256, PolicySHA256: hex.EncodeToString(digest[:]),
		ArtifactSHA256: strings.Repeat("c", 64), PolicyGeneration: 1, Addresses: []string{"1.1.1.1"}}
}

func TestDockerProfileIsClosedAndSeparatelyBound(t *testing.T) {
	p, c := dockerPolicyFixture(t), dockerConfigFixture(t)
	b, err := EncodeRichProcessesDockerPolicy(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeRichProcessesDockerPolicy(b)
	if err != nil || got != p || VerifyRichProcessesDockerBinding(c, p) != nil {
		t.Fatal("policy round trip/binding failed")
	}
	config, err := EncodeRichProcessesDockerConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRichProcessesDockerConfig(config)
	if err != nil || VerifyRichProcessesDockerBinding(decoded, p) != nil {
		t.Fatal("config round trip/binding failed")
	}
	if _, err := DecodeRichProcessesPolicy(b); err != ErrUnsafe {
		t.Fatal("Docker policy widened process-only decoding")
	}
	if _, err := DecodeRichProcessesConfig(config); err != ErrUnsafe {
		t.Fatal("Docker config widened process-only decoding")
	}
	for _, bad := range [][]byte{append(bytes.Clone(b), '\n'), bytes.Replace(b, []byte(`"containers_enabled":true`), []byte(`"containers_enabled":false`), 1), append(b[:len(b)-1], []byte(`,"extra":true}`)...)} {
		if _, err := DecodeRichProcessesDockerPolicy(bad); err != ErrUnsafe {
			t.Fatal("noncanonical or widened policy admitted")
		}
	}
	wrong := c
	wrong.SocketGID++
	if VerifyRichProcessesDockerBinding(wrong, p) != ErrUnsafe {
		t.Fatal("socket group was not policy-bound")
	}
	wrong = c
	wrong.ReaderUID++
	if VerifyRichProcessesDockerBinding(wrong, p) != ErrUnsafe {
		t.Fatal("reader was not policy-bound")
	}
	wrong = c
	wrong.FrameGID++
	if VerifyRichProcessesDockerBinding(wrong, p) != ErrUnsafe {
		t.Fatal("frame group was not policy-bound")
	}
}

func TestDockerProfileRejectsAuthorityConfusion(t *testing.T) {
	for name, change := range map[string]func(*RichProcessesDockerPolicy){
		"numeric":          func(p *RichProcessesDockerPolicy) { p.Profile = "NUMERIC_HOST_V1" },
		"process-only":     func(p *RichProcessesDockerPolicy) { p.Profile = RichProcessesProfile },
		"reader-root":      func(p *RichProcessesDockerPolicy) { p.ReaderUID = 0 },
		"reader-collector": func(p *RichProcessesDockerPolicy) { p.ReaderUID = p.CollectorUID },
		"reader-uploader":  func(p *RichProcessesDockerPolicy) { p.ReaderUID = p.UploaderUID },
		"frame-uploader":   func(p *RichProcessesDockerPolicy) { p.FrameGID = p.UploaderGID },
		"frame-handoff":    func(p *RichProcessesDockerPolicy) { p.FrameGID = p.SharedGID },
		"socket-frame":     func(p *RichProcessesDockerPolicy) { p.SocketGID = p.FrameGID },
		"socket-uploader":  func(p *RichProcessesDockerPolicy) { p.SocketGID = p.UploaderGID },
		"socket-root":      func(p *RichProcessesDockerPolicy) { p.SocketGID = 0 },
		"processes-off":    func(p *RichProcessesDockerPolicy) { p.ProcessesEnabled = false },
		"containers-off":   func(p *RichProcessesDockerPolicy) { p.ContainersEnabled = false },
		"definition":       func(p *RichProcessesDockerPolicy) { p.DefinitionSHA256 = RichProcessesDefinitionSHA256 },
	} {
		t.Run(name, func(t *testing.T) {
			p := dockerPolicyFixture(t)
			change(&p)
			if p.Validate() != ErrUnsafe {
				t.Fatal("authority confusion admitted")
			}
		})
	}
	p := dockerPolicyFixture(t)
	p.ReaderUID = p.FrameGID
	if p.Validate() != nil {
		t.Fatal("UID/GID namespace overlap was incorrectly refused")
	}
}

func TestDockerDefinitionAndExclusiveRoots(t *testing.T) {
	sum := sha256.Sum256([]byte(RichProcessesDockerDefinition))
	if hex.EncodeToString(sum[:]) != RichProcessesDockerDefinitionSHA256 || RichProcessesDockerDefinitionSHA256 == RichProcessesDefinitionSHA256 {
		t.Fatal("definition digest mismatch")
	}
	if RichProcessesDockerConfigDirectory != RichProcessesConfigDirectory || RichProcessesDockerStateDirectory != RichProcessesStateDirectory || RichProcessesDockerReleaseDirectory != RichProcessesReleaseDirectory {
		t.Fatal("a second rich installation root was introduced")
	}
	for _, want := range []string{`"profile":"RICH_PROCESSES_DOCKER_V1"`, `"ledger_user_version":3`, `"ledger_profile_kind":2`, `"reader_cycle_seconds":15`, `"reader_deadline_seconds":4`, `"frame_max_age_seconds":45`, `"frame_future_skew_seconds":5`, `"reader_memory_max_bytes":134217728`, `"reader_tasks_max":32`, `"reader_cpu_percent":25`, `"reader_gomaxprocs":2`, `"reader_go_memory_bytes":67108864`, `"frame_schema":"observer-connected-docker-frame/v1"`, `"frame_section_bytes":130560`, `"frame_metadata_bytes":512`, `"frame_record_bytes":131072`, `"frame_slots":2`} {
		if !strings.Contains(RichProcessesDockerDefinition, want) {
			t.Fatalf("definition missing fixed field %s", want)
		}
	}
}
