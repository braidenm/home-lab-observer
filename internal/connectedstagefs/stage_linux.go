//go:build linux

package connectedstagefs

import (
	"errors"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
)

const StageName = "transition-stage"
const PreparationName = "transition-preparing.json"
const transitionStageName = StageName
const transitionPreparationName = PreparationName

var ErrUnsafe = errors.New("connected_stage_unsafe")
var ErrRecovery = errors.New("connected_stage_recovery_required")

type Snapshot struct {
	Preparation []byte
	Files       map[string][]byte
	Record      connectedtransition.Record
}

// InspectAt reads only fixed private evidence beneath the caller's anchored
// config directory. It does not
// authorize cleanup, resume, replacement or worker startup. The caller must
// hold the installation lease and separately prove the stopped-worker state.
func InspectAt(config *os.File) (Snapshot, error) {
	if config == nil || os.Getuid() != 0 || os.Geteuid() != 0 {
		return Snapshot{}, ErrUnsafe
	}
	if !fixedRootDirectory(config, 0755) {
		return Snapshot{}, ErrUnsafe
	}
	fd, err := openFixedStageMember(int(config.Fd()), StageName, true)
	if err != nil {
		return Snapshot{}, ErrRecovery
	}
	stage := os.NewFile(uintptr(fd), StageName)
	defer stage.Close()
	return inspectAt(config, stage)
}

// Openat2 refuses symlinks and mount crossings relative to the verified
// directory handle, including a bind mount on the same device.
func openFixedStageMember(parent int, name string, directory bool) (int, error) {
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if directory {
		flags |= unix.O_DIRECTORY
	}
	return unix.Openat2(parent, name, &unix.OpenHow{
		Flags:   uint64(flags),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV,
	})
}

func inspectAt(config, stage *os.File) (Snapshot, error) {
	if !fixedRootDirectory(config, 0755) || !fixedRootDirectory(stage, 0700) {
		return Snapshot{}, ErrUnsafe
	}
	var configStat, stageStat unix.Stat_t
	var configFS, stageFS unix.Statfs_t
	if unix.Fstat(int(config.Fd()), &configStat) != nil || unix.Fstat(int(stage.Fd()), &stageStat) != nil ||
		configStat.Dev != stageStat.Dev ||
		unix.Fstatfs(int(config.Fd()), &configFS) != nil || unix.Fstatfs(int(stage.Fd()), &stageFS) != nil ||
		configFS.Type != unix.EXT4_SUPER_MAGIC || stageFS.Type != unix.EXT4_SUPER_MAGIC {
		return Snapshot{}, ErrUnsafe
	}
	entries, err := stage.ReadDir(6)
	if err != nil && err != io.EOF || len(entries) < 3 || len(entries) > 5 {
		return Snapshot{}, ErrUnsafe
	}
	limits := map[string]int{
		connectedtransition.StageProposalName:              connectedtransition.MaxRecordBytes,
		connectedtransition.StageOldCAName:                 connectedtransition.MaxCABytes,
		connectedtransition.StageNewCAName:                 connectedtransition.MaxCABytes,
		connectedtransition.StagePredecessorName:           connectedtransition.MaxRecordBytes,
		connectedtransition.StagePredecessorCompletionName: 256,
	}
	files := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		limit, allowed := limits[name]
		if !allowed || entry.IsDir() {
			return Snapshot{}, ErrUnsafe
		}
		data, err := readFixedStageFile(int(stage.Fd()), name, limit)
		if err != nil {
			return Snapshot{}, ErrUnsafe
		}
		files[name] = data
	}
	preparation, err := readFixedStageFile(int(config.Fd()), PreparationName, connectedtransition.MaxPreparationBytes)
	if err != nil {
		return Snapshot{}, ErrUnsafe
	}
	record, err := connectedtransition.ValidateStage(preparation, files)
	if err != nil {
		return Snapshot{}, ErrRecovery
	}
	return Snapshot{Preparation: preparation, Files: files, Record: record}, nil
}

func fixedRootDirectory(file *os.File, mode os.FileMode) bool {
	info, err := file.Stat()
	if err != nil || !info.IsDir() || info.Mode().Perm() != mode ||
		info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || !noACL(int(file.Fd())) {
		return false
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	return ok && owner.Uid == 0 && owner.Gid == 0
}

func readFixedStageFile(parent int, name string, limit int) ([]byte, error) {
	fd, err := openFixedStageMember(parent, name, false)
	if err != nil {
		return nil, ErrUnsafe
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	if !regular(file, 0, 0600, int64(limit)) {
		return nil, ErrUnsafe
	}
	before, err := file.Stat()
	if err != nil {
		return nil, ErrUnsafe
	}
	owner, ok := before.Sys().(*syscall.Stat_t)
	if !ok || owner.Gid != 0 {
		return nil, ErrUnsafe
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	after, statErr := file.Stat()
	if err != nil || statErr != nil || len(data) == 0 || len(data) > limit ||
		!os.SameFile(before, after) || !regular(file, 0, 0600, int64(limit)) {
		return nil, ErrUnsafe
	}
	return data, nil
}
