//go:build linux

package connectedinstall

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedprocess"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedstatus"
)

func TestLifecycleRefusesMissingContextAndUnknownService(t *testing.T) {
	if Start(nil) == nil || Stop(nil) == nil {
		t.Fatal("missing context accepted")
	}
	if _, err := ReadStatus(nil); err == nil {
		t.Fatal("missing status context accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if Start(ctx) == nil || Stop(ctx) == nil {
		t.Fatal("canceled operation accepted")
	}
	for _, test := range []struct{ verb, unit string }{
		{"restart", uploaderUnit}, {"enable", collectorUnit}, {"start", "other.service"},
	} {
		if workerService(context.Background(), test.verb, test.unit) == nil {
			t.Fatal("unknown service operation accepted")
		}
	}
}

func TestActivationRechecksProcessIdentityBeforeCommit(t *testing.T) {
	for _, fault := range []string{"start-time", "executable"} {
		t.Run(fault, func(t *testing.T) {
			f := newActivationFixture(t)
			ops := f.operations()
			audit := ops.audit
			ops.audit = func(ctx context.Context, pid int, path string) (connectedprocess.Identity, error) {
				identity, err := audit(ctx, pid, path)
				if f.auditCount == 2 {
					if fault == "executable" {
						return identity, ErrUnsafe
					}
					identity.StartTimeTicks++
				}
				return identity, err
			}
			if activateStoppedWith(context.Background(), f.config, ops) == nil || f.commitWritten || !hasEvent(f.events, "stop:uploader") || !hasEvent(f.events, "stop:collector") {
				t.Fatal("changed process identity authorized commit or escaped cleanup")
			}
		})
	}
}

func TestActivationCleanupAttemptsBothWorkersAfterManagerTimeout(t *testing.T) {
	f := newActivationFixture(t)
	f.fail = "publish:commit"
	ops := f.operations()
	inspect := ops.inspect
	secondAttempt := false
	ops.inspect = func(ctx context.Context, unit string) (workerInstance, error) {
		if hasEvent(f.events, "publish:commit") {
			if unit == uploaderUnit {
				return workerInstance{}, context.DeadlineExceeded
			}
			secondAttempt = ctx.Err() == nil
		}
		return inspect(ctx, unit)
	}
	err := activateStoppedWith(context.Background(), f.config, ops)
	if !errors.Is(err, ErrRecovery) || LifecyclePhase(err) != "ACTIVATION_CLEANUP" || !secondAttempt || !hasEvent(f.events, "stop:collector") {
		t.Fatal("first cleanup timeout prevented second owned worker cleanup")
	}
}

func TestStatusRequiresCoherentCurrentWorkerPair(t *testing.T) {
	for _, test := range []struct{ collector, uploader, want string }{
		{"inactive", "inactive", "STOPPED"}, {"active", "inactive", "RECOVERY_REQUIRED"},
		{"inactive", "active", "RECOVERY_REQUIRED"}, {"active", "failed", "RECOVERY_REQUIRED"},
		{"failed", "active", "RECOVERY_REQUIRED"}, {"activating", "inactive", "STARTING"},
		{"active", "deactivating", "STARTING"}, {"active", "active", "ACKNOWLEDGED_FRESH"},
	} {
		t.Run(test.collector+"/"+test.uploader, func(t *testing.T) {
			now := time.Now().UTC()
			collector, uploader := fixtureWorker(collectorUnit), fixtureWorker(uploaderUnit)
			collector.Active, uploader.Active = test.collector, test.uploader
			collected := &connectedstatus.Record{InvocationID: collector.Invocation, State: "COLLECTING"}
			uploaded := &connectedstatus.Record{InvocationID: uploader.Invocation, State: "ACKNOWLEDGED_FRESH", CollectedAt: now}
			if got := classifyPairStatus(collector, uploader, collected, uploaded, now); got.State != test.want {
				t.Fatalf("pair state = %s, want %s", got.State, test.want)
			}
		})
	}
	collector, uploader := fixtureWorker(collectorUnit), fixtureWorker(uploaderUnit)
	now := time.Now().UTC()
	collected := &connectedstatus.Record{InvocationID: collector.Invocation, State: "COLLECTING"}
	uploaded := &connectedstatus.Record{InvocationID: uploader.Invocation, State: "ACKNOWLEDGED_FRESH", CollectedAt: now.Add(-connectedstatus.FreshnessWindow - time.Second)}
	if got := classifyPairStatus(collector, uploader, collected, uploaded, now); got.State != "ACKNOWLEDGED_STALE" || uploaded.State != "ACKNOWLEDGED_FRESH" {
		t.Fatal("staleness or input isolation failed")
	}
	uploaded.InvocationID = "old"
	if classifyPairStatus(collector, uploader, collected, uploaded, now).State != "STARTING" {
		t.Fatal("old invocation represented as current")
	}
	uploaded.InvocationID = uploader.Invocation
	uploaded.State = "COLLECTING"
	if classifyPairStatus(collector, uploader, collected, uploaded, now).State != "RECOVERY_REQUIRED" {
		t.Fatal("collector-only state accepted for uploader")
	}
}

func TestActivationPreservesUncertainResponseCleanupClassification(t *testing.T) {
	f := newActivationFixture(t)
	ops := f.operations()
	ops.clearResponse = func(connectedprofile.Config) error { return ErrRecovery }
	if !errors.Is(activateStoppedWith(context.Background(), f.config, ops), ErrRecovery) || hasEvent(f.events, "start:uploader") {
		t.Fatal("uncertain prior-response removal lost recovery classification")
	}
}

func TestActivationWaitsForPreCommitStoppedStatus(t *testing.T) {
	f := newActivationFixture(t)
	ops := f.operations()
	read := ops.status
	reads, waits := 0, 0
	ops.status = func(name string, uid uint32) (connectedstatus.Record, error) {
		r, err := read(name, uid)
		if name == "uploader-status" {
			reads++
			if reads == 1 {
				r.State = "STOPPED"
			}
		}
		return r, err
	}
	ops.wait = func(context.Context) error { waits++; return nil }
	if activateStoppedWith(context.Background(), f.config, ops) != nil || reads != 2 || waits != 1 {
		t.Fatal("pre-commit status raced admitted startup")
	}
}

func TestActivationRefusesShortNonceAndAcceptsStoppedFailedInvocation(t *testing.T) {
	f := newActivationFixture(t)
	ops := f.operations()
	ops.random = func(b []byte) (int, error) { return len(b) - 1, nil }
	if activateStoppedWith(context.Background(), f.config, ops) == nil || f.requestWritten || !hasEvent(f.events, "stop:uploader") {
		t.Fatal("short entropy escaped refusal/cleanup")
	}
	f = newActivationFixture(t)
	f.workers[uploaderUnit] = workerInstance{Active: "failed"}
	if activateStoppedWith(context.Background(), f.config, f.operations()) != nil {
		t.Fatal("stopped failed invocation prevented explicit recovery")
	}
}
