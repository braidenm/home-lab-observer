//go:build linux

package connectedinstall

import (
	"strings"
	"testing"
)

func TestStoppedActionUnitRejectsActivationAndDrift(t *testing.T) {
	valid := "Id=" + actionWorkerUnit + "\nLoadState=loaded\nActiveState=inactive\nUnitFileState=static\nFragmentPath=" +
		systemUnitDirectory + "/" + actionWorkerUnit + "\nDropInPaths=\nNeedDaemonReload=no\n"
	if !isStoppedActionUnit([]byte(valid), actionWorkerUnit) {
		t.Fatal("exact stopped unit refused")
	}
	for _, change := range [][2]string{
		{"ActiveState=inactive", "ActiveState=active"},
		{"UnitFileState=static", "UnitFileState=enabled"},
		{"DropInPaths=\n", "DropInPaths=/run/systemd/system/override.conf\n"},
		{"DropInPaths=\n", ""},
		{"NeedDaemonReload=no", "NeedDaemonReload=yes"},
		{systemUnitDirectory, "/run/systemd/system"},
	} {
		if isStoppedActionUnit([]byte(strings.Replace(valid, change[0], change[1], 1)), actionWorkerUnit) {
			t.Fatalf("unit drift accepted: %q", change[1])
		}
	}
}
