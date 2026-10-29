//go:build linux

package connectedpolicy

import "context"

func ValidateRichProcessesDockerOffline(ctx context.Context, unit string, e OfflineExpectation) error {
	if ctx == nil || ctx.Err() != nil || unit != dockerEnrollmentUnit {
		return ErrUnsafe
	}
	want, err := dockerOfflineExpected(e)
	if err != nil {
		return ErrUnsafe
	}
	return inspectTransient(ctx, unit, offlineFields, func(b []byte) error { return validateOfflineAgainst(b, want) })
}
func ValidateRichProcessesDockerOnline(ctx context.Context, unit string, e OnlineExpectation) error {
	if ctx == nil || ctx.Err() != nil || unit != dockerEnrollmentUnit {
		return ErrUnsafe
	}
	want, err := dockerOnlineExpected(e)
	if err != nil {
		return ErrUnsafe
	}
	return inspectTransient(ctx, unit, onlineFields, func(b []byte) error { return validateOnlineAgainst(b, want) })
}
