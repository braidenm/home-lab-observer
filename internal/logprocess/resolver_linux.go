package logprocess

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func resolvePlatformHelper(executable, expected string) (commandSpec, error) {
	if len(expected) != 64 {
		return commandSpec{}, ErrHelperMismatch
	}
	for _, c := range expected {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return commandSpec{}, ErrHelperMismatch
		}
	}
	if os.Getuid() != os.Geteuid() || os.Getgid() != os.Getegid() {
		return commandSpec{}, ErrHelperUnavailable
	}
	if !filepath.IsAbs(executable) || filepath.Clean(executable) != executable || len(executable) > maxIdentityPathBytes || strings.ContainsRune(executable, 0) {
		return commandSpec{}, ErrHelperUnavailable
	}
	if err := verifyLinuxAncestors(filepath.Dir(executable)); err != nil {
		return commandSpec{}, ErrHelperUnavailable
	}
	main, err := openLinuxExecutable(executable)
	if err != nil {
		return commandSpec{}, ErrHelperUnavailable
	}
	main.Close()
	helper := filepath.Join(filepath.Dir(executable), "observer-journal-helper")
	if helper == executable {
		return commandSpec{}, ErrHelperMismatch
	}
	file, err := openLinuxExecutable(helper)
	if err != nil {
		return commandSpec{}, ErrHelperUnavailable
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return commandSpec{}, ErrHelperUnavailable
	}
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(file, maxHelperBytes+1))
	if err != nil || size != before.Size() || size > maxHelperBytes {
		return commandSpec{}, ErrHelperUnavailable
	}
	after, err := file.Stat()
	if err != nil || after.Size() != before.Size() || !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) {
		return commandSpec{}, ErrHelperUnavailable
	}
	pathInfo, err := os.Lstat(helper)
	if err != nil || !os.SameFile(after, pathInfo) || pathInfo.Mode()&os.ModeSymlink != 0 {
		return commandSpec{}, ErrHelperUnavailable
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return commandSpec{}, ErrHelperMismatch
	}
	return commandSpec{path: helper, env: helperEnvironment()}, nil
}

func verifyLinuxAncestors(path string) error {
	for depth := 0; depth < maxIdentityDepth; depth++ {
		var stat unix.Stat_t
		if unix.Lstat(path, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR {
			return ErrHelperUnavailable
		}
		if !safeLinuxDirectory(stat) {
			return ErrHelperUnavailable
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
	return ErrHelperUnavailable
}

func trustedLinuxOwner(uid uint32) bool { return uid == 0 || uid == uint32(os.Geteuid()) }

func safeLinuxDirectory(stat unix.Stat_t) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFDIR && trustedLinuxOwner(stat.Uid) &&
		(stat.Mode&0022 == 0 || stat.Uid == 0 && stat.Mode&unix.S_ISVTX != 0)
}

func safeLinuxExecutable(stat unix.Stat_t) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFREG && stat.Nlink == 1 && trustedLinuxOwner(stat.Uid) && stat.Mode&0022 == 0 &&
		stat.Mode&(unix.S_ISUID|unix.S_ISGID) == 0 && stat.Mode&0111 != 0 && stat.Size > 0 && stat.Size <= maxHelperBytes
}

func openLinuxExecutable(path string) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() <= 0 || before.Size() > maxHelperBytes {
		return nil, ErrHelperUnavailable
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrHelperUnavailable
	}
	file := os.NewFile(uintptr(fd), path)
	good := false
	defer func() {
		if !good {
			file.Close()
		}
	}()
	after, err := file.Stat()
	var stat unix.Stat_t
	if err != nil || !os.SameFile(before, after) || unix.Fstat(fd, &stat) != nil || !safeLinuxExecutable(stat) {
		return nil, ErrHelperUnavailable
	}
	size, err := unix.Fgetxattr(fd, "security.capability", nil)
	if size > 0 || err != nil && !errors.Is(err, unix.ENODATA) && !errors.Is(err, unix.EOPNOTSUPP) {
		return nil, ErrHelperUnavailable
	}
	good = true
	return file, nil
}
