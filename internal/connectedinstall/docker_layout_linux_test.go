//go:build linux

package connectedinstall

import (
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
	"strings"
	"testing"
)

func TestDockerLayoutKeepsExclusiveRichRootsAndPrivilegeSeparation(t *testing.T) {
	l := dockerInstallLayout{richInstallLayout: richInstallLayout{installLayout{digest: strings.Repeat("a", 64), collectorUID: 62001, uploaderUID: 62002, uploaderGID: 62003, sharedGID: 62004}}, readerUID: 62005, frameGID: 62006, socketGID: 998}
	for _, role := range dockerDirectories {
		s, err := l.directory(role)
		if err != nil || !strings.Contains(s.parent+"/"+s.name, "home-lab-observer-connected-rich") {
			t.Fatal("Docker layout escaped rich roots")
		}
	}
	f, err := l.directory(dirDockerFrame)
	if err != nil || f.uid != 62005 || f.gid != 62006 || f.mode != 0750 {
		t.Fatal("frame directory authority")
	}
	s, err := l.directory(dirDockerStatus)
	if err != nil || s.uid != 62005 || s.gid != 62006 || s.mode != 0700 {
		t.Fatal("reader status authority")
	}
	for role, want := range map[fileRole]string{fileCollectorUnit: connectedunits.RichProcessesDockerCollectorUnit, fileUploaderUnit: connectedunits.RichProcessesDockerUploaderUnit, fileDockerReaderUnit: connectedunits.RichProcessesDockerReaderUnit} {
		s, err := l.file(role)
		if err != nil || s.parent != "/etc/systemd/system" || s.name != want || s.mode != 0644 {
			t.Fatal("Docker fixed unit file")
		}
	}
	if _, err := l.richInstallLayout.directory(dirDockerFrame); err != ErrUnsafe {
		t.Fatal("old layout gained Docker state")
	}
	if _, err := l.directory(directoryRole(255)); err != ErrUnsafe {
		t.Fatal("unknown Docker role admitted")
	}
	if _, err := l.file(fileRole(255)); err != ErrUnsafe {
		t.Fatal("unknown Docker file admitted")
	}
	l.frameGID = l.socketGID
	if _, err := l.directory(dirDockerFrame); err != ErrUnsafe {
		t.Fatal("daemon group adopted for frame ownership")
	}
}
