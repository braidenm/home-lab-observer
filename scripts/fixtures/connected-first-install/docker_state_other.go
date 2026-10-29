//go:build !linux

package main

func restoreDockerReceiver() (*dockerReceiver, error) { return nil, errAdmission }
