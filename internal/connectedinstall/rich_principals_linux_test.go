//go:build linux

package connectedinstall

import (
	"strings"
	"testing"
)

func TestRichPrincipalParserKeepsExactNamesAndRejectsCrossProfileIDs(t *testing.T) {
	passwd := strings.ReplaceAll(fixturePasswd, "hlo-connected-", "hlo-connected-rich-")
	groups := strings.ReplaceAll(fixtureGroups, "hlo-connected-", "hlo-connected-rich-")
	c := principalFixture()
	if _, err := parsePrincipalsForPair([]byte(fixtureNSS), []byte(passwd), []byte(groups), c, richProcessesPair); err != nil {
		t.Fatal("fixed rich roles refused")
	}
	if _, err := parsePrincipals([]byte(fixtureNSS), []byte(passwd), []byte(groups), c); err != ErrUnsafe {
		t.Fatal("numeric parser adopted rich principals")
	}
	if _, err := parsePrincipalsForPair([]byte(fixtureNSS), []byte(fixturePasswd), []byte(fixtureGroups), c, richProcessesPair); err != ErrUnsafe {
		t.Fatal("rich parser adopted numeric principals")
	}
	for _, bad := range []string{passwd + "alias:x:60101:65534::/nonexistent:/usr/sbin/nologin\n", strings.Replace(passwd, "/usr/sbin/nologin", "/bin/bash", 1)} {
		if _, err := parsePrincipalsForPair([]byte(fixtureNSS), []byte(bad), []byte(groups), c, richProcessesPair); err != ErrUnsafe {
			t.Fatal("unsafe rich account accepted")
		}
	}
	if _, err := parsePrincipalsForPair([]byte(fixtureNSS), []byte(passwd), []byte(groups), c, workerPair(255)); err != ErrUnsafe {
		t.Fatal("unknown pair accepted")
	}
}
