//go:build linux

package connectedcredential

import "testing"

func TestRichCredentialRejectsNumericManagerDirectory(t *testing.T) {
	t.Setenv("CREDENTIALS_DIRECTORY", directory)
	if _, err := LoadRichProcesses("srv_0123456789abcdef0123456789abcdef", "agent_0123456789abcdef0123456789abcdef"); err != ErrUnsafe {
		t.Fatal("rich loader admitted numeric manager path")
	}
}

func TestCredentialUnitChoiceHasOnlyTwoFixedLayouts(t *testing.T) {
	if credentialUnit(false) != "home-lab-observer-connected-uploader.service" || credentialUnit(true) != "home-lab-observer-connected-rich-uploader.service" {
		t.Fatal("fixed manager credential unit changed")
	}
}
