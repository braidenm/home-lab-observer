package actionbroker

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

func engineDocument(generation string) []byte {
	return []byte(`{"version":"observer-action-engine/v1","engine_generation":"` + generation + `","socket_device":"0","socket_inode":"18446744073709551615","socket_group_id":"4294967295"}`)
}

func TestEngineApprovalComesOnlyFromClosedProtectedSourceValues(t *testing.T) {
	raw := engineDocument(strings.Repeat("a", 32))
	a, err := decodeEngineApproval(raw)
	if err != nil || a.generation != strings.Repeat("a", 32) || a.witness.Device != 0 || a.witness.Inode != ^uint64(0) || a.witness.GroupID != ^uint32(0) {
		t.Fatal("canonical explicit witness range changed")
	}
	for _, bad := range []string{
		strings.Replace(string(raw), `"socket_inode":"18446744073709551615"`, `"socket_inode":"0"`, 1),
		strings.Replace(string(raw), `"socket_device":"0"`, `"socket_device":"00"`, 1),
		strings.Replace(string(raw), `"socket_group_id":"4294967295"`, `"socket_group_id":"4294967296"`, 1),
		strings.Replace(string(raw), `"socket_device":"0"`, `"socket_device":0`, 1),
		strings.Replace(string(raw), `"version":`, `"Version":`, 1),
		strings.Replace(string(raw), `"version":`, `"unknown":0,"version":`, 1),
		strings.Replace(string(raw), `"socket_device":`, `"socket_device":"1","socket_device":`, 1),
		string(raw) + `{}`,
		string(raw) + strings.Repeat(" ", engineDocumentLimit),
	} {
		if _, err := decodeEngineApproval([]byte(bad)); err == nil {
			t.Fatal("unsafe witness source admitted")
		}
	}
}

func TestPolicyReaderPreservesRetainedMappingsWithoutAdoptingOldEngine(t *testing.T) {
	m := policyMapping("5", "6")
	p := fixedPolicy(m)
	r := policyReceipt(t, m)
	resolved, err := resolveReceipt(p, r, true)
	if err != nil {
		t.Fatal("reference")
	}
	if validateEnginePolicy(p, m.EngineGeneration) != nil || validateMappingReferences(p, []actioncore.Key{resolved.key}) != nil {
		t.Fatal("current immutable mapping")
	}
	if validateEnginePolicy(p, strings.Repeat("e", 32)) == nil {
		t.Fatal("current approval accepted another Engine generation")
	}
	p.Current = []string{}
	before, _ := json.Marshal(p)
	if validateEnginePolicy(p, strings.Repeat("e", 32)) != nil || validateMappingReferences(p, []actioncore.Key{resolved.key}) != nil {
		t.Fatal("retained old mapping removed or adopted")
	}
	after, _ := json.Marshal(p)
	if !bytes.Equal(before, after) {
		t.Fatal("reader rewrote retained policy history")
	}
	if requireEngineGeneration(resolved.key, strings.Repeat("e", 32)) == nil {
		t.Fatal("old retained identity adopted into current engine")
	}
	p.Mappings = []mappingDocument{}
	if validateMappingReferences(p, []actioncore.Key{resolved.key}) == nil {
		t.Fatal("referenced unknown mapping disappeared")
	}
}
