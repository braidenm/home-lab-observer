//go:build linux

package connectedinstall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedactivation"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

// activateStopped is internal: its caller holds the installation lease, has
// audited principals, and has stopped/joined both exact installed workers.
// Packaged disposable-VM acceptance remains a release prerequisite, not a flag.
func activateStopped(parent context.Context, c connectedprofile.Config) (result error) {
	return activateStoppedWith(parent, c, systemActivationOperations())
}

func activateStoppedWith(parent context.Context, c connectedprofile.Config, ops activationOperations) (result error) {
	if parent == nil || parent.Err() != nil {
		return ErrUnsafe
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	for _, unit := range []string{uploaderUnit, collectorUnit} {
		state, err := ops.inspect(ctx, unit)
		if err != nil || state.Active != "inactive" {
			return ErrUnsafe
		}
	}
	d, err := ops.pin(c.UploaderGID)
	if err != nil {
		return err
	}
	defer d.close()
	if ops.clearResponse(c) != nil {
		return ErrUnsafe
	}
	pair, err := ops.probes(ctx)
	if err != nil {
		return err
	}
	defer pair.close()
	owned := map[string]workerInstance{}
	defer func() {
		if result == nil {
			return
		}
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		for _, unit := range []string{uploaderUnit, collectorUnit} {
			want, ok := owned[unit]
			if !ok {
				continue
			}
			got, e := ops.inspect(cleanup, unit)
			if e != nil {
				result = ErrRecovery
				continue
			}
			if got.PID == 0 && (got.Active == "inactive" || got.Active == "failed") {
				continue
			}
			if want.Invocation == "" || got.Invocation != want.Invocation || (got.PID != want.PID && want.PID != 0) || ops.service(cleanup, "stop", unit) != nil {
				result = ErrRecovery
				continue
			}
			joined, e := ops.inspect(cleanup, unit)
			if e != nil || joined.PID != 0 || joined.Active != "inactive" {
				result = ErrRecovery
			}
		}
	}()
	if ops.policy(ctx, uploaderUnit, c.Addresses) != nil {
		return ErrUnsafe
	}
	if err := startActivationWorkerWith(ctx, uploaderUnit, owned, ops.service, ops.inspect); err != nil {
		return err
	}
	uploader := owned[uploaderUnit]
	identity, err := ops.audit(ctx, uploader.PID, connectedprofile.ReleaseDirectory+"/"+c.ArtifactSHA256+"/observer-connected-uploader")
	if err != nil || identity.PID != uploader.PID || !ops.sameWorker(ctx, uploaderUnit, uploader) {
		return ErrUnsafe
	}
	config, err := connectedprofile.Encode(c)
	if err != nil {
		return ErrUnsafe
	}
	digest := sha256.Sum256(config)
	nonce := make([]byte, 32)
	if _, err := ops.random(nonce); err != nil {
		return ErrUnsafe
	}
	v4, v6 := pair.ports()
	request := connectedactivation.Request{Version: connectedactivation.RequestVersion, Nonce: hex.EncodeToString(nonce), ArtifactSHA256: c.ArtifactSHA256, ConfigSHA256: hex.EncodeToString(digest[:]), PolicyGeneration: c.PolicyGeneration, IPv4Port: v4, IPv6Port: v6}
	encoded, err := connectedactivation.EncodeRequest(request)
	if err != nil {
		return ErrUnsafe
	}
	if !ops.unchanged(ctx, c, config, uploader) || !pair.denied() {
		return ErrUnsafe
	}
	if d.publish("request.json", encoded) != nil {
		return ErrRecovery
	}
	var response connectedactivation.Response
	for {
		if !ops.sameWorker(ctx, uploaderUnit, uploader) || !pair.denied() {
			return ErrUnsafe
		}
		response, err = ops.response(c)
		if err == nil {
			if !connectedactivation.MatchResponse(request, uploader.Invocation, response) {
				return ErrUnsafe
			}
			break
		}
		if !os.IsNotExist(err) || ops.wait(ctx) != nil {
			return ErrUnsafe
		}
	}
	if err := startActivationWorkerWith(ctx, collectorUnit, owned, ops.service, ops.inspect); err != nil {
		return err
	}
	collector := owned[collectorUnit]
	for {
		if !ops.sameWorker(ctx, collectorUnit, collector) || !ops.sameWorker(ctx, uploaderUnit, uploader) {
			return ErrUnsafe
		}
		record, e := ops.status("collector-status", c.CollectorUID)
		if e == nil && record.InvocationID == collector.Invocation {
			if record.State != "COLLECTING" {
				return ErrUnsafe
			}
			break
		}
		if e != nil && !os.IsNotExist(e) {
			return ErrUnsafe
		}
		if ops.wait(ctx) != nil {
			return ErrUnsafe
		}
	}
	if pair.finish(ctx) != nil || !ops.unchanged(ctx, c, config, uploader) || !ops.sameWorker(ctx, collectorUnit, collector) {
		return ErrUnsafe
	}
	commit := connectedactivation.Commit{Version: connectedactivation.CommitVersion, RequestSHA256: response.RequestSHA256, InvocationID: response.InvocationID, Challenge: response.Challenge}
	encoded, err = connectedactivation.EncodeCommit(commit)
	if err != nil || ctx.Err() != nil {
		return ErrUnsafe
	}
	if d.publish("commit.json", encoded) != nil {
		return ErrRecovery
	}
	for {
		if !ops.sameWorker(ctx, uploaderUnit, uploader) || !ops.sameWorker(ctx, collectorUnit, collector) {
			return ErrUnsafe
		}
		record, e := ops.status("uploader-status", c.UploaderUID)
		if e == nil && record.InvocationID == uploader.Invocation {
			switch record.State {
			case "WAITING_FIRST_UPLOAD", "PENDING", "ACKNOWLEDGED_FRESH":
				// The status read can race cancellation or a replaced invocation.
				// Do not turn stale readiness into a successful transaction that
				// skips cleanup. A cancellation after return remains a caller race.
				if ctx.Err() != nil || !ops.sameWorker(ctx, uploaderUnit, uploader) || !ops.sameWorker(ctx, collectorUnit, collector) || ctx.Err() != nil {
					return ErrUnsafe
				}
				return nil
			default:
				return ErrUnsafe
			}
		}
		if e != nil && !os.IsNotExist(e) {
			return ErrUnsafe
		}
		if ops.wait(ctx) != nil {
			return ErrUnsafe
		}
	}
}

func (ops activationOperations) unchanged(ctx context.Context, c connectedprofile.Config, encoded []byte, uploader workerInstance) bool {
	current, err := ops.installed()
	if err != nil {
		return false
	}
	actual, err := connectedprofile.Encode(current)
	return err == nil && bytes.Equal(actual, encoded) && ops.policy(ctx, uploaderUnit, c.Addresses) == nil && ops.sameWorker(ctx, uploaderUnit, uploader)
}

func (ops activationOperations) sameWorker(ctx context.Context, unit string, want workerInstance) bool {
	got, err := ops.inspect(ctx, unit)
	return err == nil && want.Active == "active" && got == want
}

func activationWait(ctx context.Context) error {
	timer := time.NewTimer(50 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ErrUnsafe
	case <-timer.C:
		return nil
	}
}

func startActivationWorker(ctx context.Context, unit string, owned map[string]workerInstance) error {
	return startActivationWorkerWith(ctx, unit, owned, service, inspectWorker)
}

func startActivationWorkerWith(ctx context.Context, unit string, owned map[string]workerInstance, run func(context.Context, string, string) error, inspect func(context.Context, string) (workerInstance, error)) error {
	owned[unit] = workerInstance{} // An uncertain start must remain recovery-visible.
	err := run(ctx, "start", unit)
	// Even failed/timeout start may have launched the service. Capture the owned
	// active invocation for bounded cleanup; never infer a PID from command output.
	// Parent cancellation may race with inspection itself. Always detach this
	// bounded identity capture, not just when cancellation is already visible.
	check, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	state, e := inspect(check, unit)
	if e == nil && state.Invocation != "" {
		owned[unit] = state
	}
	if err != nil || e != nil || state.Active != "active" || ctx.Err() != nil {
		return ErrRecovery
	}
	return nil
}
