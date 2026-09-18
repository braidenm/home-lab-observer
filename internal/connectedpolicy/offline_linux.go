//go:build linux

package connectedpolicy

import (
	"context"
	"io"
	"os/exec"
	"time"
)

// ValidateOffline is called after the installer verifies its fresh transient
// nonce and InvocationID, while the process is blocked on an EMPTY stdin pipe.
// It never starts a service or releases input. The installer rechecks invocation
// ownership before sending expected binding data and joins it before promotion.
func ValidateOffline(ctx context.Context, unit string, expected OfflineExpectation) error {
	if ctx == nil || ctx.Err() != nil || unit != enrollmentUnit {
		return ErrUnsafe
	}
	if _, err := offlineExpected(expected); err != nil {
		return ErrUnsafe
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	args := []string{"show", "--no-pager"}
	for _, field := range offlineFields {
		args = append(args, "--property="+field)
	}
	args = append(args, "--", unit)
	cmd := exec.CommandContext(ctx, "/usr/bin/systemctl", args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=cat", "SYSTEMD_PAGERSECURE=1"}
	var output boundedOutput
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	if cmd.Run() != nil || ctx.Err() != nil {
		return ErrUnsafe
	}
	if validateOffline(output.buffer.Bytes(), expected) != nil {
		return ErrUnsafe
	}
	// systemctl renders credential arrays as [unprintable], even when empty.
	// Query their typed D-Bus values instead. Never buffer a credential value.
	const object = "/org/freedesktop/systemd1/unit/home_2dlab_2dobserver_2dconnected_2denrollment_2eservice"
	credentials := exec.CommandContext(ctx, "/usr/bin/busctl", "--system", "--no-pager", "get-property", "org.freedesktop.systemd1", object, "org.freedesktop.systemd1.Service", "LoadCredential", "LoadCredentialEncrypted", "SetCredential", "SetCredentialEncrypted", "ImportCredential")
	credentials.Env = cmd.Env
	var empty emptyCredentialsOutput
	credentials.Stdout = &empty
	credentials.Stderr = io.Discard
	credentials.WaitDelay = time.Second
	if credentials.Run() != nil || ctx.Err() != nil || !empty.complete() {
		return ErrUnsafe
	}
	return nil
}
