//go:build linux

package main

import (
	"errors"
	"os"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

const admissionPath = "/var/lib/hlo-first-install-fixture/activation-admission.json"

func persistFixtureAdmission(connector string) error {
	body, err := admissionBytes(connector)
	if err != nil {
		return errAdmission
	}
	file, err := os.OpenFile(admissionPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errAdmission
	}
	defer file.Close()
	if _, err := file.Write(body); err != nil || file.Sync() != nil {
		return errAdmission
	}
	dir, err := os.Open("/var/lib/hlo-first-install-fixture")
	if err != nil {
		return errAdmission
	}
	defer dir.Close()
	if dir.Sync() != nil {
		return errAdmission
	}
	return nil
}

func restoreFixtureAdmission() (bool, error) {
	if _, err := os.Lstat(admissionPath); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	body, err := connectedprofile.ReadRootFileMode(admissionPath, 512, 0600)
	if err != nil {
		return false, errAdmission
	}
	config, err := connectedprofile.Load()
	if err != nil {
		return false, errAdmission
	}
	secret, err := connectedprofile.ReadRootFileMode("/etc/home-lab-observer-connected/credentials/connector.json", 512, 0600)
	defer clear(secret)
	if err != nil || validateAdmission(body, config, secret) != nil {
		return false, errAdmission
	}
	return true, nil
}
