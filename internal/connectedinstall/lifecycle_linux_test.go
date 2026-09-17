//go:build linux

package connectedinstall

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPreparingMetadataIsCanonicalClosedAndBounded(t *testing.T) {
	r := preparingRecord{Version: "observer-connected-preparing/v1", State: "PREPARING", ServerID: "srv_" + strings.Repeat("a", 32), ManifestSHA256: strings.Repeat("b", 64)}
	data, err := json.Marshal(r)
	if err != nil || !validPreparing(data) {
		t.Fatal("valid preparation refused")
	}
	for _, bad := range [][]byte{nil, append(append([]byte(nil), data...), '\n'), []byte(strings.Replace(string(data), "PREPARING", "INSTALLED_READY", 1)), []byte(strings.Replace(string(data), r.ManifestSHA256, strings.Repeat("z", 64), 1)), []byte(strings.Repeat("x", 1025))} {
		if validPreparing(bad) {
			t.Fatal("ambiguous preparation accepted")
		}
	}
}

func TestStopRequiresInactiveAndNoMainProcess(t *testing.T) {
	for _, good := range []string{"ActiveState=inactive\nMainPID=0\n", "MainPID=0\nActiveState=inactive\n"} {
		if !stoppedProperties([]byte(good)) {
			t.Fatal("stopped unit refused")
		}
	}
	for _, bad := range []string{"ActiveState=active\nMainPID=0\n", "ActiveState=inactive\nMainPID=1\n", "ActiveState=inactive\nMainPID=0\nMainPID=1\n", "MainPID=0\n", "ActiveState=failed\nMainPID=0\n"} {
		if stoppedProperties([]byte(bad)) {
			t.Fatal("unjoined or ambiguous unit accepted")
		}
	}
}
