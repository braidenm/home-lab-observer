// Package connectedcredential validates the private installed credential contract.
package connectedcredential

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
)

var ErrUnsafe = errors.New("connected_credential_unsafe")
var serverPattern = regexp.MustCompile(`^srv_[a-f0-9]{32}$`)
var connectorPattern = regexp.MustCompile(`^agent_[a-f0-9]{32}$`)

var secretPattern = regexp.MustCompile(`^hlc_[A-Za-z0-9_-]{43}$`)

// CredentialRecord is private installation data, never diagnostics or argv.
type CredentialRecord struct {
	Version     string `json:"version"`
	ServerID    string `json:"server_id"`
	ConnectorID string `json:"connector_id"`
	Secret      string `json:"secret"`
}

func DecodeCredential(b []byte, server, connector string) (CredentialRecord, error) {
	var r CredentialRecord
	if len(b) == 0 || len(b) > 512 || json.Unmarshal(b, &r) != nil || r.Version != "observer-connected-credential/v1" || !serverPattern.MatchString(server) || !connectorPattern.MatchString(connector) || r.ServerID != server || r.ConnectorID != connector || !secretPattern.MatchString(r.Secret) {
		return CredentialRecord{}, ErrUnsafe
	}
	canonical, _ := json.Marshal(r)
	defer clear(canonical)
	if !bytes.Equal(canonical, b) {
		return CredentialRecord{}, ErrUnsafe
	}
	return r, nil
}
