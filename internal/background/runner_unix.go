//go:build !windows

package background

import (
	"errors"
	"os"
	"os/exec"
)

func trustedManagerExecutable(name string) (string, error) {
	allowed := map[string][]string{
		"systemctl": {"/usr/bin/systemctl", "/bin/systemctl"},
		"launchctl": {"/bin/launchctl"},
	}
	for _, candidate := range allowed[name] {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	return "", errors.New("trusted user-manager executable is unavailable")
}

func prepareManagerCommand(cmd *exec.Cmd) {
	cmd.Env = []string{"LC_ALL=C", "LANG=C"}
	for _, key := range []string{"HOME", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS"} {
		if value, found := os.LookupEnv(key); found {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
}
