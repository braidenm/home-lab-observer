//go:build !windows

package collector

import (
	"context"

	"github.com/shirou/gopsutil/v4/process"
)

func readProcessStatus(ctx context.Context, process *process.Process) (string, error) {
	statuses, err := process.StatusWithContext(ctx)
	if err != nil {
		return "", err
	}
	return normalizedProcessStatus(statuses), nil
}
