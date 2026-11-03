// Package actionworker owns outbound worker composition, never Docker or the
// broker journal. Its active credential reader is separate from observation.
package actionworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/braidenm/home-lab-observer/internal/actionremote"
)

var ErrUnsafe = errors.New("action_worker_profile_unavailable")

const documentLimit = 4096

type Settings struct {
	Origin  string
	Binding actionremote.Binding
}

type unavailableCredential struct{}

func (unavailableCredential) Credential(context.Context, actionremote.Binding) (actionremote.Credential, error) {
	return actionremote.Credential{}, actionremote.ErrCredential
}

// orderedStrings accepts only the closed, flat, string-valued installed record.
// No generic document reader or rejected value is exposed outside this package.
func orderedStrings(raw []byte, fields ...string) ([]string, error) {
	if len(raw) == 0 || len(raw) > documentLimit || !utf8.Valid(raw) {
		return nil, ErrUnsafe
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return nil, ErrUnsafe
	}
	values := make([]string, len(fields))
	for i, field := range fields {
		key, err := d.Token()
		if err != nil || key != field {
			return nil, ErrUnsafe
		}
		value, err := d.Token()
		text, ok := value.(string)
		if err != nil || !ok {
			return nil, ErrUnsafe
		}
		values[i] = text
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') {
		return nil, ErrUnsafe
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, ErrUnsafe
	}
	return values, nil
}

func decodeSettings(raw []byte) (Settings, error) {
	v, err := orderedStrings(raw, "version", "origin", "server_id", "owner_user_id", "connector_instance_id")
	if err != nil || v[0] != "observer-action-worker/v1" {
		return Settings{}, ErrUnsafe
	}
	s := Settings{Origin: v[1], Binding: actionremote.Binding{ServerID: v[2], OwnerUserID: v[3], ConnectorInstanceID: v[4]}}
	// Reuse the transport's canonical origin/binding admission without doing I/O
	// or adding a second URL grammar. This client never receives a credential.
	c, err := actionremote.New(s.Origin, s.Binding, unavailableCredential{})
	if err != nil {
		return Settings{}, ErrUnsafe
	}
	if c.Close() != nil {
		return Settings{}, ErrUnsafe
	}
	return s, nil
}

func decodeActive(raw []byte, binding actionremote.Binding, now time.Time) (actionremote.Credential, error) {
	v, err := orderedStrings(raw, "version", "server_id", "owner_user_id", "connector_instance_id", "credential_id", "credential_revision", "state", "expires_at", "authorization_secret")
	if err != nil || v[0] != "observer-action-credential/v1" || v[1] != binding.ServerID || v[2] != binding.OwnerUserID || v[3] != binding.ConnectorInstanceID || v[6] != "ACTIVE" {
		return actionremote.Credential{}, ErrUnsafe
	}
	revision, err := strconv.ParseInt(v[5], 10, 64)
	if err != nil || revision <= 0 || strconv.FormatInt(revision, 10) != v[5] {
		return actionremote.Credential{}, ErrUnsafe
	}
	expires, err := time.Parse("2006-01-02T15:04:05.000000Z", v[7])
	if err != nil || expires.Format("2006-01-02T15:04:05.000000Z") != v[7] || now.IsZero() || !now.Before(expires) {
		return actionremote.Credential{}, ErrUnsafe
	}
	// Constructor validates the complete secret, including its public ID and
	// canonical base64. Exact public-ID equality prevents metadata substitution.
	c, err := actionremote.NewCredential(v[8])
	if err != nil || !strings.HasPrefix(v[8], v[4]+".") || len(v[4]) != 37 {
		return actionremote.Credential{}, ErrUnsafe
	}
	if !pairedID(binding.ServerID, "srv_") || !pairedID(binding.OwnerUserID, "usr_") || !pairedID(binding.ConnectorInstanceID, "agent_") {
		return actionremote.Credential{}, ErrUnsafe
	}
	return c, nil
}

func pairedID(value, prefix string) bool {
	if len(value) != len(prefix)+32 || !strings.HasPrefix(value, prefix) {
		return false
	}
	for _, c := range value[len(prefix):] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
