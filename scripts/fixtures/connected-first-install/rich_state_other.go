//go:build !linux

package main

func restoreRichReceiver() (*richReceiver, error) { return nil, errAdmission }
