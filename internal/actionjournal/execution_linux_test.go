//go:build linux

package actionjournal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionbudget"
	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

func v1ExecutionFixture(t *testing.T) (*Journal, string, actioncore.Key, [32]byte, context.Context) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	base := key(t, 1, 1)
	request := base.Metadata.Request
	request.ActorID = request.OwnerID
	request.Operation, request.GraceSeconds = actioncore.Restart, 1
	request.CreatedAt, request.ExpiresAt = now.Add(-time.Second), now.Add(299*time.Second)
	local := actioncore.Binding{ServerID: request.ServerID, InstallationID: request.InstallationID, OwnerID: request.OwnerID,
		TargetID: request.TargetID, EngineGeneration: base.Metadata.EngineGeneration, ContainerID: base.Metadata.ContainerID,
		PolicyRevision: 1, CapabilityRevision: 1, GraceSeconds: 1, Operations: []actioncore.Operation{actioncore.Restart}}
	k, err := actioncore.NewJournalKey(request, local)
	if err != nil {
		t.Fatal("paired execution key")
	}
	dir := t.TempDir()
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("private execution fixture")
	}
	binding := Binding{ServerID: k.Metadata.Request.ServerID, InstallationID: k.Metadata.Request.InstallationID, OwnerID: k.Metadata.Request.OwnerID}
	j, err := Provision(context.Background(), dir, binding)
	if err != nil {
		t.Fatal("actual execution store")
	}
	t.Cleanup(func() { j.Close() })
	r := k.Metadata.Request
	parameters := actionprotocol.Parameters{FormatVersion: 1, StopGraceSeconds: actionprotocol.Grace{Seconds: r.GraceSeconds}}
	hash, _ := actionprotocol.ParameterDigest(r.Operation, parameters)
	alias := sha256.Sum256([]byte(k.Metadata.ContainerID))
	receipt := actionprotocol.RequestReceipt{ActionID: r.ActionID, ServerID: r.ServerID, OwnerUserID: r.OwnerID, ActorUserID: r.ActorID,
		ConnectorInstanceID: r.InstallationID, Operation: r.Operation, TargetID: r.TargetID, TargetKey: "ctr_" + hex.EncodeToString(alias[:8]),
		IdempotencyKeyHash: strings.Repeat("a", 64), PolicyRevision: strconv.FormatUint(r.PolicyRevision, 10), CapabilityRevision: strconv.FormatUint(r.CapabilityRevision, 10),
		Parameters: parameters, ParameterSHA256: hash, RequestedAt: r.CreatedAt.Format(actionprotocol.HTTPTimeLayout), ExpiresAt: r.ExpiresAt.Format(actionprotocol.HTTPTimeLayout), CorrelationID: "synthetic-execution-commit"}
	digest, err := actionprotocol.ReceiptDigest(receipt)
	if err != nil {
		t.Fatal("full synthetic receipt")
	}
	clock, err := actionbudget.NewClock()
	if err != nil {
		t.Fatal("native clock")
	}
	send, _ := clock.Capture()
	server := time.Now().UTC()
	recv, _ := clock.Capture()
	readiness, err := clock.ObserveServerTime(server, send, recv)
	if err != nil {
		t.Fatal("timing bracket")
	}
	conversation, err := clock.Begin(readiness)
	if err != nil {
		t.Fatal("timing begin")
	}
	claimed := time.Now().UTC().Truncate(time.Microsecond)
	claim := actionprotocol.Claim{Request: receipt, ExecutionLeaseID: "hle_" + strings.Repeat("b", 32), ClaimCredentialID: "hlac_" + strings.Repeat("c", 32),
		ClaimCredentialRevision: "1", ClaimedAt: claimed.Format(actionprotocol.HTTPTimeLayout), LeaseExpiresAt: claimed.Add(120 * time.Second).Format(actionprotocol.HTTPTimeLayout), CorrelationID: "synthetic-claim"}
	execution, err := conversation.AcceptClaim(claim, 0, k.Metadata.ContainerID)
	if err != nil {
		t.Fatal("synthetic native execution budget, not permission")
	}
	return j, dir, k, digest, execution.Context(context.Background())
}

func TestV1ExecutionBindingRejectsMissingCanceledAndChangedIdentityWithoutMutation(t *testing.T) {
	j, _, key, digest, execution := v1ExecutionFixture(t)
	bound, err := j.BindExecution(key, digest)
	if err != nil {
		t.Fatal("explicit guarded execution binding")
	}
	canceled, cancel := context.WithCancel(execution)
	cancel()
	for _, invalid := range []context.Context{nil, context.Background(), canceled} {
		if _, err := bound.Acquire(invalid, key); !errors.Is(err, ErrRecovery) || j.failed {
			t.Fatal("invalid execution kind mutated or poisoned")
		}
	}
	s, err := j.AdmissionSnapshot(context.Background())
	if err != nil || len(s.Records) != 0 {
		t.Fatal("refused scope inserted admission")
	}
	r := key.Metadata.Request
	r.ActionID = "hla_" + strings.Repeat("9", 32)
	local := actioncore.Binding{ServerID: r.ServerID, InstallationID: r.InstallationID, OwnerID: r.OwnerID,
		TargetID: r.TargetID, EngineGeneration: key.Metadata.EngineGeneration, ContainerID: key.Metadata.ContainerID,
		PolicyRevision: r.PolicyRevision, CapabilityRevision: r.CapabilityRevision, GraceSeconds: r.GraceSeconds,
		Operations: []actioncore.Operation{r.Operation}}
	other, err := actioncore.NewJournalKey(r, local)
	if err != nil {
		t.Fatal("valid different key")
	}
	if _, err := bound.Acquire(execution, other); !errors.Is(err, actioncore.ErrIdentityConflict) || j.failed {
		t.Fatal("invocation key changed")
	}
	changed := digest
	changed[0] ^= 1
	wrong, err := j.BindExecution(key, changed)
	if err != nil {
		t.Fatal("structural binding is not authority")
	}
	if _, err := wrong.Acquire(execution, key); !errors.Is(err, ErrRecovery) || j.failed {
		t.Fatal("different receipt timing correlation admitted")
	}
	foreign := key
	foreign.Metadata.Request.ActorID = "usr_" + strings.Repeat("9", 32)
	if _, err := j.BindExecution(foreign, digest); err == nil {
		t.Fatal("owner-only execution binding changed")
	}
	a, err := bound.Acquire(execution, key)
	if err != nil {
		t.Fatal("guarded admission")
	}
	defer a.Close()
	if a.Intent(context.Background()) == nil || j.failed {
		t.Fatal("unguarded INTENT accepted or poisoned")
	}
	if a.Intent(execution) != nil {
		t.Fatal("guarded INTENT")
	}
	if a.Finish(context.Background(), actioncore.Restarted) == nil || j.failed {
		t.Fatal("missing execution context authorized terminal")
	}
	if a.Finish(execution, actioncore.Restarted) != nil {
		t.Fatal("guarded terminal")
	}
}

func TestV1ExecutionChecksActualCommitAndPreservesReopenedTruth(t *testing.T) {
	for _, stage := range []string{"admission", "intent", "terminal"} {
		for _, after := range []bool{false, true} {
			name := stage + map[bool]string{false: "-before", true: "-after"}[after]
			t.Run(name, func(t *testing.T) {
				j, dir, key, digest, execution := v1ExecutionFixture(t)
				bound, err := j.BindExecution(key, digest)
				if err != nil {
					t.Fatal("guarded binding")
				}
				var a actioncore.Attempt
				if stage != "admission" {
					a, err = bound.Acquire(execution, key)
					if err != nil {
						t.Fatal("baseline admission")
					}
				}
				if stage == "terminal" && a.Intent(execution) != nil {
					t.Fatal("baseline INTENT")
				}
				bounded, cancel := context.WithCancel(execution)
				defer cancel()
				called := false
				fault := func() error { called = true; cancel(); return nil }
				if after {
					j.afterCommit = fault
				} else {
					j.beforeCommit = fault
				}
				switch stage {
				case "admission":
					a, err = bound.Acquire(bounded, key)
				case "intent":
					err = a.Intent(bounded)
				case "terminal":
					err = a.Finish(bounded, actioncore.Restarted)
				}
				if !called || !errors.Is(err, ErrRecovery) || !j.failed {
					t.Fatal("late actual commit reported success or left usable store")
				}
				if a != nil {
					a.Close()
				}
				j.Close()
				binding := Binding{ServerID: key.Metadata.Request.ServerID, InstallationID: key.Metadata.Request.InstallationID, OwnerID: key.Metadata.Request.OwnerID}
				reopened, err := OpenExisting(context.Background(), dir, binding)
				if err != nil {
					t.Fatal("retained actual store")
				}
				defer reopened.Close()
				snapshot, err := reopened.AdmissionSnapshot(context.Background())
				if err != nil {
					t.Fatal("read-only retained truth")
				}
				if stage == "admission" && !after {
					if len(snapshot.Records) != 0 {
						t.Fatal("refused admission committed")
					}
					return
				}
				if len(snapshot.Records) != 1 {
					t.Fatal("actual commit missing")
				}
				want := actioncore.Code("")
				if stage == "intent" && after || stage == "terminal" {
					want = actioncore.OutcomeUnknown
				}
				if stage == "terminal" && after {
					want = actioncore.Restarted
				}
				if snapshot.Records[0].Code != want || snapshot.Records[0].Active {
					t.Fatal("retained truth fabricated outcome or lost actual commit")
				}
			})
		}
	}
}
