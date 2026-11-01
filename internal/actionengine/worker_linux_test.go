//go:build linux

package actionengine

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionjournal"
)

type syntheticAuthority struct{ checks int }

func (a *syntheticAuthority) Check(context.Context, actioncore.Request, actioncore.Binding) error {
	a.checks++
	return nil
}

func workerInput(op actioncore.Operation) (actioncore.Request, actioncore.Binding, actioncore.Delivery, time.Time) {
	now := time.Now().UTC()
	r := actioncore.Request{
		ActionID: "hla_" + strings.Repeat("1", 32), ServerID: "srv_" + strings.Repeat("2", 32),
		InstallationID: "agent_" + strings.Repeat("3", 32), OwnerID: "usr_" + strings.Repeat("4", 32),
		ActorID: "usr_" + strings.Repeat("5", 32), TargetID: "hlt_" + strings.Repeat("6", 32),
		Operation: op, PolicyRevision: 1, CapabilityRevision: 1, CreatedAt: now.Add(-time.Second), ExpiresAt: now.Add(4 * time.Minute),
	}
	if op != actioncore.Start {
		r.GraceSeconds = 1
	}
	b := actioncore.Binding{ServerID: r.ServerID, InstallationID: r.InstallationID, OwnerID: r.OwnerID,
		TargetID: r.TargetID, EngineGeneration: strings.Repeat("7", 32), ContainerID: fullID,
		PolicyRevision: 1, CapabilityRevision: 1, GraceSeconds: r.GraceSeconds, Operations: []actioncore.Operation{op}}
	return r, b, actioncore.Delivery{StartedAt: now, ReceivedAt: now, LeaseDeadline: now.Add(2 * time.Minute)}, now
}

func TestWorkerRealJournalAndUnixEngineRetainedReplay(t *testing.T) {
	for _, tc := range []struct {
		op        actioncore.Operation
		state     actioncore.State
		lost      bool
		want      actioncore.Code
		mutations int32
	}{
		{actioncore.Start, actioncore.Created, false, actioncore.Started, 1},
		{actioncore.Stop, actioncore.Running, false, actioncore.Stopped, 1},
		{actioncore.Restart, actioncore.Running, false, actioncore.Restarted, 1},
		{actioncore.Start, actioncore.Running, false, actioncore.AlreadyRunning, 0},
		{actioncore.Stop, actioncore.Exited, false, actioncore.AlreadyStopped, 0},
		{actioncore.Restart, actioncore.Exited, false, actioncore.StateConflict, 0},
		{actioncore.Restart, actioncore.Running, true, actioncore.OutcomeUnknown, 1},
	} {
		t.Run(string(tc.op)+"/"+string(tc.want), func(t *testing.T) {
			var inspections, mutations atomic.Int32
			e := unixFixture(t, versionJSON, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					inspections.Add(1)
					fmt.Fprintf(w, `{"Id":%q,"State":{"Status":%q}}`, fullID, tc.state)
					return
				}
				mutations.Add(1)
				if tc.lost {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err == nil {
						conn.Close()
					}
					return
				}
				w.WriteHeader(204)
			})
			a := openFixture(t, e)
			r, b, delivery, now := workerInput(tc.op)
			dir := t.TempDir()
			os.Chmod(dir, 0700)
			binding := actionjournal.Binding{ServerID: b.ServerID, InstallationID: b.InstallationID, OwnerID: b.OwnerID}
			j, err := actionjournal.Provision(context.Background(), dir, binding)
			if err != nil {
				t.Fatal("journal fixture")
			}
			authority := new(syntheticAuthority)
			worker := actioncore.New(authority, j, a, func() time.Time { return now })
			if got := worker.Execute(context.Background(), r, b, delivery); got != tc.want {
				t.Fatalf("worker result=%s", got)
			}
			j.Close()
			j, err = actionjournal.OpenExisting(context.Background(), dir, binding)
			if err != nil {
				t.Fatal("journal reopen")
			}
			defer j.Close()
			worker = actioncore.New(authority, j, a, func() time.Time { return now })
			if worker.Execute(context.Background(), r, b, delivery) != tc.want || mutations.Load() != tc.mutations || inspections.Load() != 1 || authority.checks != 3 {
				t.Fatal("retained outcome repeated Engine I/O or skipped fresh authority")
			}
		})
	}
}

func TestUnavailableRealJournalPreventsEngineIO(t *testing.T) {
	e := unixFixture(t, versionJSON, func(http.ResponseWriter, *http.Request) { t.Error("unavailable journal performed Engine I/O") })
	a := openFixture(t, e)
	r, b, delivery, now := workerInput(actioncore.Start)
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	j, err := actionjournal.Provision(context.Background(), dir, actionjournal.Binding{ServerID: b.ServerID, InstallationID: b.InstallationID, OwnerID: b.OwnerID})
	if err != nil {
		t.Fatal("journal fixture")
	}
	j.Close()
	worker := actioncore.New(new(syntheticAuthority), j, a, func() time.Time { return now })
	if worker.Execute(context.Background(), r, b, delivery) != actioncore.Unavailable {
		t.Fatal("closed journal accepted work")
	}
}
