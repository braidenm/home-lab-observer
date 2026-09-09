package collector

import (
	"context"

	"github.com/shirou/gopsutil/v4/process"
)

// gopsutil v4.26.6 explicitly does not implement process status on Windows.
// Preserve the remaining real process fields and report the missing field
// honestly instead of discarding every process.
func readProcessStatus(context.Context, *process.Process) (string, error) {
	return "unknown", nil
}
