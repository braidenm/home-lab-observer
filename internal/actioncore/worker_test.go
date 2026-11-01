package actioncore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fixture struct {
	now          time.Time
	events       []string
	state        State
	authCalls    int
	denyAt       int
	intentError  bool
	finishError  bool
	engineError  bool
	intentDelay  time.Duration
	engineDelay  time.Duration
	previous     Code
	keys         []Key
	acquireError error
	inspectError bool
	intentHook   func()
	request      Request
	binding      Binding
	delivery     Delivery
}

func newFixture() *fixture {
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	f := &fixture{now: now, state: Running}
	f.request = Request{ActionID: "hla_" + strings.Repeat("1", 32), ServerID: "srv_" + strings.Repeat("2", 32), InstallationID: "agent_" + strings.Repeat("3", 32), OwnerID: "usr_" + strings.Repeat("4", 32), ActorID: "usr_" + strings.Repeat("5", 32), TargetID: "hlt_" + strings.Repeat("6", 32), Operation: Restart, GraceSeconds: 30, PolicyRevision: 1, CapabilityRevision: 2, CreatedAt: now, ExpiresAt: now.Add(5 * time.Minute)}
	f.binding = Binding{ServerID: f.request.ServerID, InstallationID: f.request.InstallationID, OwnerID: f.request.OwnerID, TargetID: f.request.TargetID, EngineGeneration: strings.Repeat("7", 32), ContainerID: strings.Repeat("8", 64), PolicyRevision: 1, CapabilityRevision: 2, GraceSeconds: 30, Operations: []Operation{Start, Stop, Restart}}
	f.delivery = Delivery{StartedAt: now, ReceivedAt: now, LeaseDeadline: now.Add(2 * time.Minute)}
	return f
}

func (f *fixture) Check(context.Context, Request, Binding) error {
	f.authCalls++
	f.events = append(f.events, "auth")
	if f.authCalls == f.denyAt {
		return errors.New("private-authorization-canary")
	}
	return nil
}
func (f *fixture) Acquire(_ context.Context, key Key) (Attempt, error) {
	f.events = append(f.events, "acquire")
	if f.acquireError != nil {
		return nil, f.acquireError
	}
	if len(f.keys) > 0 && f.keys[0].ActionID == key.ActionID && f.keys[0].Fingerprint != key.Fingerprint {
		return nil, ErrIdentityConflict
	}
	f.keys = append(f.keys, key)
	return f, nil
}
func (f *fixture) Existing() Code { return f.previous }
func (f *fixture) Intent(context.Context) error {
	f.events = append(f.events, "intent")
	f.previous = OutcomeUnknown
	f.now = f.now.Add(f.intentDelay)
	if f.intentHook != nil {
		f.intentHook()
	}
	if f.intentError {
		return errors.New("private-storage-canary")
	}
	return nil
}
func (f *fixture) Finish(_ context.Context, code Code) error {
	f.events = append(f.events, "finish")
	if f.finishError {
		return errors.New("private-finish-canary")
	}
	f.previous = code
	return nil
}
func (f *fixture) Close() { f.events = append(f.events, "close") }
func (f *fixture) Inspect(context.Context, string) (State, error) {
	f.events = append(f.events, "inspect")
	if f.inspectError {
		return "", errors.New("private-inspect-canary")
	}
	return f.state, nil
}
func (f *fixture) mutate(op string) error {
	f.events = append(f.events, op)
	f.now = f.now.Add(f.engineDelay)
	if f.engineError {
		return errors.New("private-engine-canary")
	}
	return nil
}
func (f *fixture) Start(context.Context, string) error          { return f.mutate("start") }
func (f *fixture) Stop(context.Context, string, uint8) error    { return f.mutate("stop") }
func (f *fixture) Restart(context.Context, string, uint8) error { return f.mutate("restart") }
func (f *fixture) run(ctx context.Context) Code {
	return New(f, f, f, func() time.Time { return f.now }).Execute(ctx, f.request, f.binding, f.delivery)
}
func count(events []string, name string) int {
	n := 0
	for _, e := range events {
		if e == name {
			n++
		}
	}
	return n
}

func TestOperationStateMatrix(t *testing.T) {
	for _, op := range []Operation{Start, Stop, Restart} {
		for _, state := range []State{Created, Exited, Running, Paused, Restarting, Removing, Dead, UnknownState} {
			t.Run(string(op)+"/"+string(state), func(t *testing.T) {
				f := newFixture()
				f.request.Operation = op
				f.state = state
				if op == Start {
					f.request.GraceSeconds = 0
				}
				want := StateConflict
				call := ""
				if op == Start && (state == Created || state == Exited) {
					want = Started
					call = "start"
				}
				if op == Start && state == Running {
					want = AlreadyRunning
				}
				if op == Stop && state == Running {
					want = Stopped
					call = "stop"
				}
				if op == Stop && (state == Created || state == Exited) {
					want = AlreadyStopped
				}
				if op == Restart && state == Running {
					want = Restarted
					call = "restart"
				}
				if got := f.run(context.Background()); got != want {
					t.Fatalf("code=%s want=%s", got, want)
				}
				mutations := count(f.events, "start") + count(f.events, "stop") + count(f.events, "restart")
				if (call == "" && mutations != 0) || (call != "" && (mutations != 1 || count(f.events, call) != 1)) {
					t.Fatal("incorrect mutation count")
				}
				if call != "" {
					joined := strings.Join(f.events, ",")
					if joined != "auth,acquire,inspect,auth,intent,"+call+",finish,close" {
						t.Fatalf("bad ordering: %s", joined)
					}
				}
			})
		}
	}
}

func TestIndependentOperationGrants(t *testing.T) {
	ops := []Operation{Start, Stop, Restart}
	for _, op := range ops {
		for subset := 0; subset < 8; subset++ {
			f := newFixture()
			f.request.Operation = op
			f.binding.Operations = nil
			if op == Start {
				f.request.GraceSeconds = 0
			}
			allowed := false
			for index, candidate := range ops {
				if subset&(1<<index) != 0 {
					f.binding.Operations = append(f.binding.Operations, candidate)
					allowed = allowed || candidate == op
				}
			}
			got := f.run(context.Background())
			if allowed == (got == Denied) {
				t.Fatalf("independent operation permission failed: %s subset=%d", op, subset)
			}
			if !allowed && len(f.events) != 0 {
				t.Fatal("unapproved operation reached ports")
			}
		}
	}
}

func TestIndependentPermissionAndBindingRefusal(t *testing.T) {
	cases := map[string]func(*fixture){
		"other-operation":  func(f *fixture) { f.binding.Operations = []Operation{Start, Stop} },
		"owner":            func(f *fixture) { f.binding.OwnerID = "usr_" + strings.Repeat("9", 32) },
		"server":           func(f *fixture) { f.binding.ServerID = "srv_" + strings.Repeat("9", 32) },
		"installation":     func(f *fixture) { f.binding.InstallationID = "agent_" + strings.Repeat("9", 32) },
		"target":           func(f *fixture) { f.binding.TargetID = "hlt_" + strings.Repeat("9", 32) },
		"full-id":          func(f *fixture) { f.binding.ContainerID = "short-name" },
		"generation":       func(f *fixture) { f.binding.EngineGeneration = "" },
		"policy":           func(f *fixture) { f.binding.PolicyRevision++ },
		"capability":       func(f *fixture) { f.binding.CapabilityRevision++ },
		"grace":            func(f *fixture) { f.request.GraceSeconds = 1 },
		"zero-grace":       func(f *fixture) { f.request.GraceSeconds = 0 },
		"large-grace":      func(f *fixture) { f.request.GraceSeconds = 31; f.binding.GraceSeconds = 31 },
		"unknown-action":   func(f *fixture) { f.request.Operation = "exec" },
		"duplicate-policy": func(f *fixture) { f.binding.Operations = []Operation{Restart, Restart} },
		"expiry-equality":  func(f *fixture) { f.now = f.request.ExpiresAt },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			change(f)
			if got := f.run(context.Background()); got != Denied {
				t.Fatalf("code=%s", got)
			}
			if len(f.events) != 0 {
				t.Fatal("invalid request reached ports")
			}
		})
	}
}

func TestAuthorityLossAndUncertaintyNeverRetry(t *testing.T) {
	for _, denyAt := range []int{1, 2} {
		f := newFixture()
		f.denyAt = denyAt
		if got := f.run(context.Background()); got != Denied {
			t.Fatalf("deny code=%s", got)
		}
		if count(f.events, "intent") != 0 || count(f.events, "restart") != 0 {
			t.Fatal("denied mutation")
		}
	}
	for _, kind := range []string{"intent", "engine", "finish", "late-engine", "slow-intent"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture()
			switch kind {
			case "intent":
				f.intentError = true
			case "engine":
				f.engineError = true
			case "finish":
				f.finishError = true
			case "late-engine":
				f.engineDelay = time.Minute
			case "slow-intent":
				f.intentDelay = 30 * time.Second
			}
			if got := f.run(context.Background()); got != OutcomeUnknown {
				t.Fatalf("code=%s", got)
			}
			if count(f.events, "restart") > 1 {
				t.Fatal("automatic retry")
			}
			if (kind == "intent" || kind == "slow-intent") && count(f.events, "restart") != 0 {
				t.Fatal("dispatch after failed/exhausted intent")
			}
		})
	}
}

func TestDuplicateAndBudgets(t *testing.T) {
	for _, code := range []Code{Restarted, OutcomeUnknown} {
		f := newFixture()
		f.previous = code
		if got := f.run(context.Background()); got != code {
			t.Fatal("lost retained result")
		}
		if strings.Join(f.events, ",") != "auth,acquire,close" {
			t.Fatal("replayed engine work")
		}
	}
	f := newFixture()
	f.delivery.ReceivedAt = f.delivery.StartedAt.Add(31 * time.Second)
	f.now = f.delivery.ReceivedAt
	if got := f.run(context.Background()); got != BudgetExceeded {
		t.Fatalf("slow claim=%s", got)
	}
	f = newFixture()
	f.now = f.now.Add(30 * time.Second)
	if got := f.run(context.Background()); got != BudgetExceeded {
		t.Fatalf("budget=%s", got)
	}
	if count(f.events, "intent") != 0 {
		t.Fatal("late intent")
	}
	f = newFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := f.run(ctx); got != BudgetExceeded || len(f.events) != 0 {
		t.Fatal("canceled work")
	}
}

func TestRecoveredDeliveryAndRebinding(t *testing.T) {
	f := newFixture()
	f.engineError = true
	if f.run(context.Background()) != OutcomeUnknown {
		t.Fatal("first outcome")
	}
	f.delivery.LeaseDeadline = f.delivery.LeaseDeadline.Add(time.Minute)
	if f.run(context.Background()) != OutcomeUnknown || count(f.events, "restart") != 1 || len(f.keys) != 2 || f.keys[0] != f.keys[1] {
		t.Fatal("reclaimed delivery replayed mutation or changed identity")
	}
	f.binding.ContainerID = strings.Repeat("9", 64)
	if f.run(context.Background()) != IdentityConflict || count(f.events, "restart") != 1 {
		t.Fatal("rebound container reused request")
	}
}

func TestBoundaryFailures(t *testing.T) {
	for _, code := range []Code{Stopped, AlreadyStopped, "private-code-canary"} {
		f := newFixture()
		f.previous = code
		if f.run(context.Background()) != Unavailable || count(f.events, "inspect") != 0 {
			t.Fatal("mismatched retained result")
		}
	}
	for _, failure := range []struct {
		err  error
		code Code
	}{{ErrIdentityConflict, IdentityConflict}, {ErrUnresolvedTarget, OutcomeUnknown}, {errors.New("private-journal-canary"), Unavailable}} {
		f := newFixture()
		f.acquireError = failure.err
		if f.run(context.Background()) != failure.code || count(f.events, "inspect") != 0 {
			t.Fatal("journal refusal")
		}
	}
	f := newFixture()
	f.inspectError = true
	if f.run(context.Background()) != Unavailable || count(f.events, "intent") != 0 {
		t.Fatal("inspect refusal")
	}
	f = newFixture()
	f.engineDelay = 45 * time.Second
	if f.run(context.Background()) != OutcomeUnknown || count(f.events, "finish") != 0 {
		t.Fatal("late acknowledgement")
	}
	f = newFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.intentHook = cancel
	if f.run(ctx) != OutcomeUnknown || count(f.events, "restart") != 0 {
		t.Fatal("canceled after intent")
	}
	f = newFixture()
	f.request.GraceSeconds = 1
	f.binding.GraceSeconds = 1
	if f.run(context.Background()) != Restarted {
		t.Fatal("minimum grace rejected")
	}
}

func TestFingerprintBindsEveryImmutableField(t *testing.T) {
	changes := []func(*fixture){func(f *fixture) { f.request.ActionID = "hla_" + strings.Repeat("9", 32) }, func(f *fixture) { f.request.ServerID = "srv_" + strings.Repeat("9", 32) }, func(f *fixture) { f.request.InstallationID = "agent_" + strings.Repeat("9", 32) }, func(f *fixture) { f.request.OwnerID = "usr_" + strings.Repeat("9", 32) }, func(f *fixture) { f.request.ActorID = "usr_" + strings.Repeat("9", 32) }, func(f *fixture) { f.request.TargetID = "hlt_" + strings.Repeat("9", 32) }, func(f *fixture) { f.binding.EngineGeneration = strings.Repeat("9", 32) }, func(f *fixture) { f.binding.ContainerID = strings.Repeat("9", 64) }, func(f *fixture) { f.request.Operation = Stop }, func(f *fixture) { f.request.GraceSeconds = 1 }, func(f *fixture) { f.request.PolicyRevision++ }, func(f *fixture) { f.request.CapabilityRevision++ }, func(f *fixture) { f.request.CreatedAt = f.request.CreatedAt.Add(time.Second) }, func(f *fixture) { f.request.ExpiresAt = f.request.ExpiresAt.Add(time.Second) }}
	for index, change := range changes {
		f := newFixture()
		before := fingerprint(f.request, f.binding)
		change(f)
		if before == fingerprint(f.request, f.binding) {
			t.Fatalf("unbound field %d", index)
		}
	}
}
