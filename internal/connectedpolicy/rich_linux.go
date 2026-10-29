//go:build linux

package connectedpolicy

import "context"

func ValidateRichProcessesOffline(ctx context.Context, unit string, expected OfflineExpectation) error {
	if ctx == nil || ctx.Err() != nil || unit != richEnrollmentUnit {
		return ErrUnsafe
	}
	if _, err := offlineExpectedForProfile(expected, true); err != nil {
		return ErrUnsafe
	}
	return inspectTransient(ctx, unit, offlineFields, func(data []byte) error { return validateOfflineForProfile(data, expected, true) })
}

func ValidateRichProcessesOnline(ctx context.Context, unit string, expected OnlineExpectation) error {
	if ctx == nil || ctx.Err() != nil || unit != richEnrollmentUnit {
		return ErrUnsafe
	}
	if _, err := onlineExpectedForProfile(expected, true); err != nil {
		return ErrUnsafe
	}
	return inspectTransient(ctx, unit, onlineFields, func(data []byte) error { return validateOnlineForProfile(data, expected, true) })
}
