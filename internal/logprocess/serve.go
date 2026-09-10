package logprocess

import (
	"context"
	"io"
	"time"

	"github.com/braidenm/home-lab-observer/internal/logobs"
	"github.com/braidenm/home-lab-observer/internal/logprotocol"
)

// HelperConfig is supplied by a fixed native entrypoint, never decoded from the
// private request. Open must not acquire native state until called after Harden.
type HelperConfig struct {
	Build  logprotocol.Build
	Harden func() error
	Open   func() (logobs.Reader, func(), error)
}

// Serve processes exactly one bounded private request. The caller maps a nonzero
// result directly to process exit without logging arguments, input or errors.
// Linux entrypoints must supply journalruntime.Harden, before native loading.
func Serve(parent context.Context, config HelperConfig, input io.Reader, output io.Writer) int {
	if parent == nil || config.Harden == nil || config.Open == nil || input == nil || output == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(parent, sourceTimeout)
	defer cancel()
	if ctx.Err() != nil || config.Harden() != nil || ctx.Err() != nil {
		return 1
	}
	payload, err := io.ReadAll(io.LimitReader(input, logprotocol.MaxRequestBytes+1))
	if err != nil || ctx.Err() != nil {
		return 1
	}
	request, err := logprotocol.DecodeRequest(payload, config.Build)
	if err != nil {
		return 1
	}
	reader, closeReader, openErr := config.Open()
	closed := false
	closeOnce := func() {
		if !closed {
			closed = true
			if closeReader != nil {
				closeReader()
			}
		}
	}
	defer closeOnce()
	var batch logobs.Batch
	if ctx.Err() != nil {
		return 1
	}
	if openErr != nil || reader == nil {
		reason := logobs.ReasonLogHelperUnavailable
		at := time.Now().UTC()
		batch = logobs.Batch{Kind: logobs.BatchNormal, Source: request.Source, ExpectedRevision: request.Checkpoint.Revision,
			QueryStartedAt: request.QueryStartedAt, StartedAt: at, FinishedAt: at,
			SupportState: logobs.SupportUnavailable, CollectionState: logobs.CollectionNotRun, ReasonCode: &reason}
		if request.Checkpoint.ResetPending {
			batch.Kind = logobs.BatchResetPending
		}
	} else {
		batch, err = reader.Read(ctx, request)
		if err != nil {
			return 1
		}
	}
	closeOnce()
	if ctx.Err() != nil {
		return 1
	}
	response, err := logprotocol.EncodeResponse(config.Build, request, batch)
	if err != nil || ctx.Err() != nil {
		return 1
	}
	n, err := output.Write(response)
	if err != nil || n != len(response) || ctx.Err() != nil {
		return 1
	}
	return 0
}
