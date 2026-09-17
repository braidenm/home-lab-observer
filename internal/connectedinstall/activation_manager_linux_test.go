//go:build linux

package connectedinstall

import (
	"strings"
	"testing"
)

func workerFixture() string {
	return "ActiveState=active\nMainPID=123\nInvocationID=" + strings.Repeat("a", 32) + "\nRestart=no\nStandardInput=null\nStandardOutput=null\nStandardError=null\nFileDescriptorStoreMax=0\nNFileDescriptorStore=0\nTriggeredBy=\nUnitFileState=disabled\n"
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
	for name, bad := range map[string]string{
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
