package background

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func xmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return replacer.Replace(value)
}

func exactExternalFile(path string, expected []byte) (matched, exists bool, err error) {
	file, err := openRegular(path, int64(len(expected)+1))
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		if _, statErr := os.Lstat(path); errors.Is(statErr, os.ErrNotExist) {
			return false, false, nil
		}
		return false, true, err
	}
	defer file.Close()
	contents := make([]byte, len(expected)+1)
	count, readErr := file.Read(contents)
	if readErr != nil && count == 0 {
		return false, true, readErr
	}
	return count == len(expected) && string(contents[:count]) == string(expected), true, nil
}

func installExternalFile(target, source string, expected []byte) error {
	matched, exists, err := exactExternalFile(target, expected)
	if err != nil || (exists && !matched) {
		return errors.New("refusing to overwrite an unknown manager registration")
	}
	if exists {
		return nil
	}
	parent := filepath.Dir(target)
	if _, err := secureAbsolutePath(parent, false); err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	sourceFile, err := openRegular(source, int64(len(expected)+1))
	if err != nil {
		return err
	}
	sourceFile.Close()
	return writePrivateAtomic(target, expected)
}
