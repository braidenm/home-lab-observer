// Package ownerfs provides the small private-filesystem boundary shared by
// local control and diagnostic state. It intentionally accepts only a state
// directory and a fixed, code-owned child name.
package ownerfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrUnsafePath = errors.New("unsafe owner-only path")

// ValidateDedicatedDirectory verifies an existing dedicated directory and all
// of its ancestors without changing permissions or creating entries.
func ValidateDedicatedDirectory(path string) error {
	resolved, err := resolveStateDir(path, "validation-only")
	if err != nil {
		return err
	}
	return validateDirectoryPath(resolved)
}

// EnsurePrivateSubdir creates and restricts a fixed child of a dedicated state
// directory. Callers must supply a code-owned child name, not user input.
func EnsurePrivateSubdir(stateDir, child string) (string, error) {
	stateDir, err := resolveStateDir(stateDir, child)
	if err != nil {
		return "", err
	}
	existed, err := validateExistingDirectoryPrefix(stateDir)
	if err != nil {
		return "", err
	}
	if !existed {
		if err := os.MkdirAll(stateDir, 0o700); err != nil {
			return "", err
		}
	}
	if err := validateDirectoryPath(stateDir); err != nil {
		return "", err
	}
	if !existed {
		if err := RestrictDirectory(stateDir); err != nil {
			return "", err
		}
	}

	directory := filepath.Join(stateDir, child)
	if err := os.Mkdir(directory, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	if err := validateDirectoryPath(directory); err != nil {
		return "", fmt.Errorf("%w: private directory", ErrUnsafePath)
	}
	if err := RestrictDirectory(directory); err != nil {
		return "", err
	}
	return directory, nil
}

// OpenPrivateSubdir validates an existing fixed child without creating or
// changing anything. It is used by stop/status paths that must be read-only
// when no managed runtime exists.
func OpenPrivateSubdir(stateDir, child string) (string, error) {
	stateDir, err := resolveStateDir(stateDir, child)
	if err != nil {
		return "", err
	}
	if err := validateDirectoryPath(stateDir); err != nil {
		return "", err
	}
	directory := filepath.Join(stateDir, child)
	if err := validateDirectoryPath(directory); err != nil {
		return "", err
	}
	return directory, nil
}

func resolveStateDir(stateDir, child string) (string, error) {
	if strings.TrimSpace(stateDir) == "" || child == "" || filepath.Base(child) != child || child == "." || child == ".." {
		return "", fmt.Errorf("%w: invalid directory", ErrUnsafePath)
	}
	stateDir, err := filepath.Abs(stateDir)
	if err != nil {
		return "", fmt.Errorf("%w: resolve state directory", ErrUnsafePath)
	}
	stateDir = filepath.Clean(stateDir)
	userHome, _ := os.UserHomeDir()
	if filepath.Dir(stateDir) == stateDir || (userHome != "" && strings.EqualFold(stateDir, filepath.Clean(userHome))) {
		return "", fmt.Errorf("%w: use a dedicated state directory", ErrUnsafePath)
	}
	return stateDir, nil
}

// validateExistingDirectoryPrefix validates components in order and stops at
// the first missing component. Callers may create the missing suffix only
// after every reachable ancestor has passed the link/reparse checks.
func validateExistingDirectoryPrefix(path string) (bool, error) {
	volume := filepath.VolumeName(path)
	current := volume + string(os.PathSeparator)
	relative, err := filepath.Rel(current, path)
	if err != nil || relative == "." || relative == "" {
		return false, ErrUnsafePath
	}
	for _, part := range splitPath(relative) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false, ErrUnsafePath
		}
		if err := validateNotReparse(current); err != nil {
			return false, err
		}
	}
	return true, nil
}

// validateDirectoryPath walks every existing component before callers create
// anything below it. In particular, this prevents a missing suffix reached
// through a symlink or Windows reparse point from being created elsewhere.
func validateDirectoryPath(path string) error {
	volume := filepath.VolumeName(path)
	current := volume + string(os.PathSeparator)
	relative, err := filepath.Rel(current, path)
	if err != nil || relative == "." || relative == "" {
		return ErrUnsafePath
	}
	for _, part := range splitPath(relative) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ErrUnsafePath
		}
		if err := validateNotReparse(current); err != nil {
			return err
		}
	}
	return nil
}

func splitPath(path string) []string {
	parts := make([]string, 0, 8)
	for path != "." && path != "" {
		dir, base := filepath.Split(path)
		if base != "" {
			parts = append([]string{base}, parts...)
		}
		path = filepath.Clean(dir)
		if path == "." || path == string(os.PathSeparator) {
			break
		}
	}
	return parts
}

// ValidateRegular rejects links, devices, directories, and oversized files.
func ValidateRegular(path string, maxBytes int64) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() < 0 || info.Size() > maxBytes {
		return nil, fmt.Errorf("%w: expected bounded regular file", ErrUnsafePath)
	}
	if err := validateNotReparse(path); err != nil {
		return nil, fmt.Errorf("%w: file is a link or reparse point", ErrUnsafePath)
	}
	if err := validateSingleLink(path, info); err != nil {
		return nil, fmt.Errorf("%w: file has multiple links", ErrUnsafePath)
	}
	return info, nil
}
