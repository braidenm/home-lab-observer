//go:build !linux

package main

// The fixture executable is admitted only in the isolated Linux VM.
func persistFixtureAdmission(string) error   { return errAdmission }
func restoreFixtureAdmission() (bool, error) { return false, errAdmission }
