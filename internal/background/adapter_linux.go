//go:build linux

package background

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const managerIdentity = "home-lab-observer.service"

type linuxAdapter struct {
	runner commandRunner
	target string
	root   string
}

func launcherName() string { return "observer" }

func newPlatformAdapter(runner commandRunner, root string) (platformAdapter, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return nil, coded(CodeManagerUnavailable, err)
	}
	return &linuxAdapter{runner: runner, target: filepath.Join(config, "systemd", "user", managerIdentity), root: root}, nil
}

func (a *linuxAdapter) registration(Settings) (registration, error) {
	launcher := filepath.Join(a.root, "bin", launcherName())
	content := fmt.Sprintf(`[Unit]
Description=Home Lab Observer (user session)

[Service]
Type=simple
ExecStart=:%s background run --install-root %s
Restart=on-failure
RestartSec=5s
TimeoutStopSec=35s
SendSIGKILL=no
StandardOutput=null
StandardError=null
NoNewPrivileges=true

[Install]
WantedBy=default.target
`, systemdQuote(launcher), systemdQuote(a.root))
	return registration{fileName: managerIdentity, content: []byte(content)}, nil
}

func systemdQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "%", "%%")
	return `"` + value + `"`
}

func (a *linuxAdapter) available(ctx context.Context) bool {
	_, err := a.runner.run(ctx, command{name: "systemctl", args: []string{"--user", "show-environment"}})
	return err == nil
}

func (a *linuxAdapter) inspect(ctx context.Context, expected registration) (managerState, error) {
	if !a.available(ctx) {
		return managerState{}, nil
	}
	state := managerState{available: true}
	matched, exists, err := exactExternalFile(a.target, expected.content)
	if err != nil {
		return state, coded(CodeRegistrationMismatch, err)
	}
	if !exists {
		result, queryErr := a.runner.run(ctx, command{name: "systemctl", args: []string{"--user", "show", "--property=LoadState", "--value", managerIdentity}, capture: true})
		if queryErr != nil {
			return state, queryErr
		}
		if loaded := strings.TrimSpace(result.output); loaded != "" && loaded != "not-found" {
			return state, coded(CodeRegistrationMismatch, errors.New("systemd has a loaded job without the managed unit file"))
		}
		return state, nil
	}
	if !matched {
		return state, coded(CodeRegistrationMismatch, errors.New("existing systemd user unit is not managed by this observer"))
	}
	state.registered = true
	properties, err := a.runner.run(ctx, command{name: "systemctl", args: []string{"--user", "show", "--property=FragmentPath", "--property=DropInPaths", "--property=NeedDaemonReload", managerIdentity}, capture: true})
	if err != nil {
		return state, err
	}
	wantProperties := map[string]string{"FragmentPath": a.target, "DropInPaths": "", "NeedDaemonReload": "no"}
	seen := make(map[string]string, 3)
	for _, line := range strings.Split(strings.TrimSpace(properties.output), "\n") {
		key, value, found := strings.Cut(strings.TrimSuffix(line, "\r"), "=")
		if !found || len(seen) >= 3 {
			return state, coded(CodeRegistrationMismatch, errors.New("systemd unit properties are malformed"))
		}
		seen[key] = value
	}
	if len(seen) != len(wantProperties) {
		return state, coded(CodeRegistrationMismatch, errors.New("systemd unit properties are incomplete"))
	}
	for key, expectedValue := range wantProperties {
		if seen[key] != expectedValue {
			return state, coded(CodeRegistrationMismatch, errors.New("systemd unit has overrides or needs reload"))
		}
	}
	result, err := a.runner.run(ctx, command{name: "systemctl", args: []string{"--user", "is-active", "--quiet", managerIdentity}})
	if err == nil {
		state.running = true
		return state, nil
	}
	if result.exitCode == 3 || result.exitCode == 4 {
		return state, nil
	}
	return state, err
}

func (a *linuxAdapter) register(ctx context.Context, source string, expected registration) error {
	if err := installExternalFile(a.target, source, expected.content); err != nil {
		return err
	}
	if _, err := a.runner.run(ctx, command{name: "systemctl", args: []string{"--user", "daemon-reload"}}); err != nil {
		return err
	}
	_, err := a.runner.run(ctx, command{name: "systemctl", args: []string{"--user", "enable", managerIdentity}})
	return err
}

func (a *linuxAdapter) start(ctx context.Context) error {
	_, err := a.runner.run(ctx, command{name: "systemctl", args: []string{"--user", "start", managerIdentity}})
	return err
}

func (a *linuxAdapter) stop(ctx context.Context, force bool, stopper GracefulStopper, settings Settings) error {
	if !force {
		if stopper == nil {
			return errors.New("graceful lifecycle client is unavailable")
		}
		return stopper.RequestStop(ctx, settings.StateDir)
	}
	args := []string{"--user", "stop", managerIdentity}
	if _, err := a.runner.run(ctx, command{name: "systemctl", args: []string{"--user", "stop", "--no-block", managerIdentity}}); err != nil {
		return err
	}
	args = []string{"--user", "kill", "--kill-whom=all", "--signal=KILL", managerIdentity}
	_, err := a.runner.run(ctx, command{name: "systemctl", args: args})
	return err
}

func (a *linuxAdapter) unregister(ctx context.Context, _ string, expected registration) error {
	matched, exists, err := exactExternalFile(a.target, expected.content)
	if err != nil || (exists && !matched) {
		return errors.New("refusing to remove an unknown systemd user unit")
	}
	if exists {
		if _, err := a.runner.run(ctx, command{name: "systemctl", args: []string{"--user", "disable", managerIdentity}}); err != nil {
			return err
		}
		if err := os.Remove(a.target); err != nil {
			return err
		}
		_, err = a.runner.run(ctx, command{name: "systemctl", args: []string{"--user", "daemon-reload"}})
	}
	return err
}
