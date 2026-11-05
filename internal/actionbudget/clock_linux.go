//go:build linux

package actionbudget

import (
	"time"

	"golang.org/x/sys/unix"
)

func nativeReading() (reading, error) {
	goTime := time.Now()
	var boot unix.Timespec
	if unix.ClockGettime(unix.CLOCK_BOOTTIME, &boot) != nil || boot.Sec < 0 || boot.Nsec < 0 || boot.Nsec >= int64(time.Second) || boot.Sec > int64((1<<63-1)/time.Second) {
		return reading{}, ErrUnavailable
	}
	return reading{goTime: goTime, wall: goTime.UTC(), boot: time.Duration(boot.Sec)*time.Second + time.Duration(boot.Nsec)}, nil
}
