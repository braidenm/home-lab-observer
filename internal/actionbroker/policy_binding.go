package actionbroker

import (
	"strconv"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

type resolvedBinding struct {
	key             actioncore.Key
	binding         actioncore.Binding
	mappingRevision string
	remoteDigest    [32]byte
}

// resolveReceipt compares a freshly authenticated complete remote receipt with
// an exact protected mapping. Neither decoding nor this local lookup establishes
// current remote owner authority. Retained lookup can never grant execution.
func resolveReceipt(p policyDocument, r actionprotocol.RequestReceipt, current bool) (resolvedBinding, error) {
	if validatePolicy(p) != nil || actionprotocol.ValidateRequest(r) != nil ||
		r.ServerID != p.ServerID || r.OwnerUserID != p.OwnerUserID || r.ConnectorInstanceID != p.ConnectorInstanceID {
		return resolvedBinding{}, ErrPolicy
	}
	allowed := make(map[string]bool)
	for _, revision := range p.Current {
		allowed[revision] = true
	}
	var selected *mappingDocument
	for index := range p.Mappings {
		m := &p.Mappings[index]
		if m.TargetID != r.TargetID || m.Operation != r.Operation || m.PolicyRevision != r.PolicyRevision ||
			m.CapabilityRevision != r.CapabilityRevision || m.Parameters != r.Parameters || (current && !allowed[m.MappingRevision]) {
			continue
		}
		if selected != nil {
			return resolvedBinding{}, ErrPolicy
		}
		selected = m
	}
	if selected == nil || r.TargetKey != displayAlias(selected.ContainerID) {
		return resolvedBinding{}, ErrPolicy
	}
	created, _ := actionprotocol.ParseHTTPTime(r.RequestedAt)
	expires, _ := actionprotocol.ParseHTTPTime(r.ExpiresAt)
	policy, _ := strconv.ParseUint(r.PolicyRevision, 10, 64)
	capability, _ := strconv.ParseUint(r.CapabilityRevision, 10, 64)
	request := actioncore.Request{ActionID: r.ActionID, ServerID: r.ServerID, InstallationID: r.ConnectorInstanceID,
		OwnerID: r.OwnerUserID, ActorID: r.ActorUserID, TargetID: r.TargetID, Operation: r.Operation,
		GraceSeconds: r.Parameters.StopGraceSeconds.Seconds, PolicyRevision: policy, CapabilityRevision: capability,
		CreatedAt: created, ExpiresAt: expires}
	binding := actioncore.Binding{ServerID: r.ServerID, InstallationID: r.ConnectorInstanceID, OwnerID: r.OwnerUserID,
		TargetID: r.TargetID, EngineGeneration: selected.EngineGeneration, ContainerID: selected.ContainerID,
		PolicyRevision: policy, CapabilityRevision: capability, GraceSeconds: request.GraceSeconds,
		Operations: []actioncore.Operation{r.Operation}}
	key, err := actioncore.NewJournalKey(request, binding)
	if err != nil {
		return resolvedBinding{}, ErrPolicy
	}
	digest, err := actionprotocol.ReceiptDigest(r)
	if err != nil {
		return resolvedBinding{}, ErrPolicy
	}
	return resolvedBinding{key: key, binding: binding, mappingRevision: selected.MappingRevision, remoteDigest: digest}, nil
}
