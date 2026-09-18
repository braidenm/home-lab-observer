// Package connectedtransition defines a pure, bounded transition record codec.
// It has no filesystem, installer, credential, network, or activation authority.
package connectedtransition

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/ledgeridentity"
)

const Version = "observer-connected-transition/v1"
const MaxRecordBytes = 16 << 10

type PredecessorFormat string

const (
	PredecessorNone       PredecessorFormat = "none"
	PredecessorLegacy     PredecessorFormat = "legacy-refresh-v1"
	PredecessorTransition PredecessorFormat = "transition-v1"
)

var ErrInvalid = errors.New("connected_transition_invalid")

type Resources struct {
	CA              string `json:"ca_sha256"`
	Hosts           string `json:"hosts_sha256"`
	CollectorUnit   string `json:"collector_unit_sha256"`
	UploaderUnit    string `json:"uploader_unit_sha256"`
	InstalledConfig string `json:"installed_config_sha256"`
}

type Object struct {
	Inode      uint64 `json:"inode"`
	Generation uint32 `json:"generation"`
}

type PhysicalLedger struct {
	FilesystemUUID string `json:"filesystem_uuid"`
	Directory      Object `json:"directory"`
	Database       Object `json:"database"`
}

// FromWitness converts an already established private physical witness. The
// caller is responsible for retaining it privately and validating the ledger.
func FromWitness(w ledgeridentity.Witness) PhysicalLedger {
	return PhysicalLedger{w.FilesystemUUID, Object{w.Directory.Inode, w.Directory.Generation}, Object{w.Database.Inode, w.Database.Generation}}
}

func (p PhysicalLedger) witness() ledgeridentity.Witness {
	return ledgeridentity.Witness{FilesystemUUID: p.FilesystemUUID, Directory: ledgeridentity.Object{Inode: p.Directory.Inode, Generation: p.Directory.Generation}, Database: ledgeridentity.Object{Inode: p.Database.Inode, Generation: p.Database.Generation}}
}

type Ledger struct {
	LogicalSHA256 string         `json:"logical_sha256"`
	Physical      PhysicalLedger `json:"physical"`
}

type Record struct {
	Version                  string                  `json:"version"`
	Operation                string                  `json:"operation"`
	Previous                 connectedprofile.Config `json:"previous"`
	Next                     connectedprofile.Config `json:"next"`
	PredecessorFormat        PredecessorFormat       `json:"predecessor_format"`
	PredecessorCompletionSHA string                  `json:"predecessor_completion_sha256"`
	PreviousCodeBefore       string                  `json:"previous_code_before"`
	PreviousCodeAfter        string                  `json:"previous_code_after"`
	ContractSHA256           string                  `json:"contract_sha256"`
	PreviousResources        Resources               `json:"previous_resources"`
	NextResources            Resources               `json:"next_resources"`
	Ledger                   Ledger                  `json:"ledger"`
}

func digest(value string, optional bool) bool {
	if optional && value == "" {
		return true
	}
	if len(value) != 64 {
		return false
	}
	b, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(b) == value
}

func (r Resources) valid() bool {
	return digest(r.CA, false) && digest(r.Hosts, false) && digest(r.CollectorUnit, false) && digest(r.UploaderUnit, false) && digest(r.InstalledConfig, false)
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hosts(addresses []string) string {
	var b strings.Builder
	for _, address := range addresses {
		b.WriteString(address)
		b.WriteByte(' ')
		b.WriteString(connectedprofile.Hostname)
		b.WriteByte('\n')
	}
	return hash([]byte(b.String()))
}

// Validate checks record-internal consistency only. It does not prove that any
// described bundle, resource, predecessor or ledger actually exists.
func (r Record) Validate() error {
	if r.Version != Version || r.Previous.Validate() != nil || r.Next.Validate() != nil ||
		r.Previous.PolicyGeneration == math.MaxUint64 || r.Next.PolicyGeneration != r.Previous.PolicyGeneration+1 ||
		!validPredecessor(r.PredecessorFormat, r.PredecessorCompletionSHA) ||
		(r.PredecessorFormat == PredecessorNone && r.Previous.PolicyGeneration != 1) ||
		!digest(r.PreviousCodeBefore, true) || !digest(r.PreviousCodeAfter, true) ||
		!digest(r.ContractSHA256, false) || !r.PreviousResources.valid() || !r.NextResources.valid() ||
		!digest(r.Ledger.LogicalSHA256, false) || ledgeridentity.Validate(r.Ledger.Physical.witness()) != nil {
		return ErrInvalid
	}
	if r.Operation != "refresh" && r.Operation != "code-select" {
		return ErrInvalid
	}
	// Begin with an exact copy; replace only fields authorized by the operation.
	want := r.Previous
	want.PolicyGeneration = r.Next.PolicyGeneration
	if r.Operation == "refresh" {
		want.Addresses = slices.Clone(r.Next.Addresses)
		if r.PreviousCodeAfter != r.PreviousCodeBefore {
			return ErrInvalid
		}
	} else {
		want.ArtifactSHA256 = r.Next.ArtifactSHA256
		if r.Next.ArtifactSHA256 == r.Previous.ArtifactSHA256 || r.PreviousCodeAfter != r.Previous.ArtifactSHA256 ||
			r.PreviousResources.CA != r.NextResources.CA || r.PreviousResources.Hosts != r.NextResources.Hosts {
			return ErrInvalid
		}
	}
	wantBytes, _ := connectedprofile.Encode(want)
	nextBytes, _ := connectedprofile.Encode(r.Next)
	previousBytes, _ := connectedprofile.Encode(r.Previous)
	if !bytes.Equal(wantBytes, nextBytes) ||
		r.PreviousResources.InstalledConfig != hash(previousBytes) || r.NextResources.InstalledConfig != hash(nextBytes) ||
		r.PreviousResources.Hosts != hosts(r.Previous.Addresses) || r.NextResources.Hosts != hosts(r.Next.Addresses) {
		return ErrInvalid
	}
	return nil
}

func validPredecessor(format PredecessorFormat, receiptSHA string) bool {
	switch format {
	case PredecessorNone:
		return receiptSHA == ""
	case PredecessorLegacy, PredecessorTransition:
		return digest(receiptSHA, false)
	default:
		return false
	}
}

func Encode(r Record) ([]byte, error) {
	if r.Validate() != nil {
		return nil, ErrInvalid
	}
	b, err := json.Marshal(r)
	if err != nil || len(b) == 0 || len(b) > MaxRecordBytes {
		return nil, ErrInvalid
	}
	return b, nil
}

func Decode(data []byte) (Record, error) {
	if len(data) == 0 || len(data) > MaxRecordBytes {
		return Record{}, ErrInvalid
	}
	var r Record
	if json.Unmarshal(data, &r) != nil {
		return Record{}, ErrInvalid
	}
	canonical, err := Encode(r)
	if err != nil || !bytes.Equal(canonical, data) {
		return Record{}, ErrInvalid
	}
	return r, nil
}

// Completion is a digest-bound receipt; it never permits worker startup.
func Completion(canonicalRecord []byte) ([]byte, error) {
	if _, err := Decode(canonicalRecord); err != nil {
		return nil, ErrInvalid
	}
	sum := sha256.Sum256(canonicalRecord)
	return []byte("observer-connected-transition-complete/v1\n" + hex.EncodeToString(sum[:]) + "\n"), nil
}
