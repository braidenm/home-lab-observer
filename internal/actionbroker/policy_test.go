package actionbroker

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

func policyMapping(target, container string) mappingDocument {
	return mappingDocument{ServerID: "srv_" + strings.Repeat("1", 32), OwnerUserID: "usr_" + strings.Repeat("2", 32),
		ConnectorInstanceID: "agent_" + strings.Repeat("3", 32), TargetID: "hlt_" + strings.Repeat(target, 32),
		EngineGeneration: strings.Repeat("4", 32), ContainerID: strings.Repeat(container, 64), PolicyRevision: "1", CapabilityRevision: "1",
		Operation: actioncore.Restart, Parameters: actionprotocol.Parameters{FormatVersion: 1, StopGraceSeconds: actionprotocol.Grace{Seconds: 1}}}
}

// Fixture construction only: the first runtime has NO policy writer/update API.
func fixedPolicy(mappings ...mappingDocument) policyDocument {
	p := policyDocument{Version: "observer-action-policy/v1", ServerID: "srv_" + strings.Repeat("1", 32),
		OwnerUserID: "usr_" + strings.Repeat("2", 32), ConnectorInstanceID: "agent_" + strings.Repeat("3", 32),
		Revision: strconv.Itoa(max(1, len(mappings))), Current: []string{}, Mappings: []mappingDocument{}}
	for index, m := range mappings {
		m.MappingRevision = strconv.Itoa(index + 1)
		p.Mappings = append(p.Mappings, m)
		p.Current = append(p.Current, m.MappingRevision)
	}
	return p
}

func policyReceipt(t *testing.T, m mappingDocument) actionprotocol.RequestReceipt {
	t.Helper()
	hash, err := actionprotocol.ParameterDigest(m.Operation, m.Parameters)
	if err != nil {
		t.Fatal("fixture parameters")
	}
	now := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Second)
	return actionprotocol.RequestReceipt{ActionID: "hla_" + strings.Repeat("a", 32), ServerID: m.ServerID, OwnerUserID: m.OwnerUserID,
		ActorUserID: m.OwnerUserID, ConnectorInstanceID: m.ConnectorInstanceID, Operation: m.Operation, TargetID: m.TargetID,
		TargetKey: displayAlias(m.ContainerID), IdempotencyKeyHash: strings.Repeat("b", 64), PolicyRevision: m.PolicyRevision,
		CapabilityRevision: m.CapabilityRevision, Parameters: m.Parameters, ParameterSHA256: hash,
		RequestedAt: now.Format(actionprotocol.HTTPTimeLayout), ExpiresAt: now.Add(300 * time.Second).Format(actionprotocol.HTTPTimeLayout),
		CorrelationID: "synthetic-fixed-policy"}
}

func TestFixedPolicyRejectsPhysicalAliasesAndRetainedRebinding(t *testing.T) {
	m := policyMapping("5", "6")
	p := fixedPolicy(m)
	raw, _ := json.Marshal(p)
	if _, err := decodePolicy(raw); err != nil {
		t.Fatal("fixed policy")
	}
	alias := m
	alias.TargetID = "hlt_" + strings.Repeat("7", 32)
	if validatePolicy(fixedPolicy(m, alias)) == nil {
		t.Fatal("same native resource received two opaque targets")
	}
	replaced := m
	replaced.ContainerID = strings.Repeat("8", 64)
	retained := fixedPolicy(m, replaced)
	retained.Current = []string{"1"}
	if validatePolicy(retained) == nil {
		t.Fatal("retained opaque target rebound to replacement resource")
	}
	for _, invalid := range []string{string(raw) + `{}`, strings.Replace(string(raw), `"version":`, `"extra":1,"version":`, 1),
		strings.Replace(string(raw), `"version":`, `"Version":`, 1)} {
		if _, err := decodePolicy([]byte(invalid)); err == nil {
			t.Fatal("noncanonical/unknown policy admitted")
		}
	}
}

func TestFixedPolicyKeepsFullReceiptDistinctFromPrivateJournalFingerprint(t *testing.T) {
	m := policyMapping("5", "6")
	p := fixedPolicy(m)
	r := policyReceipt(t, m)
	first, err := resolveReceipt(p, r, true)
	if err != nil || validateMappingReferences(p, []actioncore.Key{first.key}) != nil {
		t.Fatal("fixed native mapping")
	}
	r.CorrelationID = "different-remote-correlation"
	second, err := resolveReceipt(p, r, true)
	if err != nil || first.key.Fingerprint != second.key.Fingerprint || first.remoteDigest == second.remoteDigest {
		t.Fatal("private metadata was mistaken for full-wire identity")
	}
	r.TargetKey = "ctr_0000000000000000"
	if _, err := resolveReceipt(p, r, true); err == nil {
		t.Fatal("display alias substituted for protected mapping")
	}
	if validateMappingReferences(fixedPolicy(), []actioncore.Key{first.key}) == nil {
		t.Fatal("missing retained mapping adopted")
	}
}
