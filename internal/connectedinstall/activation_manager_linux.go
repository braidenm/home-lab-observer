//go:build linux

package connectedinstall

import (
	"context"
	"strconv"
	"strings"
)

// workerInstance is manager evidence for one invocation, never a durable permit.
type workerInstance struct {
	PID        int
	Invocation string
	Active     string
}

var activationProperties = []string{"ActiveState", "MainPID", "InvocationID", "Restart", "StandardInput", "StandardOutput", "StandardError", "FileDescriptorStoreMax", "NFileDescriptorStore", "TriggeredBy", "UnitFileState"}

func inspectWorker(ctx context.Context, unit string) (workerInstance, error) {
	if verifyManagedUnit(ctx, unit) != nil {
		return workerInstance{}, ErrUnsafe
	}
	args := []string{"show"}
	for _, property := range activationProperties {
		args = append(args, "--property="+property)
	}
	args = append(args, "--", unit)
	b, err := command(ctx, "/usr/bin/systemctl", args, nil, 2048)
	if err != nil {
		return workerInstance{}, ErrUnsafe
	}
	return parseWorker(b)
}

func parseWorker(b []byte) (workerInstance, error) {
	if len(b) == 0 || len(b) > 2048 || strings.ContainsAny(string(b), "\r\x00") {
		return workerInstance{}, ErrUnsafe
	}
	values := make(map[string]string, len(activationProperties))
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return workerInstance{}, ErrUnsafe
		}
		known := false
		for _, allowed := range activationProperties {
			if key == allowed {
				known = true
				break
			}
		}
		if _, duplicate := values[key]; !known || duplicate {
			return workerInstance{}, ErrUnsafe
		}
		values[key] = value
	}
	if len(values) != len(activationProperties) {
		return workerInstance{}, ErrUnsafe
	}
	for key, want := range map[string]string{"Restart": "no", "StandardInput": "null", "StandardOutput": "null", "StandardError": "null", "FileDescriptorStoreMax": "0", "NFileDescriptorStore": "0", "TriggeredBy": "", "UnitFileState": "disabled"} {
		if values[key] != want {
			return workerInstance{}, ErrUnsafe
		}
	}
	pid, err := strconv.Atoi(values["MainPID"])
	if err != nil || pid < 0 || strconv.Itoa(pid) != values["MainPID"] {
		return workerInstance{}, ErrUnsafe
	}
	instance := workerInstance{PID: pid, Invocation: values["InvocationID"], Active: values["ActiveState"]}
	switch instance.Active {
	case "active", "activating", "deactivating":
		if pid < 2 || !invocationPattern.MatchString(instance.Invocation) {
			return workerInstance{}, ErrUnsafe
		}
	case "inactive", "failed":
		if pid != 0 || (instance.Invocation != "" && !invocationPattern.MatchString(instance.Invocation)) {
			return workerInstance{}, ErrUnsafe
		}
	default:
		return workerInstance{}, ErrUnsafe
	}
	return instance, nil
}

func sameWorker(ctx context.Context, unit string, want workerInstance) bool {
	got, err := inspectWorker(ctx, unit)
	return err == nil && want.Active == "active" && got == want
}
