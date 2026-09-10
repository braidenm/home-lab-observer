//go:build !windows

package logprocess

import "os/exec"

func configureChild(_ *exec.Cmd) {}
