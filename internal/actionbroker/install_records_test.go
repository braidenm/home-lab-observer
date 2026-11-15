package actionbroker

import (
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionengine"
	"github.com/braidenm/home-lab-observer/internal/actionjournal"
)

func TestInitialRecordsMatchBrokerReaders(t *testing.T) {
	binding := actionjournal.Binding{ServerID: "srv_" + strings.Repeat("1", 32),
		InstallationID: "agent_" + strings.Repeat("2", 32), OwnerID: "usr_" + strings.Repeat("3", 32)}
	target := "hlt_" + strings.Repeat("4", 32)
	generation, container := strings.Repeat("a", 32), strings.Repeat("b", 64)
	witness := actionengine.SocketWitness{Device: 1, Inode: 2, GroupID: 998}
	raw, engine, err := InitialRecords(binding, target, generation, container, witness, 10)
	if err != nil {
		t.Fatal(err)
	}
	p, err := decodePolicy(raw)
	e, engineErr := decodeEngineApproval(engine)
	if err != nil || engineErr != nil || e.witness != witness || len(p.Current) != 3 || len(p.Mappings) != 3 ||
		validateEnginePolicy(p, e.generation) != nil {
		t.Fatal("initial records rejected by runtime")
	}
	for i, op := range []actioncore.Operation{actioncore.Start, actioncore.Stop, actioncore.Restart} {
		m := p.Mappings[i]
		if m.Operation != op || m.TargetID != target || m.ContainerID != container ||
			m.Parameters.StopGraceSeconds.Null != (op == actioncore.Start) ||
			(op != actioncore.Start && m.Parameters.StopGraceSeconds.Seconds != 10) {
			t.Fatal("unexpected approval")
		}
	}
	for _, bad := range []string{"short-id", "container-name", strings.Repeat("g", 64)} {
		if _, _, err := InitialRecords(binding, target, generation, bad, witness, 10); err == nil {
			t.Fatal("invalid physical ID accepted")
		}
	}
	if _, _, err := InitialRecords(binding, target, generation, container, witness, 0); err == nil {
		t.Fatal("invalid stop grace accepted")
	}
}
