//go:build linux

package connectedstagefs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
)

const journalTempPrefix = ".observer-transition-journal-"
const maxJournalScanEntries = 4096

// PublishFirstJournalAt is an unused fixed first-transition write primitive.
// Its future caller must hold the installation lease, prove stopped workers,
// exact previous active resources, unchanged private ledger and a compatible
// selected package. Neither the stage nor this function starts a worker.
// Complete staged evidence is never automatically aborted after this call.
func PublishFirstJournalAt(ctx context.Context, config *os.File) error {
	return publishFirstJournalAt(ctx, config, nil)
}

func publishFirstJournalAt(ctx context.Context, config *os.File, after func(string) error) error {
	if ctx == nil || ctx.Err() != nil || config == nil {
		return ErrUnsafe
	}
	stage, err := InspectFirstPredecessorAt(config)
	if err != nil {
		return err
	}
	proposal := stage.Files[connectedtransition.StageProposalName]
	sum := sha256.Sum256(proposal)
	temp := journalTempPrefix + hex.EncodeToString(sum[:])
	if err := inspectJournalTempNames(config, temp); err != nil {
		return ErrRecovery
	}
	authority, err := ReadAuthorityAt(config)
	if err != nil || authority.Completion != nil ||
		(authority.Journal != nil && !bytes.Equal(authority.Journal, proposal)) {
		return ErrRecovery
	}
	if err := clearJournalTemp(config, temp); err != nil || ctx.Err() != nil {
		return ErrRecovery
	}
	if authority.Journal != nil {
		// A rename interrupted before its parent sync may leave next bytes
		// visible without a durable directory entry. Sync and reopen on retry.
		if config.Sync() != nil || journalInterrupted(ctx, after, "parent-sync") {
			return ErrRecovery
		}
		return confirmFirstJournal(ctx, config, proposal)
	}
	if journalInterrupted(ctx, after, "before-create") {
		return ErrRecovery
	}
	fd, err := unix.Openat2(int(config.Fd()), temp, &unix.OpenHow{
		Flags: uint64(unix.O_WRONLY | unix.O_CREAT | unix.O_EXCL | unix.O_NOFOLLOW | unix.O_CLOEXEC),
		Mode: 0600,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV,
	})
	if err != nil {
		return ErrRecovery
	}
	file := os.NewFile(uintptr(fd), temp)
	info, statErr := file.Stat()
	owner, ok := infoOwner(info)
	if statErr != nil || !ok || owner.Gid != 0 || file.Chown(0, 0) != nil || file.Chmod(0600) != nil ||
		!regular(file, 0, 0600, connectedtransition.MaxRecordBytes) {
		file.Close()
		return ErrRecovery
	}
	n, writeErr := file.Write(proposal)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || n != len(proposal) || syncErr != nil || closeErr != nil ||
		journalInterrupted(ctx, after, "file-sync") {
		return ErrRecovery
	}
	// The authoritative name was absent before the temporary write. It must
	// still be absent, and the old receipt must still be absent, immediately
	// before rename. Any competing privileged mutation is a stopped refusal.
	before, err := ReadAuthorityAt(config)
	if err != nil || before.Journal != nil || before.Completion != nil || ctx.Err() != nil {
		return ErrRecovery
	}
	if unix.Renameat2(int(config.Fd()), temp, int(config.Fd()), JournalName, unix.RENAME_NOREPLACE) != nil ||
		journalInterrupted(ctx, after, "rename") || config.Sync() != nil ||
		journalInterrupted(ctx, after, "parent-sync") {
		return ErrRecovery
	}
	return confirmFirstJournal(ctx, config, proposal)
}

func journalInterrupted(ctx context.Context, after func(string) error, step string) bool {
	return ctx.Err() != nil || (after != nil && after(step) != nil)
}

func confirmFirstJournal(ctx context.Context, config *os.File, proposal []byte) error {
	observed, err := ReadAuthorityAt(config)
	if err != nil || ctx.Err() != nil || observed.Completion != nil || !bytes.Equal(observed.Journal, proposal) {
		return ErrRecovery
	}
	return nil
}

func inspectJournalTempNames(config *os.File, expected string) error {
	fd, err := unix.Openat2(int(config.Fd()), ".", &unix.OpenHow{
		Flags: uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV,
	})
	if err != nil {
		return ErrRecovery
	}
	dir := os.NewFile(uintptr(fd), "journal-temp-scan")
	defer dir.Close()
	entries, err := dir.ReadDir(maxJournalScanEntries + 1)
	if err != nil || len(entries) > maxJournalScanEntries {
		return ErrRecovery
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == ".install-next" || strings.HasPrefix(name, journalTempPrefix) && name != expected {
			return ErrRecovery
		}
	}
	return nil
}

func clearJournalTemp(config *os.File, name string) error {
	var st unix.Stat_t
	if err := unix.Fstatat(int(config.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW); err == unix.ENOENT {
		return nil
	} else if err != nil {
		return ErrRecovery
	}
	fd, err := openFixedStageMember(int(config.Fd()), name, false)
	if err != nil {
		return ErrRecovery
	}
	file := os.NewFile(uintptr(fd), name)
	valid := regular(file, 0, 0600, connectedtransition.MaxRecordBytes)
	before, statErr := file.Stat()
	closeErr := file.Close()
	owner, ok := infoOwner(before)
	if !valid || statErr != nil || closeErr != nil || !ok || owner.Gid != 0 ||
		owner.Ino != st.Ino || owner.Dev != st.Dev || owner.Nlink != 1 ||
		before.Size() != st.Size || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0777 != 0600 {
		return ErrRecovery
	}
	var current unix.Stat_t
	if unix.Fstatat(int(config.Fd()), name, &current, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		current.Ino != st.Ino || current.Dev != st.Dev || current.Size != st.Size ||
		current.Uid != 0 || current.Gid != 0 || current.Nlink != 1 ||
		current.Mode&unix.S_IFMT != unix.S_IFREG || current.Mode&0777 != 0600 {
		return ErrRecovery
	}
	if unix.Unlinkat(int(config.Fd()), name, 0) != nil || config.Sync() != nil {
		return ErrRecovery
	}
	return nil
}

func infoOwner(info os.FileInfo) (*syscall.Stat_t, bool) {
	if info == nil {
		return nil, false
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	return owner, ok
}
