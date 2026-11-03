//go:build linux

package actionworker

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionipc"
	"github.com/braidenm/home-lab-observer/internal/actionremote"
)

// RunWorker composes the fixed installed identity, protected binding/credential,
// authenticated local broker and actual fixed-origin TLS client. Observation
// credentials, caller paths and substitute transports cannot enter this API.
// One sequential conversation is a conservative subset of broker capacity.
func RunWorker(ctx context.Context) error {
	if ctx == nil {
		return ErrUnsafe
	}
	if ctx.Err() != nil {
		return nil
	}
	i, err := workerIdentity(ctx)
	if err != nil {
		return ErrUnsafe
	}
	s, err := LoadSettings(ctx)
	if err != nil {
		return ErrUnsafe
	}
	credentials, err := NewActiveCredentials(ctx, s)
	if err != nil {
		return ErrUnsafe
	}
	client, err := actionremote.New(s.Origin, s.Binding, credentials)
	if err != nil {
		return ErrUnsafe
	}
	defer client.Close()
	w := &conversationClient{remote: client, binding: s.Binding}
	return runWorkerLoop(ctx, w, func(ctx context.Context) (*net.UnixConn, error) { return dialBroker(ctx, i) }, workerPace)
}

func workerPace(ctx context.Context) error {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ErrUnsafe
	case <-timer.C:
		return nil
	}
}

// Synthetic socket tests can supply only these private process-boundary seams.
// Production always uses dialBroker, the same conversation/latch and fixed pace.
func runWorkerLoop(ctx context.Context, w *conversationClient, connect func(context.Context) (*net.UnixConn, error), pace func(context.Context) error) error {
	if ctx == nil || w == nil || connect == nil || pace == nil {
		return ErrUnsafe
	}
	for ctx.Err() == nil {
		c, err := connect(ctx)
		if ctx.Err() != nil {
			if c != nil {
				c.Close()
			}
			return nil
		}
		if err != nil {
			if c != nil {
				c.Close()
			}
			if !errors.Is(err, errBrokerAbsent) {
				return ErrUnsafe
			}
		} else {
			if c == nil {
				return ErrUnsafe
			}
			joined := make(chan struct{})
			stop := context.AfterFunc(ctx, func() { c.Close(); close(joined) })
			err = w.converse(ctx, c)
			if !stop() {
				<-joined
			}
			c.Close()
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, errInitialSelection) || err != nil && !w.executionDisabled() && !errors.Is(err, actionipc.ErrNoSelection) {
				return ErrUnsafe
			}
		}
		if pace(ctx) != nil {
			if ctx.Err() != nil {
				return nil
			}
			return ErrUnsafe
		}
	}
	return nil
}
