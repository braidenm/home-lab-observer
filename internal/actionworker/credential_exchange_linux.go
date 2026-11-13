//go:build linux

package actionworker

import (
	"context"
	"strings"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionremote"
	"golang.org/x/sys/unix"
)

// RunExchange is only the fixed WORKER-role bootstrap. Package management and
// broker provisioning are not implemented or inferred from a root invocation.
func RunExchange(ctx context.Context) error {
	op, err := newCredentialOperation(ctx)
	if err != nil {
		return ErrCredentialRecovery
	}
	defer op.close()
	if _, err := workerIdentity(op.ctx); err != nil {
		return ErrCredentialRecovery
	}
	s, err := LoadSettings(op.ctx)
	if err != nil || op.check() != nil {
		return ErrCredentialRecovery
	}
	l, err := openCredentialLifecycle(op.ctx, s)
	if err != nil {
		return ErrCredentialRecovery
	}
	defer l.close()
	h, raw, err := l.store.load(op.ctx)
	clear(raw)
	if err != nil || h.Phase != phaseUnenrolled {
		return ErrCredentialRecovery
	}
	secret, err := readEnrollment(op)
	if err != nil {
		return ErrCredentialRecovery
	}
	return l.exchange(op, secret)
}
func readEnrollment(op *credentialOperation) (actionremote.EnrollmentSecret, error) {
	var before, after unix.Stat_t
	if op.check() != nil || unix.Fstat(0, &before) != nil || before.Mode&unix.S_IFMT != unix.S_IFIFO {
		return actionremote.EnrollmentSecret{}, ErrUnsafe
	}
	// Opening this fixed proc descriptor creates an independently nonblocking
	// file description; dup/F_SETFL would change the caller's pipe flags.
	fd, err := unix.Open("/proc/self/fd/0", unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return actionremote.EnrollmentSecret{}, ErrUnsafe
	}
	defer unix.Close(fd)
	if unix.Fstat(fd, &after) != nil || !samePipe(before, after) || unix.Fstat(0, &after) != nil || !samePipe(before, after) {
		return actionremote.EnrollmentSecret{}, ErrUnsafe
	}
	return consumeEnrollment(op, fd)
}
func samePipe(a, b unix.Stat_t) bool {
	return a.Mode&unix.S_IFMT == unix.S_IFIFO && b.Mode&unix.S_IFMT == unix.S_IFIFO && a.Dev == b.Dev && a.Ino == b.Ino && a.Uid == b.Uid && a.Gid == b.Gid
}
func consumeEnrollment(op *credentialOperation, fd int) (actionremote.EnrollmentSecret, error) {
	var witness unix.Stat_t
	if unix.Fstat(fd, &witness) != nil || witness.Mode&unix.S_IFMT != unix.S_IFIFO {
		return actionremote.EnrollmentSecret{}, ErrUnsafe
	}
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_NONBLOCK == 0 {
		return actionremote.EnrollmentSecret{}, ErrUnsafe
	}
	buf := make([]byte, 83)
	defer clear(buf)
	count := 0
	for {
		now, err := op.now()
		if err != nil {
			return actionremote.EnrollmentSecret{}, ErrUnsafe
		}
		if _, err := elapsed(op.start, now, 5*time.Second); err != nil {
			return actionremote.EnrollmentSecret{}, ErrUnsafe
		}
		var current unix.Stat_t
		if unix.Fstat(fd, &current) != nil || !samePipe(witness, current) {
			return actionremote.EnrollmentSecret{}, ErrUnsafe
		}
		n, readErr := unix.Read(fd, buf[count:])
		if n > 0 {
			count += n
			if count > 82 {
				return actionremote.EnrollmentSecret{}, ErrUnsafe
			}
		}
		if readErr != nil && readErr != unix.EAGAIN && readErr != unix.EINTR {
			return actionremote.EnrollmentSecret{}, ErrUnsafe
		}
		if n == 0 && readErr == nil {
			now, err = op.now()
			if err != nil {
				return actionremote.EnrollmentSecret{}, ErrUnsafe
			}
			if _, err := elapsed(op.start, now, 5*time.Second); err != nil {
				return actionremote.EnrollmentSecret{}, ErrUnsafe
			}
			text := string(buf[:count])
			if len(text) == 82 && strings.HasSuffix(text, "\n") {
				text = text[:81]
			}
			secret, err := actionremote.NewEnrollmentSecret(text)
			if err != nil {
				return actionremote.EnrollmentSecret{}, ErrUnsafe
			}
			return secret, nil
		}
		if _, err := unix.Poll([]unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN | unix.POLLHUP}}, 25); err != nil && err != unix.EINTR {
			return actionremote.EnrollmentSecret{}, ErrUnsafe
		}
	}
}
