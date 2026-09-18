//go:build linux

package connectedinstall

import (
	"context"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedstagefs"
)

// Start creates a new, freshly checked worker invocation under the installation
// lease. An already running owned pair is stopped first; no cached proof is used.
// Exact packaged disposable-VM acceptance remains a release prerequisite.
func Start(ctx context.Context) error { return startInstalled(ctx) }

// Restart repeats the same full checks as Start, preserving credentials and the
// durable ledger. It does not enable automatic restart or boot activation.
func Restart(ctx context.Context) error { return startInstalled(ctx) }

func startInstalled(ctx context.Context) error {
	return startInstalledWith(ctx, startOperations{
		lease:     func() (io.Closer, error) { return acquireLease() },
		preflight: preflight, inspect: inspectInstalled, markers: rejectRecoveryMarkers,
		audit: auditPrincipals, stop: stopOwnedWorkers, activate: activateStopped,
	})
}

// startOperations is a private test boundary, never a caller-supplied policy or
// command surface. Production uses only the fixed adapters above.
type startOperations struct {
	lease     func() (io.Closer, error)
	preflight func(context.Context) error
	inspect   func() (connectedprofile.Config, error)
	markers   func() error
	audit     func(context.Context, connectedprofile.Config) error
	stop      func(context.Context) error
	activate  func(context.Context, connectedprofile.Config) error
}

func startInstalledWith(parent context.Context, ops startOperations) (result error) {
	if parent == nil || parent.Err() != nil {
		return ErrUnsafe
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	lock, err := ops.lease()
	if err != nil {
		return err
	}
	defer func() {
		if lock.Close() != nil {
			result = ErrRecovery
		}
	}()
	if ops.markers() != nil {
		return ErrRecovery
	}
	c, err := ops.inspect()
	if err != nil {
		return err
	}
	// Once exact ownership is established, stop before checking principal/host
	// drift: failed admission must not leave an old owned invocation running.
	if err := ops.stop(ctx); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ErrUnsafe
	}
	if ops.preflight(ctx) != nil || ops.audit(ctx, c) != nil || ctx.Err() != nil {
		return ErrUnsafe
	}
	return ops.activate(ctx, c)
}

func rejectRecoveryMarkers() error {
	parent, err := rootDirectory(connectedprofile.ConfigDirectory)
	if err != nil {
		return ErrUnsafe
	}
	defer parent.Close()
	return rejectRecoveryMarkersAt(parent)
}

func rejectRecoveryMarkersAt(parent *os.File) error {
	if parent == nil {
		return ErrUnsafe
	}
	for _, name := range []string{
		"uninstalling.json", "uninstalled.json",
		connectedstagefs.PreparationName, connectedstagefs.StageName,
	} {
		// Any marker type, including a dangling symlink, refuses normal work.
		var stat unix.Stat_t
		if unix.Fstatat(int(parent.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW) != unix.ENOENT {
			return ErrRecovery
		}
	}
	return nil
}
