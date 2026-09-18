//go:build linux

package connectedstagefs

import (
	"os"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
)

const JournalName = "transition.json"
const CompletionName = "transition-complete"

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
	if config == nil || os.Getuid() != 0 || os.Geteuid() != 0 || !fixedRootDirectory(config, 0755) {
		return Authority{}, ErrUnsafe
	}
	var fs unix.Statfs_t
	if unix.Fstatfs(int(config.Fd()), &fs) != nil || fs.Type != unix.EXT4_SUPER_MAGIC {
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
