//go:build linux

package actionengine

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionjournal"
)

// Test-only sequencing decorators: real Engine/journal delegates are unchanged.
// Authority remains synthetic; it is not a remote permission implementation.
type raceTrace struct{ steps []string }

func (r *raceTrace) add(step string) {
	if len(r.steps) >= 32 {
		panic("race trace bound")
	}
	r.steps = append(r.steps, step)
}

type raceAuthority struct {
	trace    *raceTrace
	checks   int
	verify   func(context.Context) error
	external func(context.Context) error
}

func (a *raceAuthority) Check(ctx context.Context, _ actioncore.Request, _ actioncore.Binding) error {
	a.checks++
	if a.checks > 2 {
		return ErrUnavailable
	}
	a.trace.add("authority")
	if err := a.verify(ctx); err != nil {
		return err
	}
	if a.checks == 2 {
		a.trace.add("external")
		if err := a.external(ctx); err != nil {
			return err
		}
		a.trace.add("external-ack")
	}
	return nil
}

type raceEngine struct {
	actioncore.Engine
	trace                  *raceTrace
	inspections, mutations int
	mutationError          error
}

func (e *raceEngine) Inspect(ctx context.Context, id string) (actioncore.State, error) {
	e.inspections++
	e.trace.add("inspect")
	return e.Engine.Inspect(ctx, id)
}
func (e *raceEngine) Start(ctx context.Context, id string) error {
	e.mutations++
	e.trace.add("dispatch")
	e.mutationError = e.Engine.Start(ctx, id)
	return e.mutationError
}
func (e *raceEngine) Stop(ctx context.Context, id string, grace uint8) error {
	e.mutations++
	e.trace.add("dispatch")
	e.mutationError = e.Engine.Stop(ctx, id, grace)
	return e.mutationError
}
func (e *raceEngine) Restart(ctx context.Context, id string, grace uint8) error {
	e.mutations++
	e.trace.add("dispatch")
	e.mutationError = e.Engine.Restart(ctx, id, grace)
	return e.mutationError
}

type raceJournal struct {
	actioncore.Journal
	trace *raceTrace
}

func (j *raceJournal) Acquire(ctx context.Context, key actioncore.Key) (actioncore.Attempt, error) {
	a, err := j.Journal.Acquire(ctx, key)
	if err != nil {
		return nil, err
	}
	return &raceAttempt{Attempt: a, trace: j.trace}, nil
}

type raceAttempt struct {
	actioncore.Attempt
	trace *raceTrace
}

func (a *raceAttempt) Intent(ctx context.Context) error {
	if err := a.Attempt.Intent(ctx); err != nil {
		return err
	}
	a.trace.add("intent-persisted")
	return nil
}
func (a *raceAttempt) Finish(ctx context.Context, code actioncore.Code) error {
	if err := a.Attempt.Finish(ctx, code); err != nil {
		return err
	}
	a.trace.add("finish")
	return nil
}

// Observe only status codes, never consume/replace a response or its private body.
// All requests still traverse the adapter's actual pinned HTTP transport.
type raceHTTPObserver struct {
	base     http.RoundTripper
	statuses []int
}

func (o *raceHTTPObserver) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := o.base.RoundTrip(r)
	if r.Method == http.MethodPost && response != nil {
		if len(o.statuses) >= 4 {
			panic("race status bound")
		}
		o.statuses = append(o.statuses, response.StatusCode)
	}
	return response, err
}

func raceRequest(r actioncore.Request, b actioncore.Binding, op actioncore.Operation, id int) (actioncore.Request, actioncore.Delivery) {
	now := time.Now().UTC()
	r.ActionID = fmt.Sprintf("hla_%032x", id)
	r.Operation, r.GraceSeconds = op, 0
	if op != actioncore.Start {
		r.GraceSeconds = b.GraceSeconds
	}
	r.CreatedAt, r.ExpiresAt = now.Add(-time.Second), now.Add(4*time.Minute)
	return r, actioncore.Delivery{StartedAt: now, ReceivedAt: now, LeaseDeadline: now.Add(2 * time.Minute)}
}

func runOwnedStateRaces(t *testing.T, owned *ownedNativeEngine, request actioncore.Request, binding actioncore.Binding, dir string, journal *actionjournal.Journal) {
	t.Helper()
	owned.test = t
	originalTransport := owned.client.Transport
	responseObserver := &raceHTTPObserver{base: originalTransport}
	owned.client.Transport = responseObserver
	defer func() { owned.client.Transport = originalTransport }()
	// A second actual adapter represents an independently operating local actor.
	externalAdapter := openFixture(t, owned.endpoint)
	external := &ownedNativeEngine{Engine: externalAdapter, id: owned.id, image: owned.image, run: owned.run, mode: owned.mode, test: t}
	verify := func(ctx context.Context) error { owned.verifyContext(ctx, t); return nil }
	scenarios := []string{"start-unknown"}
	if owned.mode == "graceful" {
		scenarios = []string{"stale-noop", "stop-unknown"}
	}
	for n, scenario := range scenarios {
		if n > 0 {
			<-time.NewTimer(30 * time.Second).C
		} // Actual journal cooldown, not a clock bypass.
		op, want, initial, final := actioncore.Start, actioncore.OutcomeUnknown, "created", "running"
		if scenario == "stale-noop" {
			op, want = actioncore.Stop, actioncore.AlreadyStopped
		}
		if scenario == "stop-unknown" {
			op, initial, final = actioncore.Stop, "running", "exited"
		}
		if owned.verify(t).State.Status != initial {
			t.Fatal("race initial state refused")
		}
		r, delivery := raceRequest(request, binding, op, 100+n)
		trace := &raceTrace{}
		engine := &raceEngine{Engine: owned, trace: trace}
		externalBefore := external.calls
		authority := &raceAuthority{trace: trace, verify: verify, external: func(ctx context.Context) error {
			var err error
			if scenario == "stop-unknown" {
				err = external.Stop(ctx, owned.id, binding.GraceSeconds)
			} else {
				err = external.Start(ctx, owned.id)
			}
			if err != nil {
				return err
			}
			if external.verifyContext(ctx, t).State.Status != final {
				return ErrUnavailable
			}
			return nil
		}}
		worker := actioncore.New(authority, &raceJournal{Journal: journal, trace: trace}, engine, time.Now)
		if worker.Execute(context.Background(), r, binding, delivery) != want {
			t.Fatal("race outcome refused")
		}
		expected := []string{"authority", "inspect", "authority", "external", "external-ack", "intent-persisted", "dispatch"}
		if scenario == "stale-noop" {
			expected = []string{"authority", "inspect", "authority", "external", "external-ack", "finish"}
		}
		if !reflect.DeepEqual(trace.steps, expected) || authority.checks != 2 || engine.inspections != 1 || external.calls != externalBefore+1 {
			t.Fatal("race ordering or external count refused")
		}
		if want == actioncore.OutcomeUnknown {
			// Distinguish a real non-204 response from a timeout or a synthetic error.
			if engine.mutations != 1 || engine.mutationError != ErrUnavailable || len(responseObserver.statuses) != 1 ||
				responseObserver.statuses[0] == http.StatusNoContent {
				t.Fatal("race mutation refusal missing")
			}
		} else if engine.mutations != 0 || len(responseObserver.statuses) != 0 {
			t.Fatal("race no-op dispatched")
		}
		after := owned.verify(t)
		if after.State.Status != final {
			t.Fatal("race final state refused")
		}
		// UNKNOWN is last for this target. Reopen exactly this database, never reset it.
		if want == actioncore.OutcomeUnknown {
			if journal.Close() != nil {
				t.Fatal("race journal close refused")
			}
			var err error
			journal, err = actionjournal.OpenExisting(context.Background(), dir, actionjournal.Binding{ServerID: binding.ServerID, InstallationID: binding.InstallationID, OwnerID: binding.OwnerID})
			if err != nil {
				t.Fatal("race journal reopen refused")
			}
			defer journal.Close()
		}
		beforeInspect, beforeMutate, beforeExternal := engine.inspections, engine.mutations, external.calls
		replay := actioncore.New(&nativeOwnerAuthority{owned}, journal, engine, time.Now)
		now := time.Now().UTC()
		fresh := actioncore.Delivery{StartedAt: now, ReceivedAt: now, LeaseDeadline: now.Add(2 * time.Minute)}
		if replay.Execute(context.Background(), r, binding, fresh) != want {
			t.Fatal("race retained replay refused")
		}
		if want == actioncore.OutcomeUnknown {
			next, nextDelivery := raceRequest(request, binding, op, 200+n)
			if replay.Execute(context.Background(), next, binding, nextDelivery) != actioncore.OutcomeUnknown {
				t.Fatal("race new-key fence missing")
			}
		}
		replayed := owned.verify(t)
		if engine.inspections != beforeInspect || engine.mutations != beforeMutate || external.calls != beforeExternal ||
			replayed.State.Status != after.State.Status || replayed.State.StartedAt != after.State.StartedAt {
			t.Fatal("race replay changed fixture")
		}
		fmt.Println("HLO_ENGINE_NATIVE_RACE_PASS=" + scenario)
	}
	fmt.Println("HLO_ENGINE_NATIVE_RACES_CASE_PASS=" + owned.mode)
}

// Ordinary synthetic Unix transport tests validate harness mechanics only.
// They do not opt into Docker or count as the manual native acceptance proof.
func TestRaceCheckpointOrderingAndPersistentFence(t *testing.T) {
	for _, scenario := range []string{"stale-noop", "stop-unknown", "start-unknown", "external-failure"} {
		t.Run(scenario, func(t *testing.T) {
			var mu sync.Mutex
			state := actioncore.Created
			if scenario == "stop-unknown" {
				state = actioncore.Running
			}
			endpoint := unixFixture(t, versionJSON, func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				if r.Method == http.MethodGet {
					fmt.Fprintf(w, `{"Id":%q,"State":{"Status":%q}}`, fullID, state)
					return
				}
				switch r.URL.Path {
				case "/v1.45/containers/" + fullID + "/start":
					if state == actioncore.Running {
						w.WriteHeader(http.StatusNotModified)
						return
					}
					state = actioncore.Running
				case "/v1.45/containers/" + fullID + "/stop":
					if state != actioncore.Running {
						w.WriteHeader(http.StatusNotModified)
						return
					}
					state = actioncore.Exited
				default:
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})
			adapter, other := openFixture(t, endpoint), openFixture(t, endpoint)
			observer := &raceHTTPObserver{base: adapter.client.Transport}
			adapter.client.Transport = observer
			op, want := actioncore.Start, actioncore.OutcomeUnknown
			if scenario == "stale-noop" {
				op, want = actioncore.Stop, actioncore.AlreadyStopped
			}
			if scenario == "stop-unknown" {
				op = actioncore.Stop
			}
			if scenario == "external-failure" {
				want = actioncore.Denied
			}
			r, b, delivery, _ := workerInput(op)
			dir := t.TempDir()
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal("race fixture permissions")
			}
			journal, err := actionjournal.Provision(context.Background(), dir, actionjournal.Binding{ServerID: b.ServerID, InstallationID: b.InstallationID, OwnerID: b.OwnerID})
			if err != nil {
				t.Fatal("race fixture journal")
			}
			defer journal.Close()
			trace := &raceTrace{}
			engine := &raceEngine{Engine: adapter, trace: trace}
			externalCalls := 0
			authority := &raceAuthority{trace: trace, verify: func(context.Context) error { return nil }, external: func(ctx context.Context) error {
				externalCalls++
				if scenario == "external-failure" {
					return ErrUnavailable
				}
				if scenario == "stop-unknown" {
					return other.Stop(ctx, fullID, b.GraceSeconds)
				}
				return other.Start(ctx, fullID)
			}}
			worker := actioncore.New(authority, &raceJournal{Journal: journal, trace: trace}, engine, time.Now)
			if worker.Execute(context.Background(), r, b, delivery) != want {
				t.Fatal("race fixture result")
			}
			expected := []string{"authority", "inspect", "authority", "external", "external-ack", "intent-persisted", "dispatch"}
			if scenario == "stale-noop" {
				expected = []string{"authority", "inspect", "authority", "external", "external-ack", "finish"}
			}
			if scenario == "external-failure" {
				expected = []string{"authority", "inspect", "authority", "external"}
			}
			if !reflect.DeepEqual(trace.steps, expected) || authority.checks != 2 || externalCalls != 1 || engine.inspections != 1 {
				t.Fatal("race fixture order")
			}
			beforeInspect, beforeMutate := engine.inspections, engine.mutations
			if want == actioncore.OutcomeUnknown {
				if engine.mutations != 1 || !reflect.DeepEqual(observer.statuses, []int{http.StatusNotModified}) {
					t.Fatal("race fixture response")
				}
				if journal.Close() != nil {
					t.Fatal("race fixture close")
				}
				journal, err = actionjournal.OpenExisting(context.Background(), dir, actionjournal.Binding{ServerID: b.ServerID, InstallationID: b.InstallationID, OwnerID: b.OwnerID})
				if err != nil {
					t.Fatal("race fixture reopen")
				}
				defer journal.Close()
			} else if engine.mutations != 0 || len(observer.statuses) != 0 {
				t.Fatal("race fixture unexpected mutation")
			}
			// Refuse an unexpected third use of the hook without another external call.
			if authority.Check(context.Background(), r, b) != ErrUnavailable || externalCalls != 1 {
				t.Fatal("race hook repeated")
			}
			if scenario == "external-failure" {
				return
			}
			replay := actioncore.New(&syntheticAuthority{}, journal, engine, time.Now)
			if replay.Execute(context.Background(), r, b, delivery) != want {
				t.Fatal("race fixture replay")
			}
			if want == actioncore.OutcomeUnknown {
				next := r
				next.ActionID = "hla_" + strings.Repeat("a", 32)
				if replay.Execute(context.Background(), next, b, delivery) != actioncore.OutcomeUnknown {
					t.Fatal("race fixture fence")
				}
			}
			if engine.inspections != beforeInspect || engine.mutations != beforeMutate || externalCalls != 1 {
				t.Fatal("race fixture replay dispatched")
			}
		})
	}
}
