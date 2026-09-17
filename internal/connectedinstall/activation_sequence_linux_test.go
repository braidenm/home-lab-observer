//go:build linux

package connectedinstall

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedactivation"
	"github.com/braidenm/home-lab-observer/internal/connectedprocess"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
)

// All effects below are in-memory fakes. No service, process, listener,
// installation path, credential, or ledger is opened by these transaction tests.
type activationFixture struct {
	t                                                                              *testing.T
	config                                                                         connectedprofile.Config
	workers                                                                        map[string]workerInstance
	events                                                                         []string
	fail                                                                           string
	on                                                                             func(string)
	inspects                                                                       map[string]int
	policyChecks, configChecks                                                     int
	request                                                                        connectedactivation.Request
	response                                                                       connectedactivation.Response
	requestWritten, commitWritten, audited, responseRead, collecting, finished     bool
	filesClosed, probesClosed                                                      bool
	staleResponse, denyTraffic, responseMissing, collectorMissing, uploaderMissing bool
	unknownCapture, stopFailure, joinFailure                                       bool
}

func newActivationFixture(t *testing.T) *activationFixture {
	t.Helper()
	return &activationFixture{t: t,
		config:  connectedprofile.Config{Version: connectedprofile.Version, State: "INSTALLED_READY", ServerID: "srv_" + strings.Repeat("a", 32), ConnectorID: "agent_" + strings.Repeat("b", 32), CollectorUID: 1001, UploaderUID: 1002, UploaderGID: 1003, SharedGID: 1004, ArtifactSHA256: strings.Repeat("c", 64), PolicyGeneration: 1, Addresses: []string{"1.1.1.1"}},
		workers: map[string]workerInstance{uploaderUnit: {Active: "inactive"}, collectorUnit: {Active: "inactive"}}, inspects: map[string]int{}}
}

func role(unit string) string {
	if unit == uploaderUnit {
		return "uploader"
	}
	return "collector"
}
func fixtureWorker(unit string) workerInstance {
	if unit == uploaderUnit {
		return workerInstance{PID: 123, Invocation: strings.Repeat("d", 32), Active: "active"}
	}
	return workerInstance{PID: 124, Invocation: strings.Repeat("e", 32), Active: "active"}
}
func (f *activationFixture) event(name string) bool {
	f.events = append(f.events, name)
	if f.on != nil {
		f.on(name)
	}
	return f.fail == name
}
func (f *activationFixture) operations() activationOperations {
	return activationOperations{
		inspect: func(ctx context.Context, unit string) (workerInstance, error) {
			f.inspects[unit]++
			if f.event("inspect:"+role(unit)) || ctx.Err() != nil {
				return workerInstance{}, ErrUnsafe
			}
			if f.unknownCapture && unit == uploaderUnit && f.inspects[unit] == 2 {
				return workerInstance{}, ErrUnsafe
			}
			return f.workers[unit], nil
		},
		service: func(ctx context.Context, verb, unit string) error {
			if ctx.Err() != nil {
				f.t.Fatal("service operation received canceled context")
			}
			if verb == "start" {
				f.workers[unit] = fixtureWorker(unit)
			}
			if f.event(verb + ":" + role(unit)) {
				return ErrRecovery
			}
			if verb == "stop" {
				if f.stopFailure {
					return ErrRecovery
				}
				if !f.joinFailure {
					f.workers[unit] = workerInstance{Active: "inactive"}
				}
			}
			return nil
		},
		audit: func(ctx context.Context, pid int, path string) (connectedprocess.Identity, error) {
			if f.requestWritten || pid != 123 || path != connectedprofile.ReleaseDirectory+"/"+f.config.ArtifactSHA256+"/observer-connected-uploader" {
				f.t.Fatal("audit identity or ordering invalid")
			}
			if f.event("audit") {
				return connectedprocess.Identity{}, ErrUnsafe
			}
			f.audited = true
			return connectedprocess.Identity{PID: pid, StartTimeTicks: 456}, nil
		},
		pin: func(gid uint32) (activationRecords, error) {
			if gid != f.config.UploaderGID {
				f.t.Fatal("wrong activation owner")
			}
			if f.event("pin") {
				return nil, ErrUnsafe
			}
			return fixtureRecords{f}, nil
		},
		clearResponse: func(connectedprofile.Config) error {
			if f.event("clear") {
				return ErrUnsafe
			}
			return nil
		},
		probes: func(context.Context) (activationProbes, error) {
			if f.event("probes") {
				return nil, ErrUnsafe
			}
			return fixtureProbes{f}, nil
		},
		policy: func(ctx context.Context, unit string, addresses []string) error {
			f.policyChecks++
			if unit != uploaderUnit || !reflect.DeepEqual(addresses, f.config.Addresses) {
				f.t.Fatal("unexpected policy input")
			}
			if f.event("policy") || ctx.Err() != nil {
				return ErrUnsafe
			}
			return nil
		},
		installed: func() (connectedprofile.Config, error) {
			f.configChecks++
			if f.event("installed") {
				return connectedprofile.Config{}, ErrUnsafe
			}
			return f.config, nil
		},
		random: func(b []byte) (int, error) {
			if f.event("random") {
				return 0, ErrUnsafe
			}
			for i := range b {
				b[i] = 0xab
			}
			return len(b), nil
		},
		response: func(connectedprofile.Config) (connectedactivation.Response, error) {
			if !f.requestWritten {
				f.t.Fatal("response before request")
			}
			if f.event("response") {
				return connectedactivation.Response{}, ErrUnsafe
			}
			if f.responseMissing {
				return connectedactivation.Response{}, os.ErrNotExist
			}
			digest, _ := connectedactivation.RequestDigest(f.request)
			f.response = connectedactivation.Response{Version: connectedactivation.ResponseVersion, RequestSHA256: digest, InvocationID: fixtureWorker(uploaderUnit).Invocation, Challenge: strings.Repeat("f", 64), Result: connectedactivation.Pass}
			if f.staleResponse {
				f.response.RequestSHA256 = strings.Repeat("0", 64)
			}
			f.responseRead = true
			return f.response, nil
		},
		status: func(name string, uid uint32) (connectedstatus.Record, error) {
			r := "uploader"
			unit := uploaderUnit
			state := "WAITING_FIRST_UPLOAD"
			if name == "collector-status" {
				r = "collector"
				unit = collectorUnit
				state = "COLLECTING"
				if uid != f.config.CollectorUID {
					f.t.Fatal("wrong collector status owner")
				}
			} else if name != "uploader-status" || uid != f.config.UploaderUID {
				f.t.Fatal("wrong uploader status owner")
			}
			if f.event("status:" + r) {
				return connectedstatus.Record{}, ErrUnsafe
			}
			if r == "collector" && f.collectorMissing || r == "uploader" && f.uploaderMissing {
				return connectedstatus.Record{}, os.ErrNotExist
			}
			if r == "collector" {
				f.collecting = true
			} else if !f.commitWritten {
				f.t.Fatal("uploader readiness read before commit")
			}
			return connectedstatus.Record{InvocationID: fixtureWorker(unit).Invocation, State: state}, nil
		},
		wait: func(ctx context.Context) error {
			f.event("wait")
			if ctx.Err() != nil {
				return ErrUnsafe
			}
			f.t.Fatal("unexpected unbounded fake wait")
			return ErrUnsafe
		},
	}
}

type fixtureRecords struct{ f *activationFixture }

func (r fixtureRecords) close() { r.f.filesClosed = true; r.f.event("close:files") }
func (r fixtureRecords) publish(name string, b []byte) error {
	f := r.f
	if name == "request.json" {
		if !f.audited || f.policyChecks < 2 || f.configChecks < 1 || f.requestWritten {
			f.t.Fatal("request before all pre-probe checks")
		}
		if f.event("publish:request") {
			return ErrRecovery
		}
		request, err := connectedactivation.DecodeRequest(b)
		if err != nil {
			f.t.Fatal("invalid actual request")
		}
		f.request = request
		f.requestWritten = true
		return nil
	}
	if name != "commit.json" || !f.responseRead || !f.collecting || !f.finished || f.policyChecks < 3 || f.configChecks < 2 || f.commitWritten {
		f.t.Fatal("commit before all evidence checks")
	}
	if f.event("publish:commit") {
		return ErrRecovery
	}
	commit, err := connectedactivation.DecodeCommit(b)
	if err != nil || !connectedactivation.MatchCommit(f.request, f.response, commit) {
		f.t.Fatal("invalid actual commit binding")
	}
	f.commitWritten = true
	return nil
}

type fixtureProbes struct{ f *activationFixture }

func (p fixtureProbes) ports() (uint16, uint16) { return 32001, 32002 }
func (p fixtureProbes) denied() bool            { p.f.event("denied"); return !p.f.denyTraffic }
func (p fixtureProbes) close()                  { p.f.probesClosed = true; p.f.event("close:probes") }
func (p fixtureProbes) finish(ctx context.Context) error {
	if p.f.event("finish") || p.f.denyTraffic || ctx.Err() != nil {
		return ErrUnsafe
	}
	p.f.finished = true
	return nil
}

func hasEvent(events []string, want string) bool {
	for _, event := range events {
		if event == want {
			return true
		}
	}
	return false
}

func TestActivationTransactionSuccessOrdering(t *testing.T) {
	f := newActivationFixture(t)
	if err := activateStoppedWith(context.Background(), f.config, f.operations()); err != nil {
		t.Fatal("valid sequence refused")
	}
	if !f.commitWritten || !f.filesClosed || !f.probesClosed || hasEvent(f.events, "stop:uploader") || hasEvent(f.events, "stop:collector") {
		t.Fatal("success lifecycle invalid")
	}
	previous := -1
	for _, required := range []string{"pin", "clear", "probes", "start:uploader", "audit", "publish:request", "response", "start:collector", "status:collector", "finish", "publish:commit", "status:uploader"} {
		found := -1
		for i, event := range f.events {
			if event == required {
				found = i
				break
			}
		}
		if found <= previous {
			t.Fatalf("required order broken at %s", required)
		}
		previous = found
	}
}

func TestActivationTransactionFaultBoundaries(t *testing.T) {
	for _, stage := range []string{"pin", "clear", "probes", "policy", "start:uploader", "audit", "random", "installed", "publish:request", "response", "start:collector", "status:collector", "finish", "publish:commit", "status:uploader"} {
		t.Run(stage, func(t *testing.T) {
			f := newActivationFixture(t)
			f.fail = stage
			err := activateStoppedWith(context.Background(), f.config, f.operations())
			if err == nil {
				t.Fatal("fault accepted")
			}
			if hasEvent(f.events, "start:uploader") && !hasEvent(f.events, "stop:uploader") {
				t.Fatal("owned uploader not stopped")
			}
			if hasEvent(f.events, "start:collector") && !hasEvent(f.events, "stop:collector") {
				t.Fatal("owned collector not stopped")
			}
			if stage != "status:uploader" && f.commitWritten {
				t.Fatal("failure authorized uploader")
			}
			if stage != "pin" && !f.filesClosed {
				t.Fatal("record handle not closed")
			}
			if hasEvent(f.events, "start:uploader") && !f.probesClosed {
				t.Fatal("fixture not joined")
			}
		})
	}
}

func TestActivationStaleResponseAndTrafficReject(t *testing.T) {
	for _, fault := range []string{"stale-response", "traffic-before-request", "traffic-after-response", "policy-drift", "configuration-drift"} {
		t.Run(fault, func(t *testing.T) {
			f := newActivationFixture(t)
			f.on = func(event string) {
				switch fault {
				case "stale-response":
					f.staleResponse = true
				case "traffic-before-request":
					if event == "audit" {
						f.denyTraffic = true
					}
				case "traffic-after-response":
					if event == "response" {
						f.denyTraffic = true
					}
				case "policy-drift":
					if event == "finish" {
						f.fail = "policy"
					}
				case "configuration-drift":
					if event == "finish" {
						f.config.PolicyGeneration++
					}
				}
			}
			if err := activateStoppedWith(context.Background(), f.config, f.operations()); err == nil || f.commitWritten {
				t.Fatal("invalid evidence authorized upload")
			}
			if !hasEvent(f.events, "stop:uploader") {
				t.Fatal("invalid evidence uploader left active")
			}
		})
	}
}

func TestActivationReplacedAndUnknownInvocationNeverStopped(t *testing.T) {
	for _, fault := range []string{"replacement", "unknown-capture", "same-invocation-new-pid"} {
		t.Run(fault, func(t *testing.T) {
			f := newActivationFixture(t)
			if fault == "unknown-capture" {
				f.unknownCapture = true
			} else {
				f.on = func(event string) {
					if event == "audit" {
						w := f.workers[uploaderUnit]
						if fault == "replacement" {
							w.Invocation = strings.Repeat("0", 32)
						} else {
							w.PID++
						}
						f.workers[uploaderUnit] = w
					}
				}
			}
			if err := activateStoppedWith(context.Background(), f.config, f.operations()); err != ErrRecovery {
				t.Fatal("ambiguous ownership not recovery")
			}
			if hasEvent(f.events, "stop:uploader") || f.requestWritten || f.commitWritten {
				t.Fatal("foreign/unknown invocation mutated")
			}
		})
	}
}

func TestActivationCancellationUsesIndependentCleanup(t *testing.T) {
	for _, stage := range []string{"start:uploader", "response", "status:collector", "publish:commit", "status:uploader"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := newActivationFixture(t)
			f.on = func(event string) {
				if event == stage {
					cancel()
					if stage == "response" {
						f.responseMissing = true
					}
					if stage == "status:collector" {
						f.collectorMissing = true
					}
				}
			}
			if err := activateStoppedWith(ctx, f.config, f.operations()); err == nil {
				t.Fatal("cancellation accepted")
			}
			if !hasEvent(f.events, "stop:uploader") {
				t.Fatal("canceled uploader not joined")
			}
			if hasEvent(f.events, "start:collector") && !hasEvent(f.events, "stop:collector") {
				t.Fatal("canceled collector not joined")
			}
			if !f.filesClosed || !f.probesClosed {
				t.Fatal("canceled resources not closed")
			}
		})
	}
}

func TestActivationUncertainCommitStillStopsBothWorkers(t *testing.T) {
	f := newActivationFixture(t)
	f.on = func(event string) {
		if event == "publish:commit" {
			// Publication may have reached the worker even if final sync reports
			// failure. Cleanup must not rely on the absence of a commit record.
			f.commitWritten = true
			f.fail = "publish:commit"
		}
	}
	if err := activateStoppedWith(context.Background(), f.config, f.operations()); err != ErrRecovery {
		t.Fatal("uncertain publication not recovery")
	}
	if !f.commitWritten || !hasEvent(f.events, "stop:uploader") || !hasEvent(f.events, "stop:collector") {
		t.Fatal("uncertain commit escaped cleanup")
	}
}

func TestActivationFinalStatusCannotAdoptReplacedInvocation(t *testing.T) {
	for _, unit := range []string{uploaderUnit, collectorUnit} {
		t.Run(role(unit), func(t *testing.T) {
			f := newActivationFixture(t)
			f.on = func(event string) {
				if event == "status:uploader" {
					w := f.workers[unit]
					w.Invocation = strings.Repeat("0", 32)
					f.workers[unit] = w
				}
			}
			if err := activateStoppedWith(context.Background(), f.config, f.operations()); err != ErrRecovery {
				t.Fatal("replaced invocation accepted as ready")
			}
			if !f.commitWritten || hasEvent(f.events, "stop:"+role(unit)) {
				t.Fatal("foreign invocation stopped during cleanup")
			}
			other := uploaderUnit
			if unit == uploaderUnit {
				other = collectorUnit
			}
			if !hasEvent(f.events, "stop:"+role(other)) {
				t.Fatal("remaining owned invocation not stopped")
			}
		})
	}
}

func TestActivationWaitsForCurrentStatusInvocation(t *testing.T) {
	for _, name := range []string{"collector-status", "uploader-status"} {
		for _, neverRefresh := range []bool{false, true} {
			label := name + "/refresh"
			if neverRefresh {
				label = name + "/canceled"
			}
			t.Run(label, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				f := newActivationFixture(t)
				ops := f.operations()
				status := ops.status
				reads, waits := 0, 0
				ops.status = func(actual string, uid uint32) (connectedstatus.Record, error) {
					record, err := status(actual, uid)
					if actual == name {
						reads++
						if reads == 1 || neverRefresh {
							record.InvocationID = strings.Repeat("0", 32)
							if name == "collector-status" {
								f.collecting = false
							}
						}
					}
					return record, err
				}
				ops.wait = func(check context.Context) error {
					waits++
					f.event("wait")
					if waits != 1 || reads != 1 {
						t.Fatal("unbounded stale-status polling")
					}
					if name == "collector-status" && f.commitWritten {
						t.Fatal("stale collector status authorized commit")
					}
					if name == "uploader-status" && !f.commitWritten {
						t.Fatal("uploader readiness read before commit")
					}
					if _, ok := check.Deadline(); !ok {
						t.Fatal("status polling missing deadline")
					}
					if neverRefresh {
						cancel()
						return ErrUnsafe
					}
					return nil
				}
				err := activateStoppedWith(ctx, f.config, ops)
				if neverRefresh {
					if err == nil || !hasEvent(f.events, "stop:uploader") || !hasEvent(f.events, "stop:collector") {
						t.Fatal("stale canceled status escaped cleanup")
					}
				} else if err != nil || reads != 2 || waits != 1 || !f.commitWritten {
					t.Fatal("current status did not complete restart")
				}
			})
		}
	}
}

func TestActivationCleanupFailuresRemainRecovery(t *testing.T) {
	for _, mode := range []string{"stop-fails", "not-joined", "inspection-fails"} {
		t.Run(mode, func(t *testing.T) {
			f := newActivationFixture(t)
			f.on = func(event string) {
				if event == "publish:commit" {
					f.fail = "publish:commit"
					switch mode {
					case "stop-fails":
						f.stopFailure = true
					case "not-joined":
						f.joinFailure = true
					case "inspection-fails":
						f.fail = "inspect:uploader"
					}
				}
			}
			if mode == "inspection-fails" {
				f.on = func(event string) {
					if event == "response" {
						f.fail = "inspect:uploader"
					}
				}
			}
			if err := activateStoppedWith(context.Background(), f.config, f.operations()); !errors.Is(err, ErrRecovery) {
				t.Fatal("failed cleanup not recovery")
			}
			if !f.probesClosed || !f.filesClosed {
				t.Fatal("failed cleanup leaked fixture")
			}
		})
	}
}
