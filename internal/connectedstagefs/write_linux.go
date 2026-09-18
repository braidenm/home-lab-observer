//go:build linux

package connectedstagefs

import (
	"bytes"
	"context"
	"os"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
)

// PublishAt only prepares fixed root-owned evidence. It does not replace the
// active journal or authorize workers. Its caller must hold the installation
// lease, prove stopped owned workers and predecessor/ledger identity, and
// fence start/refresh on any preparation residue. No command calls it yet.
func PublishAt(ctx context.Context, config *os.File, preparation []byte, files map[string][]byte) error {
	return publishAt(ctx, config, preparation, files, nil)
}

func publishAt(
	ctx context.Context,
	config *os.File,
	preparation []byte,
	files map[string][]byte,
	afterSync func(string) error,
) error {
	if ctx == nil || ctx.Err() != nil || config == nil || os.Getuid() != 0 || os.Geteuid() != 0 ||
		!fixedRootDirectory(config, 0755) {
		return ErrUnsafe
	}
	if _, err := connectedtransition.ValidateStage(preparation, files); err != nil {
		return ErrUnsafe
	}
	var filesystem unix.Statfs_t
	if unix.Fstatfs(int(config.Fd()), &filesystem) != nil || filesystem.Type != unix.EXT4_SUPER_MAGIC ||
		!fixedStageNameAbsent(int(config.Fd()), transitionPreparationName) ||
		!fixedStageNameAbsent(int(config.Fd()), transitionStageName) {
		return ErrRecovery
	}
	if err := writeFixedStageFile(config, transitionPreparationName, preparation, connectedtransition.MaxPreparationBytes); err != nil {
		return ErrRecovery
	}
	if interruptedAfterSync(ctx, afterSync, "preparation") {
		return ErrRecovery
	}
	if unix.Mkdirat(int(config.Fd()), transitionStageName, 0700) != nil {
		return ErrRecovery
	}
	fd, err := openFixedStageMember(int(config.Fd()), transitionStageName, true)
	if err != nil {
		return ErrRecovery
	}
	stage := os.NewFile(uintptr(fd), transitionStageName)
	defer stage.Close()
	if stage.Chown(0, 0) != nil || stage.Chmod(0700) != nil || !fixedRootDirectory(stage, 0700) {
		return ErrRecovery
	}
	var configStat, stageStat unix.Stat_t
	var stageFS unix.Statfs_t
	if unix.Fstat(int(config.Fd()), &configStat) != nil || unix.Fstat(int(stage.Fd()), &stageStat) != nil ||
		configStat.Dev != stageStat.Dev || unix.Fstatfs(int(stage.Fd()), &stageFS) != nil ||
		stageFS.Type != unix.EXT4_SUPER_MAGIC || stage.Sync() != nil || config.Sync() != nil {
		return ErrRecovery
	}
	if interruptedAfterSync(ctx, afterSync, "stage-directory") {
		return ErrRecovery
	}
	for _, name := range []string{
		connectedtransition.StageOldCAName,
		connectedtransition.StageNewCAName,
		connectedtransition.StagePredecessorName,
		connectedtransition.StagePredecessorCompletionName,
		connectedtransition.StageProposalName,
	} {
		data, present := files[name]
		if !present {
			continue
		}
		if ctx.Err() != nil || writeFixedStageFile(stage, name, data, stageFileLimit(name)) != nil ||
			interruptedAfterSync(ctx, afterSync, name) {
			return ErrRecovery
		}
	}
	observed, err := inspectAt(config, stage)
	if err != nil || !bytes.Equal(observed.Preparation, preparation) || len(observed.Files) != len(files) {
		return ErrRecovery
	}
	for name, expected := range files {
		if !bytes.Equal(observed.Files[name], expected) {
			return ErrRecovery
		}
	}
	return nil
}

func interruptedAfterSync(ctx context.Context, hook func(string) error, step string) bool {
	if ctx.Err() != nil {
		return true
	}
	return hook != nil && hook(step) != nil
}

func fixedStageNameAbsent(parent int, name string) bool {
	var stat unix.Stat_t
	return unix.Fstatat(parent, name, &stat, unix.AT_SYMLINK_NOFOLLOW) == unix.ENOENT
}

func stageFileLimit(name string) int {
	switch name {
	case connectedtransition.StageOldCAName, connectedtransition.StageNewCAName:
		return connectedtransition.MaxCABytes
	case connectedtransition.StagePredecessorCompletionName:
		return 256
	case connectedtransition.StageProposalName, connectedtransition.StagePredecessorName:
		return connectedtransition.MaxRecordBytes
	default:
		return 0
	}
}

func writeFixedStageFile(parent *os.File, name string, data []byte, limit int) error {
	expectedLimit := stageFileLimit(name)
	if name == transitionPreparationName {
		expectedLimit = connectedtransition.MaxPreparationBytes
	}
	if expectedLimit == 0 || limit != expectedLimit || len(data) == 0 || len(data) > limit {
		return ErrUnsafe
	}
	fd, err := unix.Openat2(int(parent.Fd()), name, &unix.OpenHow{
		Flags: uint64(unix.O_WRONLY | unix.O_CREAT | unix.O_EXCL | unix.O_NOFOLLOW | unix.O_CLOEXEC),
		Mode: 0600,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV,
	})
	if err != nil {
		return ErrRecovery
	}
	file := os.NewFile(uintptr(fd), name)
	if file.Chown(0, 0) != nil || file.Chmod(0600) != nil || !regular(file, 0, 0600, 0) {
		file.Close()
		return ErrRecovery
	}
	info, statErr := file.Stat()
	if statErr != nil {
		file.Close()
		return ErrRecovery
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Gid != 0 {
		file.Close()
		return ErrRecovery
	}
	n, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || n != len(data) || syncErr != nil || closeErr != nil || parent.Sync() != nil {
		return ErrRecovery
	}
	return nil
}
