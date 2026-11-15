//go:build linux

package connectedinstall

import (
	"bytes"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/actionengine"
	"github.com/braidenm/home-lab-observer/internal/actionjournal"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

func TestActionConfigSharesBindingButKeepsEngineDetailsOutOfWorker(t *testing.T) {
	inputs := actionConfigInputs{
		binding: actionjournal.Binding{ServerID: "srv_" + strings.Repeat("1", 32),
			OwnerID: "usr_" + strings.Repeat("2", 32), InstallationID: "agent_" + strings.Repeat("3", 32)},
		target: "hlt_" + strings.Repeat("4", 32), generation: strings.Repeat("5", 32),
		container: strings.Repeat("6", 64), grace: 10,
	}
	witness := actionengine.SocketWitness{Device: 1, Inode: 2, GroupID: 998}
	records, err := encodeActionConfig(inputs, witness)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{inputs.binding.ServerID, inputs.binding.OwnerID, inputs.binding.InstallationID} {
		if !bytes.Contains(records.worker, []byte(name)) || !bytes.Contains(records.policy, []byte(name)) {
			t.Fatal("worker and broker records disagree on binding")
		}
	}
	if !bytes.Contains(records.worker, []byte(connectedprofile.Origin)) ||
		bytes.Contains(records.worker, []byte(inputs.container)) || bytes.Contains(records.worker, []byte(inputs.generation)) ||
		bytes.Contains(records.policy, []byte(connectedprofile.Origin)) {
		t.Fatal("role records were not separated")
	}
	inputs.binding.ServerID = "invalid"
	if _, err := encodeActionConfig(inputs, witness); err == nil {
		t.Fatal("invalid server binding accepted")
	}
}
