package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/braidenm/home-lab-observer/internal/connectedcredential"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

var errAdmission = errors.New("fixture_admission_refused")

// This is synthetic VM receiver state, not a production authorization store.
// Reboot must retain enrollment without repeating the one-use exchange.
type fixtureAdmission struct {
	Version          string `json:"version"`
	Server           string `json:"server_id"`
	Connector        string `json:"connector_id"`
	CredentialDigest string `json:"credential_sha256"`
}

func admissionBytes(connector string) ([]byte, error) {
	if !connectedprofile.ValidConnectorID(connector) {
		return nil, errAdmission
	}
	digest := sha256.Sum256([]byte(credential))
	return json.Marshal(fixtureAdmission{"fixture-admission/v1", serverID, connector, hex.EncodeToString(digest[:])})
}

func validateAdmission(body []byte, config connectedprofile.Config, secret []byte) error {
	if len(body) == 0 || len(body) > 512 || config.ServerID != serverID {
		return errAdmission
	}
	expected, err := admissionBytes(config.ConnectorID)
	if err != nil || !bytes.Equal(body, expected) {
		return errAdmission
	}
	record, err := connectedcredential.DecodeCredential(secret, config.ServerID, config.ConnectorID)
	if err != nil || record.Secret != credential {
		return errAdmission
	}
	return nil
}
