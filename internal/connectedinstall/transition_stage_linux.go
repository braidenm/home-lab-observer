//go:build linux

package connectedinstall

import (
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
)

const transitionStageName = "transition-stage"
const transitionPreparationName = "transition-preparing.json"

type transitionStageSnapshot struct {
	preparation []byte
	files       map[string][]byte
	record      connectedtransition.Record
}

// inspectTransitionStage reads only fixed private evidence. It does not
// authorize cleanup, resume, replacement or worker startup. The caller must
// hold the installation lease and separately prove the stopped-worker state.
func inspectTransitionStage() (transitionStageSnapshot, error) {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		return transitionStageSnapshot{}, ErrUnsafe
	}
	config, err := rootDirectory(connectedprofile.ConfigDirectory)
	if err != nil {
		return transitionStageSnapshot{}, ErrUnsafe
	}
	defer config.Close()
	fd, err := openFixedStageMember(int(config.Fd()), transitionStageName, true)
	if err != nil {
		return transitionStageSnapshot{}, ErrRecovery
	}
	stage := os.NewFile(uintptr(fd), transitionStageName)
	defer stage.Close()
	return inspectTransitionStageAt(config, stage)
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

func inspectTransitionStageAt(config, stage *os.File) (transitionStageSnapshot, error) {
	if !fixedRootDirectory(config, 0755) || !fixedRootDirectory(stage, 0700) {
		return transitionStageSnapshot{}, ErrUnsafe
	}
	var configStat, stageStat unix.Stat_t
	var configFS, stageFS unix.Statfs_t
	if unix.Fstat(int(config.Fd()), &configStat) != nil || unix.Fstat(int(stage.Fd()), &stageStat) != nil ||
		configStat.Dev != stageStat.Dev ||
		unix.Fstatfs(int(config.Fd()), &configFS) != nil || unix.Fstatfs(int(stage.Fd()), &stageFS) != nil ||
		configFS.Type != unix.EXT4_SUPER_MAGIC || stageFS.Type != unix.EXT4_SUPER_MAGIC {
		return transitionStageSnapshot{}, ErrUnsafe
	}
	entries, err := stage.ReadDir(6)
	if err != nil && err != io.EOF || len(entries) < 3 || len(entries) > 5 {
		return transitionStageSnapshot{}, ErrUnsafe
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
			return transitionStageSnapshot{}, ErrUnsafe
		}
		data, err := readFixedStageFile(int(stage.Fd()), name, limit)
		if err != nil {
			return transitionStageSnapshot{}, ErrUnsafe
		}
		files[name] = data
	}
	preparation, err := readFixedStageFile(int(config.Fd()), transitionPreparationName, connectedtransition.MaxPreparationBytes)
	if err != nil {
		return transitionStageSnapshot{}, ErrUnsafe
	}
	record, err := connectedtransition.ValidateStage(preparation, files)
	if err != nil {
		return transitionStageSnapshot{}, ErrRecovery
	}
	return transitionStageSnapshot{preparation: preparation, files: files, record: record}, nil
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
