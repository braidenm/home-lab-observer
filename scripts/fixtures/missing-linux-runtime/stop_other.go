//go:build !linux

package main

import "os/exec"

func stop(command *exec.Cmd, done <-chan error) error { return errProof }

func killAndReap(command *exec.Cmd, done <-chan error) error { return errProof }
