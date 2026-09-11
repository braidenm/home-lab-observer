//go:build !linux

package numerichost

import (
	"context"
	"errors"
	"time"
)

// GopsutilProvider is unsupported until native adapters pass the isolated
// dependency and installation evidence gates. No other OS collector is imported.
type GopsutilProvider struct{}

func (GopsutilProvider) LogicalCPUCount(context.Context) (int, error) {
	return 0, errors.ErrUnsupported
}
func (GopsutilProvider) CPUPercent(context.Context, time.Duration) (float64, error) {
	return 0, errors.ErrUnsupported
}
func (GopsutilProvider) Memory(context.Context) (MemoryStat, error) {
	return MemoryStat{}, errors.ErrUnsupported
}
func (GopsutilProvider) Swap(context.Context) (SwapStat, error) {
	return SwapStat{}, errors.ErrUnsupported
}
func (GopsutilProvider) Partitions(context.Context) ([]Partition, error) {
	return nil, errors.ErrUnsupported
}
func (GopsutilProvider) Usage(context.Context, string) (UsageStat, error) {
	return UsageStat{}, errors.ErrUnsupported
}
func (GopsutilProvider) Uptime(context.Context) (uint64, error) { return 0, errors.ErrUnsupported }
