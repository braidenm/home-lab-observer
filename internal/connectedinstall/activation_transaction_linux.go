//go:build linux

package connectedinstall

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedactivation"
	"github.com/braidenm/home-lab-observer/internal/connectedpolicy"
	"github.com/braidenm/home-lab-observer/internal/connectedprocess"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

// activateStopped is internal: its caller holds the installation lease, has
// audited principals, and has stopped/joined both exact installed workers.
// Packaged disposable-VM acceptance remains a release prerequisite, not a flag.
func activateStopped(parent context.Context, c connectedprofile.Config) (result error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	for _, unit := range []string{uploaderUnit, collectorUnit} {
		state, err := inspectWorker(ctx, unit)
		if err != nil || state.Active != "inactive" {
			return ErrUnsafe
		}
	}
	d, err := pinActivation(c.UploaderGID)
	if err != nil {
		return err
	}
	defer d.Close()
	if clearActivationResponse(c) != nil {
		return ErrUnsafe
	}
	pair, err := newProbePair(ctx)
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
			got, e := inspectWorker(cleanup, unit)
			if e != nil {
				result = ErrRecovery
				continue
			}
			if got.PID == 0 && (got.Active == "inactive" || got.Active == "failed") {
				continue
			}
			if want.Invocation == "" || got.Invocation != want.Invocation || (got.PID != want.PID && want.PID != 0) || service(cleanup, "stop", unit) != nil {
				result = ErrRecovery
				continue
			}
			joined, e := inspectWorker(cleanup, unit)
			if e != nil || joined.PID != 0 || joined.Active != "inactive" {
				result = ErrRecovery
			}
		}
	}()
	if connectedpolicy.ValidateEffective(ctx, uploaderUnit, c.Addresses) != nil {
		return ErrUnsafe
	}
	if err := startActivationWorker(ctx, uploaderUnit, owned); err != nil {
		return err
	}
	uploader := owned[uploaderUnit]
	identity, err := connectedprocess.Audit(ctx, uploader.PID, connectedprofile.ReleaseDirectory+"/"+c.ArtifactSHA256+"/observer-connected-uploader")
	if err != nil || identity.PID != uploader.PID || !sameWorker(ctx, uploaderUnit, uploader) {
		return ErrUnsafe
	}
	config, err := connectedprofile.Encode(c)
	if err != nil {
		return ErrUnsafe
	}
	digest := sha256.Sum256(config)
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return ErrUnsafe
	}
	request := connectedactivation.Request{Version: connectedactivation.RequestVersion, Nonce: hex.EncodeToString(nonce), ArtifactSHA256: c.ArtifactSHA256, ConfigSHA256: hex.EncodeToString(digest[:]), PolicyGeneration: c.PolicyGeneration, IPv4Port: pair.v4.port(), IPv6Port: pair.v6.port()}
	encoded, err := connectedactivation.EncodeRequest(request)
	if err != nil {
		return ErrUnsafe
	}
	if !unchangedActivation(ctx, c, config, uploader) || !pair.denied() {
		return ErrUnsafe
	}
	if publishActivation(d, "request.json", encoded, c.UploaderGID) != nil {
		return ErrRecovery
	}
	var response connectedactivation.Response
	for {
		if !sameWorker(ctx, uploaderUnit, uploader) || !pair.denied() {
			return ErrUnsafe
		}
		response, err = readActivationResponse(c)
		if err == nil {
			if !connectedactivation.MatchResponse(request, uploader.Invocation, response) {
				return ErrUnsafe
			}
			break
		}
		if !os.IsNotExist(err) || activationWait(ctx) != nil {
			return ErrUnsafe
		}
	}
	if err := startActivationWorker(ctx, collectorUnit, owned); err != nil {
		return err
	}
	collector := owned[collectorUnit]
	for {
		if !sameWorker(ctx, collectorUnit, collector) || !sameWorker(ctx, uploaderUnit, uploader) {
			return ErrUnsafe
		}
		record, e := readStatusRecord("collector-status", c.CollectorUID)
		if e == nil && record.InvocationID == collector.Invocation {
			if record.State != "COLLECTING" {
				return ErrUnsafe
			}
			break
		}
		if e != nil && !os.IsNotExist(e) {
			return ErrUnsafe
		}
		if activationWait(ctx) != nil {
			return ErrUnsafe
		}
	}
	if pair.finish(ctx) != nil || !unchangedActivation(ctx, c, config, uploader) || !sameWorker(ctx, collectorUnit, collector) {
		return ErrUnsafe
	}
	commit := connectedactivation.Commit{Version: connectedactivation.CommitVersion, RequestSHA256: response.RequestSHA256, InvocationID: response.InvocationID, Challenge: response.Challenge}
	encoded, err = connectedactivation.EncodeCommit(commit)
	if err != nil || ctx.Err() != nil {
		return ErrUnsafe
	}
	if publishActivation(d, "commit.json", encoded, c.UploaderGID) != nil {
		return ErrRecovery
	}
	for {
		if !sameWorker(ctx, uploaderUnit, uploader) || !sameWorker(ctx, collectorUnit, collector) {
			return ErrUnsafe
		}
		record, e := readStatusRecord("uploader-status", c.UploaderUID)
		if e == nil && record.InvocationID == uploader.Invocation {
			switch record.State {
			case "WAITING_FIRST_UPLOAD", "PENDING", "ACKNOWLEDGED_FRESH":
				return nil
			default:
				return ErrUnsafe
			}
		}
		if e != nil && !os.IsNotExist(e) {
			return ErrUnsafe
		}
		if activationWait(ctx) != nil {
			return ErrUnsafe
		}
	}
}

func unchangedActivation(ctx context.Context, c connectedprofile.Config, encoded []byte, uploader workerInstance) bool {
	current, err := inspectInstalled()
	if err != nil {
		return false
	}
	actual, err := connectedprofile.Encode(current)
	return err == nil && bytes.Equal(actual, encoded) && connectedpolicy.ValidateEffective(ctx, uploaderUnit, c.Addresses) == nil && sameWorker(ctx, uploaderUnit, uploader)
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
