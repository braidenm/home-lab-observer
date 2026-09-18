//go:build linux

package connectedstagefs

import (
	"os"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
)

const JournalName = "transition.json"
const CompletionName = "transition-complete"
const LegacyJournalName = "refresh.json"
const LegacyCompletionName = "refresh-complete"

// Authority is a detached read of the two fixed authoritative names. Nil
// means absent; a present empty, linked, broad or foreign file is refused.
// It is evidence only, never permission to resume, abort, clean, or start.
type Authority struct {
	Journal, Completion []byte
}

// ReadAuthorityAt requires an anchored, exact root-owned ext4 config
// directory. The caller holds the installation lease and proves both owned
// workers are stopped before interpreting the returned bytes.
func ReadAuthorityAt(config *os.File) (Authority, error) {
	if !fixedAuthorityParent(config) {
		return Authority{}, ErrUnsafe
	}
	journal, err := readOptionalAuthority(config, JournalName, connectedtransition.MaxRecordBytes)
	if err != nil {
		return Authority{}, ErrRecovery
	}
	completion, err := readOptionalAuthority(config, CompletionName, 256)
	if err != nil {
		return Authority{}, ErrRecovery
	}
	return Authority{Journal: journal, Completion: completion}, nil
}

// LegacyAuthority is the detached read of the two fixed old refresh names.
// Pair coherence and comparison with staged predecessor bytes are separate
// admission steps; absence of these names never proves an initial install.
type LegacyAuthority struct {
	Journal, Completion []byte
}

// ReadLegacyAt preserves absence separately for each name and refuses an
// unsafe present file. It never writes, repairs, or chooses a recovery phase.
func ReadLegacyAt(config *os.File) (LegacyAuthority, error) {
	if !fixedAuthorityParent(config) {
		return LegacyAuthority{}, ErrUnsafe
	}
	journal, err := readOptionalAuthority(config, LegacyJournalName, 8<<10)
	if err != nil {
		return LegacyAuthority{}, ErrRecovery
	}
	completion, err := readOptionalAuthority(config, LegacyCompletionName, 256)
	if err != nil {
		return LegacyAuthority{}, ErrRecovery
	}
	return LegacyAuthority{Journal: journal, Completion: completion}, nil
}

func fixedAuthorityParent(config *os.File) bool {
	if config == nil || os.Getuid() != 0 || os.Geteuid() != 0 || !fixedRootDirectory(config, 0755) {
		return false
	}
	var fs unix.Statfs_t
	return unix.Fstatfs(int(config.Fd()), &fs) == nil && fs.Type == unix.EXT4_SUPER_MAGIC
}

func readOptionalAuthority(config *os.File, name string, limit int) ([]byte, error) {
	var st unix.Stat_t
	err := unix.Fstatat(int(config.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW)
	if err == unix.ENOENT {
		return nil, nil
	}
	if err != nil {
		return nil, ErrUnsafe
	}
	return readFixedStageFile(int(config.Fd()), name, limit)
}
