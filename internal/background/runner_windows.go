//go:build windows

package background

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func trustedManagerExecutable(name string) (string, error) {
	base := strings.ToLower(filepath.Base(name))
	if base != "schtasks.exe" && base != "powershell.exe" {
		return "", errors.New("manager executable is not allowlisted")
	}
	directory, err := windows.GetSystemDirectory()
	if err != nil {
		return "", err
	}
	path := filepath.Join(directory, base)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("trusted Windows manager executable is unavailable")
	}
	return path, nil
}

func prepareManagerCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Env = os.Environ()
}
