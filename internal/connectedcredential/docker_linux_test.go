//go:build linux

package connectedcredential

import "testing"

func TestDockerCredentialCannotUseExistingManagerMount(t *testing.T) {
	for _, unit := range []string{directory, "/run/credentials/" + credentialUnit(true), "/run/credentials/foreign.service"} {
		t.Setenv("CREDENTIALS_DIRECTORY", unit)
		if _, err := LoadRichProcessesDocker("srv_0123456789abcdef0123456789abcdef", "agent_0123456789abcdef0123456789abcdef"); err != ErrUnsafe {
			t.Fatal("Docker loader accepted a foreign manager mount")
		}
	}
}
