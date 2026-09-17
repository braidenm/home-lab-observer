//go:build linux

package connectedinstall

import (
	"strings"
	"testing"
)

func workerFixture() string {
	return "ActiveState=active\nMainPID=123\nInvocationID=" + strings.Repeat("a", 32) + "\nRestart=no\nStandardInput=null\nStandardOutput=null\nStandardError=null\nFileDescriptorStoreMax=0\nNFileDescriptorStore=0\nTriggeredBy=\nUnitFileState=disabled\nPrivateIPC=yes\nMemoryPressureWatch=skip\n"
}

func TestActivationMemoryPressurePolicy(t *testing.T) {
	for _, value := range []string{"off", "on", "auto", "unknown", "", "SKIP"} {
		bad := strings.ReplaceAll(workerFixture(), "MemoryPressureWatch=skip", "MemoryPressureWatch="+value)
		if _, err := parseWorker([]byte(bad)); err != ErrUnsafe {
			t.Fatal("memory-pressure drift accepted")
		}
	}
	for _, suffix := range []string{"", "MemoryPressureWatch=skip\nMemoryPressureWatch=skip\n"} {
		bad := strings.ReplaceAll(workerFixture(), "MemoryPressureWatch=skip\n", suffix)
		if _, err := parseWorker([]byte(bad)); err != ErrUnsafe {
			t.Fatal("missing or duplicate memory-pressure policy accepted")
		}
	}
}

func TestActivationWorkerProperties(t *testing.T) {
	b := workerFixture()
	if got, err := parseWorker([]byte(b)); err != nil || got.PID != 123 {
		t.Fatal("valid active refused")
	}
	inactive := strings.ReplaceAll(strings.ReplaceAll(b, "ActiveState=active", "ActiveState=inactive"), "MainPID=123", "MainPID=0")
	if _, err := parseWorker([]byte(inactive)); err != nil {
		t.Fatal("valid stopped refused")
	}
	for _, state := range []string{"activating", "deactivating"} {
		transition := strings.ReplaceAll(strings.ReplaceAll(b, "ActiveState=active", "ActiveState="+state), "MainPID=123", "MainPID=0")
		if got, err := parseWorker([]byte(transition)); err != nil || got.PID != 0 || got.Invocation == "" {
			t.Fatal("transition lost invocation identity")
		}
	}
	for name, bad := range map[string]string{
		"shared IPC":         strings.ReplaceAll(b, "PrivateIPC=yes", "PrivateIPC=no"),
		"missing IPC":        strings.ReplaceAll(b, "PrivateIPC=yes\n", ""),
		"restart":            strings.ReplaceAll(b, "Restart=no", "Restart=on-failure"),
		"stdio":              strings.ReplaceAll(b, "StandardInput=null", "StandardInput=socket"),
		"configured store":   strings.ReplaceAll(b, "FileDescriptorStoreMax=0", "FileDescriptorStoreMax=1"),
		"stored fd":          strings.ReplaceAll(b, "NFileDescriptorStore=0", "NFileDescriptorStore=1"),
		"socket activation":  strings.ReplaceAll(b, "TriggeredBy=", "TriggeredBy=other.socket"),
		"boot enabled":       strings.ReplaceAll(b, "UnitFileState=disabled", "UnitFileState=enabled"),
		"duplicate":          b + "MainPID=123\n",
		"unknown":            b + "Other=\n",
		"missing":            strings.ReplaceAll(b, "TriggeredBy=\n", ""),
		"leading zero pid":   strings.ReplaceAll(b, "MainPID=123", "MainPID=0123"),
		"missing invocation": strings.ReplaceAll(b, strings.Repeat("a", 32), ""),
		"dead active":        strings.ReplaceAll(b, "MainPID=123", "MainPID=0"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseWorker([]byte(bad)); err == nil {
				t.Fatal("unsafe properties accepted")
			}
		})
	}
}
