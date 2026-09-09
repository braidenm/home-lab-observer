package lifecycle

import (
	"errors"
	"os"

	"github.com/braidenm/home-lab-observer/internal/ownerfs"
)

type fileLock struct{ file *os.File }

func acquireFileLock(path string) (*fileLock, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	created := err == nil
	if errors.Is(err, os.ErrExist) {
		pathInfo, validateErr := ownerfs.ValidateRegular(path, maxControlBytes)
		if validateErr != nil {
			return nil, classifyPath(validateErr)
		}
		file, err = os.OpenFile(path, os.O_RDWR, 0o600)
		if err == nil {
			opened, statErr := file.Stat()
			if statErr != nil || !os.SameFile(opened, pathInfo) {
				_ = file.Close()
				return nil, ErrUnsafePath
			}
		}
	}
	if err != nil {
		return nil, classifyPath(err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = file.Close()
		}
	}()
	if created {
		if err := ownerfs.RestrictFile(path); err != nil {
			return nil, classifyPath(err)
		}
	}
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	pathInfo, err := ownerfs.ValidateRegular(path, maxControlBytes)
	if err != nil || !os.SameFile(opened, pathInfo) {
		return nil, ErrUnsafePath
	}
	if !created {
		// The path was validated before opening; restricting it now cannot affect
		// an unrelated hard-linked file.
		if err := ownerfs.RestrictFile(path); err != nil {
			return nil, classifyPath(err)
		}
	}
	if err := lockFile(file); err != nil {
		return nil, err
	}
	ok = true
	return &fileLock{file: file}, nil
}

func (l *fileLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err1 := unlockFile(l.file)
	err2 := l.file.Close()
	l.file = nil
	return errors.Join(err1, err2)
}
