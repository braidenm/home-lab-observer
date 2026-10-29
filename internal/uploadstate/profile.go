package uploadstate

import (
	"encoding/json"
	"time"

	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
)

// ProfileKind is a closed local validation choice, not a wire-schema claim or
// proof of persisted server approval. The numeric zero preserves existing APIs.
type ProfileKind uint8

const (
	NumericProfile ProfileKind = iota
	// RichProcessesProfile corresponds to server approval RICH_PROCESSES_V1.
	// It never allows container-enabled inventory, even if that document is valid.
	RichProcessesProfile
	// RichProcessesDockerProfile is separately approved for both workload groups.
	RichProcessesDockerProfile
)

// ProfileBinding is immutable for an installation's upload identity. The caller
// authenticates the expected policy record separately; a digest is not authority.
type ProfileBinding struct {
	Kind         ProfileKind
	PolicyDigest [32]byte
}

func validProfile(p ProfileBinding) bool {
	switch p.Kind {
	case NumericProfile:
		return p.PolicyDigest == [32]byte{}
	case RichProcessesProfile, RichProcessesDockerProfile:
		return p.PolicyDigest != [32]byte{}
	default:
		return false
	}
}

func maxBodyBytes(p ProfileBinding) int {
	if p.Kind == RichProcessesProfile || p.Kind == RichProcessesDockerProfile {
		return combinedprojection.MaxBytes
	}
	return remoteprojection.MaxBytes
}

func collectionTime(body []byte, server string, p ProfileBinding) (time.Time, bool) {
	if !validProfile(p) || len(body) == 0 || len(body) > maxBodyBytes(p) {
		return time.Time{}, false
	}
	if p.Kind == NumericProfile {
		at, err := remoteprojection.UploadCollectionTime(body, server)
		return at, err == nil
	}
	at, err := combinedprojection.CollectionTime(body, server)
	if err != nil {
		return time.Time{}, false
	}
	// The canonical combined validator has already checked all fields, duplicate
	// keys, disabled-section invariants and privacy. Extract only the group claim
	// needed for this narrower installed profile; do not infer authority from it.
	var groups struct {
		Workloads struct {
			Policy struct {
				Processes  bool `json:"processes_enabled"`
				Containers bool `json:"containers_enabled"`
			} `json:"policy"`
		} `json:"workload_inventory"`
	}
	if json.Unmarshal(body, &groups) != nil || !groups.Workloads.Policy.Processes || groups.Workloads.Policy.Containers != (p.Kind == RichProcessesDockerProfile) {
		return time.Time{}, false
	}
	return at, true
}
