package sharedhandoff

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

const (
	RichProcessesDockerMaxBytes = combinedprojection.MaxBytes + 512
	DockerFrameMaxBytes         = 128 * 1024
	DockerFrameMaxAge           = 45 * time.Second
	DockerFrameFutureSkew       = 5 * time.Second
	dockerProfile               = "RICH_PROCESSES_DOCKER_V1"
	dockerFrameVersion          = "observer-connected-docker-frame/v1"
)

// RichProcessesDockerPolicy binds the collector/uploader boundary. It is not
// approval inferred from an observation; callers authenticate policy each use.
type RichProcessesDockerPolicy struct {
	Policy
	PolicyDigest [32]byte
}

// DockerFramePolicy uses CollectorUID for the reader/producer, UploaderUID for
// the host collector/consumer, and SharedGID for its dedicated frame group.
// Neither digest is a MAC: fixed principal/DAC admission is an independent gate.
type DockerFramePolicy struct {
	Policy
	DefinitionDigest, PolicyDigest [32]byte
}

func validDockerPolicy(p RichProcessesDockerPolicy) bool {
	return validRichPolicy(RichProcessesPolicy{Policy: p.Policy, PolicyDigest: p.PolicyDigest}) && serverPattern.MatchString(p.ServerID)
}

func encodeRichProcessesDocker(body []byte, p RichProcessesDockerPolicy) ([]byte, error) {
	if !validDockerPolicy(p) || combinedprojection.Validate(body, p.ServerID) != nil {
		return nil, ErrInvalid
	}
	var groups struct {
		Workloads struct {
			Policy struct {
				Processes  bool `json:"processes_enabled"`
				Containers bool `json:"containers_enabled"`
			} `json:"policy"`
		} `json:"workload_inventory"`
	}
	if json.Unmarshal(body, &groups) != nil || !groups.Workloads.Policy.Processes || !groups.Workloads.Policy.Containers {
		return nil, ErrInvalid
	}
	b, err := json.Marshal(richRecord{Version: "observer-connected-rich-handoff/v1", Profile: dockerProfile, Server: p.ServerID, Digest: hex.EncodeToString(p.PolicyDigest[:]), Body: body})
	if err != nil || len(b) > RichProcessesDockerMaxBytes {
		return nil, ErrInvalid
	}
	return b, nil
}

func decodeRichProcessesDocker(b []byte, p RichProcessesDockerPolicy) ([]byte, error) {
	if len(b) == 0 || len(b) > RichProcessesDockerMaxBytes {
		return nil, ErrInvalid
	}
	var record richRecord
	if json.Unmarshal(b, &record) != nil {
		return nil, ErrInvalid
	}
	canonical, err := encodeRichProcessesDocker(record.Body, p)
	if err != nil || !bytes.Equal(b, canonical) {
		return nil, ErrInvalid
	}
	return bytes.Clone(record.Body), nil
}

type dockerFrame struct {
	Version       string          `json:"version"`
	Profile       string          `json:"profile"`
	Server        string          `json:"server_id"`
	Definition    string          `json:"definition_sha256"`
	Policy        string          `json:"policy_sha256"`
	ProducedAt    string          `json:"produced_at"`
	SectionDigest string          `json:"section_sha256"`
	Containers    json.RawMessage `json:"containers"`
}

func validFramePolicy(p DockerFramePolicy) bool {
	return validDockerPolicy(RichProcessesDockerPolicy{Policy: p.Policy, PolicyDigest: p.PolicyDigest}) && p.DefinitionDigest != [32]byte{}
}

func frameTime(at time.Time) bool {
	at = at.UTC()
	return !at.IsZero() && at.Year() >= 1 && at.Year() <= 9999
}

func freshDockerFrame(s workloadprojection.ContainerSection, produced, now time.Time) bool {
	if !frameTime(produced) || !frameTime(now) || now.Sub(produced) > DockerFrameMaxAge || produced.Sub(now) > DockerFrameFutureSkew {
		return false
	}
	if at, ok := s.ObservedAt(); ok {
		return now.Sub(at) <= DockerFrameMaxAge && at.Sub(now) <= DockerFrameFutureSkew && at.Sub(produced) <= DockerFrameFutureSkew
	}
	_, err := s.Bytes()
	return err == nil
}

func encodeDockerFrame(section workloadprojection.ContainerSection, at time.Time, p DockerFramePolicy) ([]byte, error) {
	if !validFramePolicy(p) || !freshDockerFrame(section, at, at) {
		return nil, ErrInvalid
	}
	body, err := section.Bytes()
	if err != nil {
		return nil, ErrInvalid
	}
	digest := sha256.Sum256(body)
	b, err := json.Marshal(dockerFrame{Version: dockerFrameVersion, Profile: dockerProfile, Server: p.ServerID,
		Definition: hex.EncodeToString(p.DefinitionDigest[:]), Policy: hex.EncodeToString(p.PolicyDigest[:]), ProducedAt: at.UTC().Format(time.RFC3339Nano), SectionDigest: hex.EncodeToString(digest[:]), Containers: body})
	if err != nil || len(b) > DockerFrameMaxBytes {
		return nil, ErrInvalid
	}
	return b, nil
}

func decodeDockerFrame(b []byte, p DockerFramePolicy) (workloadprojection.ContainerSection, time.Time, error) {
	invalid := func() (workloadprojection.ContainerSection, time.Time, error) {
		return workloadprojection.ContainerSection{}, time.Time{}, ErrInvalid
	}
	if len(b) == 0 || len(b) > DockerFrameMaxBytes || !validFramePolicy(p) {
		return invalid()
	}
	var frame dockerFrame
	if json.Unmarshal(b, &frame) != nil {
		return invalid()
	}
	at, err := time.Parse(time.RFC3339Nano, frame.ProducedAt)
	if err != nil || !frameTime(at) || at.UTC().Format(time.RFC3339Nano) != frame.ProducedAt {
		return invalid()
	}
	s, err := workloadprojection.ParseContainers(frame.Containers)
	if err != nil {
		return invalid()
	}
	canonical, err := encodeDockerFrame(s, at, p)
	if err != nil || !bytes.Equal(b, canonical) {
		return invalid()
	}
	return s, at, nil
}
