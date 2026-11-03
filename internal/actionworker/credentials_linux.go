//go:build linux

package actionworker

import (
	"bytes"
	"context"
	"io"
	"os"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionprofile"
	"github.com/braidenm/home-lab-observer/internal/actionremote"
	"golang.org/x/sys/unix"
)

type protectedPart struct {
	name           string
	uid, gid, mode uint32
}

// ActiveCredentials is a fixed-path reader, not an enrollment or rotation
// mechanism. There is no path/identity/secret override in its public API.
type ActiveCredentials struct {
	binding  actionremote.Binding
	identity actionprofile.Identity
}

func workerIdentity(ctx context.Context) (actionprofile.Identity, error) {
	if ctx == nil || ctx.Err() != nil {
		return actionprofile.Identity{}, ErrUnsafe
	}
	identity, err := actionprofile.VerifyWorker()
	if err != nil || unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0) != nil || ctx.Err() != nil {
		return actionprofile.Identity{}, ErrUnsafe
	}
	return identity, nil
}

func LoadSettings(ctx context.Context) (Settings, error) {
	i, err := workerIdentity(ctx)
	if err != nil {
		return Settings{}, ErrUnsafe
	}
	raw, err := readProtected(ctx, settingsParts(i))
	defer clear(raw)
	if err != nil {
		return Settings{}, ErrUnsafe
	}
	settings, err := decodeSettings(raw)
	if err != nil || ctx.Err() != nil {
		return Settings{}, ErrUnsafe
	}
	return settings, nil
}

func NewActiveCredentials(ctx context.Context, settings Settings) (*ActiveCredentials, error) {
	i, err := workerIdentity(ctx)
	if err != nil {
		return nil, ErrUnsafe
	}
	client, err := actionremote.New(settings.Origin, settings.Binding, unavailableCredential{})
	if err != nil {
		return nil, ErrUnsafe
	}
	if client.Close() != nil {
		return nil, ErrUnsafe
	}
	return &ActiveCredentials{binding: settings.Binding, identity: i}, nil
}

func (p *ActiveCredentials) Credential(ctx context.Context, binding actionremote.Binding) (actionremote.Credential, error) {
	if p == nil || p.binding != binding {
		return actionremote.Credential{}, ErrUnsafe
	}
	i, err := workerIdentity(ctx)
	if err != nil || i != p.identity {
		return actionremote.Credential{}, ErrUnsafe
	}
	raw, err := readProtected(ctx, activeParts(i))
	defer clear(raw)
	if err != nil {
		return actionremote.Credential{}, ErrUnsafe
	}
	credential, err := decodeActive(raw, binding, time.Now().UTC())
	if err != nil || ctx.Err() != nil {
		return actionremote.Credential{}, ErrUnsafe
	}
	return credential, nil
}

func settingsParts(i actionprofile.Identity) []protectedPart {
	return []protectedPart{{"etc", 0, 0, 0}, {"home-lab-observer-actions", 0, i.IPCGID, 0750}, {"worker.json", 0, i.WorkerGID, 0640}}
}

func activeParts(i actionprofile.Identity) []protectedPart {
	return []protectedPart{{"var", 0, 0, 0}, {"lib", 0, 0, 0}, {"home-lab-observer-actions", 0, 0, 0755}, {"worker", i.WorkerUID, i.WorkerGID, 0700}, {"active.json", i.WorkerUID, i.WorkerGID, 0600}}
}

func noACL(fd int) bool {
	for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
		n, err := unix.Fgetxattr(fd, name, nil)
		if err == unix.ENODATA || err == unix.ENOTSUP {
			continue
		}
		if err != nil || n != 0 {
			return false
		}
	}
	return true
}

func sameWitness(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Uid == b.Uid && a.Gid == b.Gid && a.Nlink == b.Nlink && a.Size == b.Size && a.Mtim == b.Mtim && a.Ctim == b.Ctim
}

// readProtected retains every opened ancestor through the bounded read, then
// rechecks both each descriptor and its entry in the still-open parent. A
// changed path is never silently accepted as the originally approved record.
func readProtected(ctx context.Context, parts []protectedPart) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil || len(parts) < 1 || len(parts) > 5 {
		return nil, ErrUnsafe
	}
	root, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrUnsafe
	}
	var stat unix.Stat_t
	if unix.Fstat(root, &stat) != nil || stat.Uid != 0 || stat.Gid != 0 {
		unix.Close(root)
		return nil, ErrUnsafe
	}
	return readProtectedAt(ctx, root, parts)
}

// The private descriptor seam permits synthetic unprivileged file tests; the
// only production caller opens the fixed root and supplies the two closed plans.
func readProtectedAt(ctx context.Context, root int, parts []protectedPart) ([]byte, error) {
	return readProtectedAtObserved(ctx, root, parts, nil)
}

// The private observer exercises a read-window mutation in owned fixtures;
// production always supplies nil and retains the fixed root and closed plans.
func readProtectedAtObserved(ctx context.Context, root int, parts []protectedPart, afterRead func()) ([]byte, error) {
	fds := []int{root}
	defer func() {
		for _, fd := range fds {
			unix.Close(fd)
		}
	}()
	if ctx == nil || ctx.Err() != nil || len(parts) < 1 || len(parts) > 5 {
		return nil, ErrUnsafe
	}
	var rootStat unix.Stat_t
	if unix.Fstat(root, &rootStat) != nil || rootStat.Mode&unix.S_IFMT != unix.S_IFDIR || rootStat.Mode&07022 != 0 || !noACL(root) {
		return nil, ErrUnsafe
	}
	// Production root is host root. Tests use an exclusively owned temp root.
	if rootStat.Uid != 0 && rootStat.Uid != uint32(os.Geteuid()) {
		return nil, ErrUnsafe
	}
	witnesses := []unix.Stat_t{rootStat}
	for index, part := range parts {
		if ctx.Err() != nil || part.name == "" || part.name == "." || part.name == ".." {
			return nil, ErrUnsafe
		}
		for _, c := range part.name {
			if c == '/' || c == 0 {
				return nil, ErrUnsafe
			}
		}
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
		if index < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		fd, err := unix.Openat(fds[len(fds)-1], part.name, flags, 0)
		if err != nil {
			return nil, ErrUnsafe
		}
		fds = append(fds, fd)
		var stat unix.Stat_t
		if unix.Fstat(fd, &stat) != nil || stat.Uid != part.uid || stat.Gid != part.gid || !noACL(fd) || stat.Mode&07000 != 0 {
			return nil, ErrUnsafe
		}
		if part.mode == 0 {
			if stat.Mode&0022 != 0 {
				return nil, ErrUnsafe
			}
		} else if stat.Mode&07777 != part.mode {
			return nil, ErrUnsafe
		}
		if index < len(parts)-1 {
			if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
				return nil, ErrUnsafe
			}
		} else if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Size < 1 || stat.Size > documentLimit {
			return nil, ErrUnsafe
		}
		witnesses = append(witnesses, stat)
	}
	// Dup gives os.File sole ownership without invalidating the witnessed FD.
	dup, err := unix.FcntlInt(uintptr(fds[len(fds)-1]), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, ErrUnsafe
	}
	f := os.NewFile(uintptr(dup), "action-worker-record")
	raw, readErr := io.ReadAll(io.LimitReader(f, documentLimit+1))
	if afterRead != nil {
		afterRead()
	}
	// File timestamps can be coarse even on an otherwise supported filesystem.
	// Require a second consistent bounded capture, not just matching metadata.
	var verification []byte
	var verificationErr error
	if readErr == nil && ctx.Err() == nil && len(raw) <= documentLimit {
		verification, verificationErr = io.ReadAll(io.NewSectionReader(f, 0, documentLimit+1))
	} else {
		verificationErr = ErrUnsafe
	}
	defer clear(verification)
	closeErr := f.Close()
	if readErr != nil || verificationErr != nil || !bytes.Equal(raw, verification) || closeErr != nil || len(raw) < 1 || len(raw) > documentLimit || int64(len(raw)) != witnesses[len(witnesses)-1].Size || ctx.Err() != nil {
		clear(raw)
		return nil, ErrUnsafe
	}
	for index, fd := range fds {
		var current unix.Stat_t
		if unix.Fstat(fd, &current) != nil || !sameWitness(current, witnesses[index]) || !noACL(fd) {
			clear(raw)
			return nil, ErrUnsafe
		}
		if index > 0 {
			if unix.Fstatat(fds[index-1], parts[index-1].name, &current, unix.AT_SYMLINK_NOFOLLOW) != nil || !sameWitness(current, witnesses[index]) {
				clear(raw)
				return nil, ErrUnsafe
			}
		}
	}
	if ctx.Err() != nil {
		clear(raw)
		return nil, ErrUnsafe
	}
	return raw, nil
}
