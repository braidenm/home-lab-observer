package sharedhandoff

import (
	"bytes"
	"encoding/hex"
	"encoding/json"

	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
)

// RichProcessesPolicy is an installed binding, not consent derived from a body.
// The caller authenticates its root-owned policy before opening and every use.
type RichProcessesPolicy struct {
	Policy
	PolicyDigest [32]byte
}

// A fixed small envelope binds the canonical combined document to installed
// consent. Numeric handoff limits and records remain unchanged.
const RichProcessesMaxBytes = combinedprojection.MaxBytes + 512

type richRecord struct {
	Version string          `json:"version"`
	Profile string          `json:"profile"`
	Server  string          `json:"server_id"`
	Digest  string          `json:"policy_sha256"`
	Body    json.RawMessage `json:"observation"`
}

func validRichPolicy(p RichProcessesPolicy) bool {
	validID := func(id uint32) bool { return id != 0 && id != ^uint32(0) }
	return validID(p.CollectorUID) && validID(p.UploaderUID) && validID(p.SharedGID) && p.CollectorUID != p.UploaderUID && p.PolicyDigest != [32]byte{}
}

func encodeRichProcesses(body []byte, p RichProcessesPolicy) ([]byte, error) {
	if !validRichPolicy(p) || combinedprojection.Validate(body, p.ServerID) != nil {
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
	if json.Unmarshal(body, &groups) != nil || !groups.Workloads.Policy.Processes || groups.Workloads.Policy.Containers {
		return nil, ErrInvalid
	}
	b, err := json.Marshal(richRecord{Version: "observer-connected-rich-handoff/v1", Profile: "RICH_PROCESSES_V1", Server: p.ServerID, Digest: hex.EncodeToString(p.PolicyDigest[:]), Body: body})
	if err != nil || len(b) > RichProcessesMaxBytes {
		return nil, ErrInvalid
	}
	return b, nil
}

func decodeRichProcesses(b []byte, p RichProcessesPolicy) ([]byte, error) {
	if len(b) == 0 || len(b) > RichProcessesMaxBytes {
		return nil, ErrInvalid
	}
	var record richRecord
	if json.Unmarshal(b, &record) != nil {
		return nil, ErrInvalid
	}
	canonical, err := encodeRichProcesses(record.Body, p)
	if err != nil || !bytes.Equal(b, canonical) {
		return nil, ErrInvalid
	}
	return bytes.Clone(record.Body), nil
}
