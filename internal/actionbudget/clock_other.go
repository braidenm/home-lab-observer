//go:build !linux

package actionbudget

func nativeReading() (reading, error) { return reading{}, ErrUnavailable }
