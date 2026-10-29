//go:build linux

package connectedinstall

import (
	"strings"
	"testing"
)

func TestRichLayoutIsFixedAndNeverAdoptsNumericPaths(t *testing.T) {
	l := richInstallLayout{installLayout{digest: strings.Repeat("a", 64), collectorUID: 61001, uploaderUID: 61002, uploaderGID: 61012, sharedGID: 61011}}
	config, err := l.path(dirUploaderConfig)
	if err != nil || config != "/var/lib/home-lab-observer-connected-rich/uploader-root/etc/home-lab-observer-connected-rich" {
		t.Fatal("rich inner configuration mountpoint is incorrect")
	}
	for role := dirOptRoot; role <= dirCredentials; role++ {
		spec, err := l.directory(role)
		if err != nil {
			t.Fatal("rich directory refused")
		}
		path := spec.parent + "/" + spec.name
		if !strings.Contains(path, "home-lab-observer-connected-rich") || strings.Contains(path, "home-lab-observer-connected/") {
			t.Fatal("rich directory aliases numeric root")
		}
	}
	for _, role := range []fileRole{fileCollectorUnit, fileUploaderUnit, fileInstalledConfig, fileCredential, fileCA, fileHosts, fileResolver, fileNSS, richPolicyFile} {
		spec, err := l.file(role)
		if err != nil || strings.Contains(spec.parent+"/"+spec.name, "home-lab-observer-connected/") {
			t.Fatal("rich file aliases numeric root")
		}
	}
	if _, err := l.directory(directoryRole(255)); err != ErrUnsafe {
		t.Fatal("unknown directory role accepted")
	}
	if _, err := l.file(fileRole(255)); err != ErrUnsafe {
		t.Fatal("unknown file role accepted")
	}
	if _, err := l.installLayout.file(richPolicyFile); err != ErrUnsafe {
		t.Fatal("numeric layout gained rich policy role")
	}
}
