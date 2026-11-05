// Package actionbroker owns the private local policy and execution conversations.
// A decoded policy is local scope only, never remote owner authentication.
package actionbroker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

var ErrPolicy = errors.New("action_policy_unavailable")

const policyBytes = 2 * 1024 * 1024

type mappingDocument struct {
	ServerID            string                    `json:"serverId"`
	OwnerUserID         string                    `json:"ownerUserId"`
	ConnectorInstanceID string                    `json:"connectorInstanceId"`
	TargetID            string                    `json:"targetId"`
	EngineGeneration    string                    `json:"engineGeneration"`
	ContainerID         string                    `json:"containerId"`
	PolicyRevision      string                    `json:"policyRevision"`
	CapabilityRevision  string                    `json:"capabilityRevision"`
	Operation           actioncore.Operation      `json:"operation"`
	Parameters          actionprotocol.Parameters `json:"parameters"`
	// Omission is used only by the internal semantic allocation preimage.
	// Every stored mapping is required to have an explicit positive revision.
	MappingRevision string `json:"mappingRevision,omitempty"`
}

type policyDocument struct {
	Version             string            `json:"version"`
	ServerID            string            `json:"serverId"`
	OwnerUserID         string            `json:"ownerUserId"`
	ConnectorInstanceID string            `json:"connectorInstanceId"`
	Revision            string            `json:"revision"`
	Current             []string          `json:"current"`
	Mappings            []mappingDocument `json:"mappings"`
}

func localHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func localID(s, prefix string) bool {
	return len(s) == len(prefix)+32 && s[:len(prefix)] == prefix && localHex(s[len(prefix):], 32)
}

func localRevision(s string) (uint64, bool) {
	if len(s) == 0 || len(s) > 19 || s[0] < '1' || s[0] > '9' {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil && n <= math.MaxInt64 && strconv.FormatUint(n, 10) == s
}

func semantic(m mappingDocument) string {
	m.MappingRevision = ""
	raw, _ := json.Marshal(m)
	return string(raw)
}

func displayAlias(container string) string {
	hash := sha256.Sum256([]byte(container))
	return "ctr_" + hex.EncodeToString(hash[:8])
}

func validMapping(m mappingDocument, revisionRequired bool) bool {
	_, policy := localRevision(m.PolicyRevision)
	_, capability := localRevision(m.CapabilityRevision)
	_, mapping := localRevision(m.MappingRevision)
	_, parameters := actionprotocol.ParameterDigest(m.Operation, m.Parameters)
	raw, err := json.Marshal(m)
	return localID(m.ServerID, "srv_") && localID(m.OwnerUserID, "usr_") && localID(m.ConnectorInstanceID, "agent_") &&
		localID(m.TargetID, "hlt_") && localHex(m.EngineGeneration, 32) && localHex(m.ContainerID, 64) && policy && capability &&
		(!revisionRequired || mapping) && parameters == nil && err == nil && len(raw) <= 1024
}

// policyShape bounds nesting before materialization. The canonical closed ASCII
// file has no escapes/whitespace or arbitrary string values; JSON decoding and
// canonical equality below also reject duplicate, case-aliased and unknown keys.
func policyShape(raw []byte) bool {
	if len(raw) == 0 || len(raw) > policyBytes {
		return false
	}
	for _, b := range raw {
		if b < 0x21 || b > 0x7e || b == '\\' {
			return false
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var visit func(int) bool
	visit = func(depth int) bool {
		token, err := d.Token()
		if err != nil {
			return false
		}
		kind, container := token.(json.Delim)
		if !container {
			return true
		}
		if depth >= 8 || (kind != '{' && kind != '[') {
			return false
		}
		count, seen := 0, make(map[string]bool)
		for d.More() {
			count++
			if count > 1032 {
				return false
			}
			if kind == '{' {
				key, err := d.Token()
				name, ok := key.(string)
				if err != nil || !ok || seen[name] || count > 16 {
					return false
				}
				seen[name] = true
			}
			if !visit(depth + 1) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && (kind == '{' && end == json.Delim('}') || kind == '[' && end == json.Delim(']'))
	}
	if !visit(0) {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}

func decodePolicy(raw []byte) (policyDocument, error) {
	var p policyDocument
	if !policyShape(raw) {
		return p, ErrPolicy
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&p) != nil || d.Decode(new(any)) != io.EOF || validatePolicy(p) != nil {
		return policyDocument{}, ErrPolicy
	}
	canonical, err := json.Marshal(p)
	if err != nil || !bytes.Equal(raw, canonical) {
		return policyDocument{}, ErrPolicy
	}
	return p, nil
}

func validatePolicy(p policyDocument) error {
	head, valid := localRevision(p.Revision)
	if !valid || p.Version != "observer-action-policy/v1" || !localID(p.ServerID, "srv_") ||
		!localID(p.OwnerUserID, "usr_") || !localID(p.ConnectorInstanceID, "agent_") || p.Current == nil || p.Mappings == nil ||
		len(p.Current) > 32 || len(p.Mappings) > 1032 {
		return ErrPolicy
	}
	byRevision := make(map[string]mappingDocument)
	bySemantic := make(map[string]bool)
	targetBinding := make(map[string]string)
	var previous uint64
	for _, m := range p.Mappings {
		n, ok := localRevision(m.MappingRevision)
		if !ok || n <= previous || n > head || !validMapping(m, true) || m.ServerID != p.ServerID || m.OwnerUserID != p.OwnerUserID || m.ConnectorInstanceID != p.ConnectorInstanceID {
			return ErrPolicy
		}
		previous = n
		tuple := semantic(m)
		if bySemantic[tuple] {
			return ErrPolicy
		}
		bySemantic[tuple] = true
		physical := m.EngineGeneration + ":" + m.ContainerID
		if old, exists := targetBinding[m.TargetID]; exists && old != physical {
			return ErrPolicy
		}
		targetBinding[m.TargetID] = physical
		byRevision[m.MappingRevision] = m
	}
	previous = 0
	pairs, aliases, physicalTargets := make(map[string]bool), make(map[string]string), make(map[string]string)
	for _, revision := range p.Current {
		n, valid := localRevision(revision)
		m, exists := byRevision[revision]
		if !valid || n <= previous || !exists {
			return ErrPolicy
		}
		previous = n
		pair := m.TargetID + ":" + string(m.Operation)
		physical := m.EngineGeneration + ":" + m.ContainerID
		alias := displayAlias(m.ContainerID)
		if pairs[pair] || (aliases[alias] != "" && aliases[alias] != m.TargetID) || (physicalTargets[physical] != "" && physicalTargets[physical] != m.TargetID) {
			return ErrPolicy
		}
		pairs[pair], aliases[alias], physicalTargets[physical] = true, m.TargetID, m.TargetID
	}
	if len(p.Mappings)-len(p.Current) > 1000 {
		return ErrPolicy
	}
	return nil
}

func mappingForKey(k actioncore.Key) (mappingDocument, error) {
	if _, err := actioncore.CanonicalJournalMetadata(k); err != nil {
		return mappingDocument{}, ErrPolicy
	}
	r := k.Metadata.Request
	m := mappingDocument{ServerID: r.ServerID, OwnerUserID: r.OwnerID, ConnectorInstanceID: r.InstallationID,
		TargetID: r.TargetID, EngineGeneration: k.Metadata.EngineGeneration, ContainerID: k.Metadata.ContainerID,
		PolicyRevision: strconv.FormatUint(r.PolicyRevision, 10), CapabilityRevision: strconv.FormatUint(r.CapabilityRevision, 10),
		Operation: r.Operation, Parameters: actionprotocol.Parameters{FormatVersion: 1, StopGraceSeconds: actionprotocol.Grace{Seconds: r.GraceSeconds}}}
	if r.Operation == actioncore.Start {
		m.Parameters.StopGraceSeconds = actionprotocol.Grace{Null: true}
	}
	if r.OwnerID != r.ActorID || !validMapping(m, false) {
		return mappingDocument{}, ErrPolicy
	}
	return m, nil
}
