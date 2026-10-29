//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"os"

	"github.com/braidenm/home-lab-observer/internal/connectedcredential"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

const richStatePath = "/var/lib/hlo-first-install-fixture/rich-receiver.json"

func restoreRichReceiver() (*richReceiver, error) {
	state := richCounts{}
	if _, err := os.Lstat(richStatePath); !os.IsNotExist(err) {
		body, err := connectedprofile.ReadRootFileMode(richStatePath, 1024, 0600)
		if err != nil || json.Unmarshal(body, &state) != nil {
			return nil, errAdmission
		}
		canonical, _ := json.Marshal(state)
		if !bytes.Equal(body, canonical) {
			return nil, errAdmission
		}
		config, err := connectedprofile.LoadRichProcessesConfig()
		if err != nil || config.ServerID != richServerID || config.ConnectorID != state.Connector {
			return nil, errAdmission
		}
		secret, err := connectedprofile.ReadRootFileMode(connectedprofile.RichProcessesConfigDirectory+"/credentials/connector.json", 512, 0600)
		defer clear(secret)
		record, decodeErr := connectedcredential.DecodeCredential(secret, richServerID, state.Connector)
		if err != nil || decodeErr != nil || record.Secret != richCredential {
			return nil, errAdmission
		}
	}
	return &richReceiver{state: state, hold: func() bool {
		_, err := os.Lstat("/var/lib/hlo-first-install-fixture/rich-hold")
		return !os.IsNotExist(err)
	}, save: func(value richCounts) error {
		body, err := json.Marshal(value)
		if err != nil {
			return errAdmission
		}
		f, err := os.OpenFile(richStatePath+".next", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return errAdmission
		}
		if _, err = f.Write(body); err != nil {
			f.Close()
			return errAdmission
		}
		if f.Sync() != nil || f.Close() != nil || os.Rename(richStatePath+".next", richStatePath) != nil {
			return errAdmission
		}
		d, err := os.Open("/var/lib/hlo-first-install-fixture")
		if err != nil {
			return errAdmission
		}
		defer d.Close()
		return d.Sync()
	}}, nil
}
