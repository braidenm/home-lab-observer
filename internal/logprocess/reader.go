package logprocess

import (
	"context"
	"errors"
	"runtime"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
	"github.com/braidenm/home-lab-observer/internal/logprotocol"
)

// Reader owns one non-queuing helper slot across all configured sources. It must
// be shared by the runtime's single log collector, not recreated for each poll.
type Reader struct {
	build     logprotocol.Build
	transport *transport
	now       func() time.Time
}

var _ logobs.Reader = (*Reader)(nil)

// NewReader accepts only code-owned release identity, never a command or path.
// Construction performs no filesystem, process or native acquisition operation.
func NewReader(version, commit, linuxSHA256 string) *Reader {
	build := logprotocol.Build{Version: version, Commit: commit, OS: runtime.GOOS, Arch: runtime.GOARCH}
	return &Reader{build: build, now: time.Now, transport: newTransport(func() (commandSpec, error) { return resolveHelper(build, linuxSHA256) })}
}

func (r *Reader) Read(ctx context.Context, request logobs.ReadRequest) (logobs.Batch, error) {
	if ctx == nil || request.Validate() != nil {
		return logobs.Batch{}, errInputInvalid
	}
	if ctx.Err() != nil {
		return logobs.Batch{}, ctx.Err()
	}
	request = request.Clone()
	started := r.now().UTC()
	if r.build.OS != "linux" && r.build.OS != "windows" {
		return r.unavailable(request, started, logobs.SupportUnsupported, logobs.ReasonPlatformUnsupported)
	}
	if r.build.OS == "linux" && request.Source != logobs.SourceSystem {
		return logobs.Batch{}, errInputInvalid
	}
	input, err := logprotocol.EncodeRequest(r.build, request)
	if err != nil {
		return r.unavailable(request, started, logobs.SupportUnavailable, logobs.ReasonLogHelperMismatch)
	}
	output, err := r.transport.exchange(ctx, input)
	if ctx.Err() != nil {
		return logobs.Batch{}, ctx.Err()
	}
	if errors.Is(err, ErrHelperUnavailable) {
		return r.unavailable(request, started, logobs.SupportUnavailable, logobs.ReasonLogHelperUnavailable)
	}
	if errors.Is(err, ErrHelperMismatch) {
		return r.unavailable(request, started, logobs.SupportUnavailable, logobs.ReasonLogHelperMismatch)
	}
	if err != nil {
		// Without a valid helper batch we cannot claim native support, progress,
		// or a specific native failure. The collector records its fixed failed
		// attempt, preserving durable history and the private checkpoint.
		return logobs.Batch{}, errProcessFailed
	}
	batch, err := logprotocol.DecodeResponse(output, r.build, request)
	if ctx.Err() != nil {
		return logobs.Batch{}, ctx.Err()
	}
	if errors.Is(err, logprotocol.ErrIdentityMismatch) {
		return r.unavailable(request, started, logobs.SupportUnavailable, logobs.ReasonLogHelperMismatch)
	}
	if err != nil {
		return logobs.Batch{}, errProcessFailed
	}
	return batch, nil
}

func (r *Reader) unavailable(request logobs.ReadRequest, started time.Time, support logobs.SupportState, reason logobs.ReasonCode) (logobs.Batch, error) {
	batch := logobs.Batch{Kind: logobs.BatchNormal, Source: request.Source, ExpectedRevision: request.Checkpoint.Revision,
		QueryStartedAt: request.QueryStartedAt, StartedAt: started, FinishedAt: r.now().UTC(), SupportState: support, CollectionState: logobs.CollectionNotRun, ReasonCode: &reason}
	if request.Checkpoint.ResetPending {
		batch.Kind = logobs.BatchResetPending
	}
	if batch.Validate() != nil {
		return logobs.Batch{}, errProcessFailed
	}
	return batch, nil
}
