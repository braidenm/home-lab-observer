//go:build linux

package actionworker

import (
	"context"
	"time"
)

// Credential maintenance has no Engine/COMMIT permit. Its original invocation
// budget uses the existing worker clock and cannot be restarted by a reply.
type credentialOperation struct {
	ctx     context.Context
	cancel  context.CancelFunc
	start   nativeStamp
	capture func() (nativeStamp, error)
}

func newCredentialOperation(parent context.Context) (*credentialOperation, error) {
	if parent == nil || parent.Err() != nil {
		return nil, ErrUnsafe
	}
	start, err := captureStamp()
	if err != nil || parent.Err() != nil {
		return nil, ErrUnsafe
	}
	ctx, cancel := context.WithDeadline(parent, start.at.Add(30*time.Second))
	return &credentialOperation{ctx, cancel, start, captureStamp}, nil
}
func (o *credentialOperation) close() {
	if o != nil && o.cancel != nil {
		o.cancel()
	}
}
func (o *credentialOperation) now() (nativeStamp, error) {
	if o == nil || o.ctx == nil || o.capture == nil || o.ctx.Err() != nil {
		return nativeStamp{}, ErrUnsafe
	}
	now, err := o.capture()
	if err != nil || o.ctx.Err() != nil {
		return nativeStamp{}, ErrUnsafe
	}
	if _, err := elapsed(o.start, now, 30*time.Second); err != nil {
		return nativeStamp{}, ErrUnsafe
	}
	return now, nil
}
func (o *credentialOperation) check() error { _, err := o.now(); return err }

func (o *credentialOperation) checkRemote(start nativeStamp) error {
	now, err := o.now()
	if err != nil {
		return err
	}
	_, err = elapsed(start, now, 5*time.Second)
	return err
}
