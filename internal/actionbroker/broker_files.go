package actionbroker

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"
	"strconv"
	"unicode/utf8"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionengine"
)

const engineDocumentLimit = 4096

type engineApproval struct {
	generation string
	witness    actionengine.SocketWitness
	digest     [32]byte
}

// Values originate only in the separately protected root approval document.
// This parser never inspects a live socket, selects a path or adopts a witness.
func decodeEngineApproval(raw []byte) (engineApproval, error) {
	if len(raw) == 0 || len(raw) > engineDocumentLimit || !utf8.Valid(raw) {
		return engineApproval{}, ErrPolicy
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return engineApproval{}, ErrPolicy
	}
	fields := []string{"version", "engine_generation", "socket_device", "socket_inode", "socket_group_id"}
	values := make([]string, len(fields))
	for i, name := range fields {
		key, err := d.Token()
		if err != nil || key != name {
			return engineApproval{}, ErrPolicy
		}
		value, err := d.Token()
		text, ok := value.(string)
		if err != nil || !ok {
			return engineApproval{}, ErrPolicy
		}
		values[i] = text
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') {
		return engineApproval{}, ErrPolicy
	}
	if _, err := d.Token(); err != io.EOF {
		return engineApproval{}, ErrPolicy
	}
	device, e1 := strconv.ParseUint(values[2], 10, 64)
	inode, e2 := strconv.ParseUint(values[3], 10, 64)
	group, e3 := strconv.ParseUint(values[4], 10, 32)
	if values[0] != "observer-action-engine/v1" || !localHex(values[1], 32) || e1 != nil || e2 != nil || e3 != nil || inode == 0 || group == 0 ||
		strconv.FormatUint(device, 10) != values[2] || strconv.FormatUint(inode, 10) != values[3] || strconv.FormatUint(group, 10) != values[4] {
		return engineApproval{}, ErrPolicy
	}
	return engineApproval{generation: values[1], witness: actionengine.SocketWitness{Device: device, Inode: inode, GroupID: uint32(group)}, digest: sha256.Sum256(raw)}, nil
}

func validateEnginePolicy(p policyDocument, generation string) error {
	if validatePolicy(p) != nil || !localHex(generation, 32) {
		return ErrPolicy
	}
	current := make(map[string]bool, len(p.Current))
	for _, revision := range p.Current {
		current[revision] = true
	}
	for _, mapping := range p.Mappings {
		if current[mapping.MappingRevision] && mapping.EngineGeneration != generation {
			return ErrPolicy
		}
	}
	return nil
}

// References are read from the exclusively held validated journal, never IPC.
// Keeping an old engine mapping is evidence retention, NOT current-engine use.
func validateMappingReferences(p policyDocument, keys []actioncore.Key) error {
	if validatePolicy(p) != nil || len(keys) > 1000 {
		return ErrPolicy
	}
	mappings := make(map[string]bool, len(p.Mappings))
	for _, mapping := range p.Mappings {
		mappings[semantic(mapping)] = true
	}
	for _, key := range keys {
		mapping, err := mappingForKey(key)
		if err != nil || !mappings[semantic(mapping)] {
			return ErrPolicy
		}
	}
	return nil
}

func requireEngineGeneration(key actioncore.Key, generation string) error {
	if _, err := actioncore.CanonicalJournalMetadata(key); err != nil || !localHex(generation, 32) || key.Metadata.EngineGeneration != generation {
		return ErrPolicy
	}
	return nil
}
