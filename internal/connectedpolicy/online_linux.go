//go:build linux

package connectedpolicy

import "context"

// ValidateOnline checks the actual transient enrollment UID, mounts,
// confinement and empty credential arrays before the installer writes its
// one-use grant. ValidateEffective separately checks exact endpoint/slice IP
// policy. Neither check starts a unit or releases any input.
func ValidateOnline(ctx context.Context, unit string, expected OnlineExpectation) error {
	if ctx == nil || ctx.Err() != nil || unit != enrollmentUnit {
		return ErrUnsafe
	}
	if _, err := onlineExpected(expected); err != nil {
		return ErrUnsafe
	}
	return inspectTransient(ctx, unit, onlineFields, func(data []byte) error {
		return validateOnline(data, expected)
	})
}
