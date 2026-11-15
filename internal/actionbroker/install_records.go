package actionbroker

import (
	"encoding/json"
	"strconv"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionengine"
	"github.com/braidenm/home-lab-observer/internal/actionjournal"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

// InitialRecords encodes one explicitly selected physical target. The caller
// must obtain the owner binding and confirm the pinned Engine witness locally.
// This function cannot discover targets, publish policy or change authority.
func InitialRecords(binding actionjournal.Binding, target, generation, container string, witness actionengine.SocketWitness, grace uint8) (policy, engine []byte, err error) {
	p := policyDocument{
		Version: "observer-action-policy/v1", ServerID: binding.ServerID,
		OwnerUserID: binding.OwnerID, ConnectorInstanceID: binding.InstallationID, Revision: "3",
	}
	for i, op := range []actioncore.Operation{actioncore.Start, actioncore.Stop, actioncore.Restart} {
		revision := strconv.Itoa(i + 1)
		params := actionprotocol.Parameters{FormatVersion: 1, StopGraceSeconds: actionprotocol.Grace{Seconds: grace}}
		if op == actioncore.Start {
			params.StopGraceSeconds = actionprotocol.Grace{Null: true}
		}
		p.Current = append(p.Current, revision)
		p.Mappings = append(p.Mappings, mappingDocument{
			ServerID: p.ServerID, OwnerUserID: p.OwnerUserID, ConnectorInstanceID: p.ConnectorInstanceID,
			TargetID: target, EngineGeneration: generation, ContainerID: container,
			PolicyRevision: "1", CapabilityRevision: "1", Operation: op, Parameters: params, MappingRevision: revision,
		})
	}
	if validateEnginePolicy(p, generation) != nil {
		return nil, nil, ErrPolicy
	}
	policy, err = json.Marshal(p)
	if err != nil {
		return nil, nil, ErrPolicy
	}
	engine, err = json.Marshal(struct {
		Version    string `json:"version"`
		Generation string `json:"engine_generation"`
		Device     string `json:"socket_device"`
		Inode      string `json:"socket_inode"`
		Group      string `json:"socket_group_id"`
	}{"observer-action-engine/v1", generation, strconv.FormatUint(witness.Device, 10),
		strconv.FormatUint(witness.Inode, 10), strconv.FormatUint(uint64(witness.GroupID), 10)})
	if err != nil {
		return nil, nil, ErrPolicy
	}
	if _, err := decodeEngineApproval(engine); err != nil {
		return nil, nil, ErrPolicy
	}
	return policy, engine, nil
}
