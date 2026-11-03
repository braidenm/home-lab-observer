package actionworker

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionremote"
)

const settingsFixture = `{"version":"observer-action-worker/v1","origin":"https://observer.example.invalid","server_id":"srv_11111111111111111111111111111111","owner_user_id":"usr_22222222222222222222222222222222","connector_instance_id":"agent_33333333333333333333333333333333"}`

func activeFixture() string {
	return `{"version":"observer-action-credential/v1","server_id":"srv_11111111111111111111111111111111","owner_user_id":"usr_22222222222222222222222222222222","connector_instance_id":"agent_33333333333333333333333333333333","credential_id":"hlac_44444444444444444444444444444444","credential_revision":"1","state":"ACTIVE","expires_at":"2030-01-02T03:04:05.000000Z","authorization_secret":"hlac_44444444444444444444444444444444.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`
}

func TestSettingsAndActiveCredential(t *testing.T) {
	s, err := decodeSettings([]byte(settingsFixture))
	if err != nil || s.Origin != "https://observer.example.invalid" {
		t.Fatal("settings refused")
	}
	now := time.Date(2030, 1, 2, 3, 4, 4, 0, time.UTC)
	c, err := decodeActive([]byte(activeFixture()), s.Binding, now)
	if err != nil {
		t.Fatal("active refused")
	}
	for _, text := range []string{fmt.Sprint(c), fmt.Sprintf("%#v", c)} {
		if strings.Contains(text, "hlac_") || strings.Contains(text, "AAAA") {
			t.Fatal("secret escaped formatting")
		}
	}
	if _, err := decodeActive([]byte(activeFixture()), s.Binding, now.Add(time.Second)); err == nil {
		t.Fatal("expiry admitted")
	}
	other := s.Binding
	other.OwnerUserID = "usr_55555555555555555555555555555555"
	if _, err := decodeActive([]byte(activeFixture()), other, now); err == nil {
		t.Fatal("foreign binding admitted")
	}
}

func TestClosedCredentialDocuments(t *testing.T) {
	s, _ := decodeSettings([]byte(settingsFixture))
	now := time.Date(2029, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, raw := range []string{
		strings.Replace(activeFixture(), `"ACTIVE"`, `"PREPARED"`, 1),
		strings.Replace(activeFixture(), `"credential_revision":"1"`, `"credential_revision":"01"`, 1),
		strings.Replace(activeFixture(), `"credential_revision":"1"`, `"credential_revision":"9223372036854775808"`, 1),
		strings.Replace(activeFixture(), `"credential_revision":"1"`, `"credential_revision":1`, 1),
		strings.Replace(activeFixture(), `"state":"ACTIVE"`, `"State":"ACTIVE"`, 1),
		strings.Replace(activeFixture(), `"state":"ACTIVE"`, `"state":"ACTIVE","state":"ACTIVE"`, 1),
		strings.Replace(activeFixture(), `"state":"ACTIVE"`, `"state":null`, 1),
		strings.Replace(activeFixture(), `"expires_at":"2030-01-02T03:04:05.000000Z"`, `"expires_at":"2030-01-02T03:04:05Z"`, 1),
		strings.Replace(activeFixture(), `"credential_id":"hlac_44444444444444444444444444444444"`, `"credential_id":"hlac_55555555555555555555555555555555"`, 1),
		activeFixture() + `{}`,
		strings.Repeat(" ", 4096) + activeFixture(),
		strings.Replace(activeFixture(), `"version"`, `"extra":"x","version"`, 1),
	} {
		if _, err := decodeActive([]byte(raw), s.Binding, now); err == nil {
			t.Fatal("invalid active admitted")
		}
	}
	for _, raw := range []string{
		strings.Replace(settingsFixture, `https://observer.example.invalid`, `http://observer.example.invalid`, 1),
		strings.Replace(settingsFixture, `https://observer.example.invalid`, `https://observer.example.invalid/`, 1),
		strings.Replace(settingsFixture, `https://observer.example.invalid`, `https://user:password@observer.example.invalid`, 1),
		strings.Replace(settingsFixture, `"origin"`, `"Origin"`, 1),
		strings.Replace(settingsFixture, `"server_id"`, `"server_id":null,"duplicate"`, 1),
		settingsFixture + "\xff",
	} {
		if _, err := decodeSettings([]byte(raw)); err == nil {
			t.Fatal("invalid settings admitted")
		}
	}
	if _, err := decodeActive([]byte(activeFixture()), actionremote.Binding{}, now); err == nil {
		t.Fatal("zero binding admitted")
	}
}
