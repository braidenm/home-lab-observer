//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"os"

	"github.com/braidenm/home-lab-observer/internal/connectedcredential"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

const dockerStatePath = "/var/lib/hlo-first-install-fixture/docker-receiver.json"

func restoreDockerReceiver() (*dockerReceiver, error) {
	state := dockerCounts{}
	if _, err := os.Lstat(dockerStatePath); !os.IsNotExist(err) {
		body, err := connectedprofile.ReadRootFileMode(dockerStatePath, 1024, 0600)
		if err != nil || json.Unmarshal(body, &state) != nil || !validDockerCounts(state) {
			return nil, errAdmission
		}
		canonical, _ := json.Marshal(state)
		if !bytes.Equal(body, canonical) {
			return nil, errAdmission
		}
		c, err := connectedprofile.LoadRichProcessesDockerConfig()
		if err != nil || c.ServerID != dockerServerID || c.ConnectorID != state.Connector {
			return nil, errAdmission
		}
		secret, err := connectedprofile.ReadRootFileMode(connectedprofile.RichProcessesDockerConfigDirectory+"/credentials/connector.json", 512, 0600)
		defer clear(secret)
		record, decodeErr := connectedcredential.DecodeCredential(secret, dockerServerID, state.Connector)
		if err != nil || decodeErr != nil || record.Secret != dockerCredential {
			return nil, errAdmission
		}
	}
	return &dockerReceiver{state: state, hold: func() bool {
		_, err := os.Lstat("/var/lib/hlo-first-install-fixture/docker-hold")
		return !os.IsNotExist(err)
	}, save: saveDockerCounts}, nil
}

func saveDockerCounts(value dockerCounts) error {
	if !validDockerCounts(value) {
		return errAdmission
	}
	body, err := json.Marshal(value)
	if err != nil || len(body) > 1024 {
		return errAdmission
	}
	f, err := os.OpenFile(dockerStatePath+".next", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errAdmission
	}
	n, writeErr := f.Write(body)
	if writeErr != nil || n != len(body) {
		f.Close()
		return errAdmission
	}
	if f.Sync() != nil || f.Close() != nil || os.Rename(dockerStatePath+".next", dockerStatePath) != nil {
		return errAdmission
	}
	d, err := os.Open("/var/lib/hlo-first-install-fixture")
	if err != nil {
		return errAdmission
	}
	defer d.Close()
	return d.Sync()
}
