package actionprofile

import (
	"strings"
	"testing"
)

func roleFixture() (string, string, string) {
	passwd := WorkerName + ":x:4101:4201::/nonexistent:/usr/sbin/nologin\n" + BrokerName + ":x:4102:4202::/nonexistent:/usr/sbin/nologin\n"
	groups := WorkerName + ":x:4201:\n" + BrokerName + ":x:4202:\n" + IPCName + ":x:4203:" + WorkerName + "," + BrokerName + "\ndocker:x:4204:" + BrokerName + "\n"
	return passwd, groups, "passwd: files systemd\ngroup: files systemd\n"
}

func TestRoleResolutionUsesClosedStaticUniqueLocalIdentities(t *testing.T) {
	passwd, groups, nss := roleFixture()
	roles, err := resolveIdentities(passwd, groups, nss)
	if err != nil || roles.identity != (Identity{4101, 4201, 4102, 4202, 4203}) {
		t.Fatal("source-owned static roles")
	}
	for _, bad := range []struct{ p, g, n string }{
		{passwd + "alias:x:4101:9999::/x:/bin/false\n", groups, nss},
		{strings.Replace(passwd, ":4101:", ":0:", 1), groups, nss},
		{strings.Replace(passwd, "/usr/sbin/nologin", "/bin/sh", 1), groups, nss},
		{passwd, groups + "alias:x:4203:\n", nss},
		{passwd, strings.Replace(groups, WorkerName+","+BrokerName, WorkerName+",foreign", 1), nss},
		{passwd, groups + "foreign:x:4400:" + WorkerName + "\n", nss},
		{passwd, groups, "passwd: ldap files\ngroup: files\n"},
		{passwd, groups, "passwd: systemd\ngroup: files\n"},
	} {
		if _, err := resolveIdentities(bad.p, bad.g, bad.n); err == nil {
			t.Fatal("ambiguous/foreign local role admitted")
		}
	}
}

func TestFixedProfileHasNoCallerPathOrIdentityAliases(t *testing.T) {
	want := map[string]string{
		WorkerSettingsPath: "/etc/home-lab-observer-actions/worker.json", ActiveCredentialPath: "/var/lib/home-lab-observer-actions/worker/active.json",
		WorkerSocketPath: "/run/home-lab-observer-actions/worker/worker.sock", AdminSocketPath: "/run/home-lab-observer-actions/admin/recovery.sock",
		JournalPath: "/var/lib/home-lab-observer-actions/journal", PolicyPath: "/etc/home-lab-observer-actions/policy.json",
		EngineSettingsPath: "/etc/home-lab-observer-actions/engine.json", HostUserNamespacePath: "/run/home-lab-observer-actions/host-userns",
	}
	if len(want) != 8 {
		t.Fatal("fixed paths collide")
	}
	for actual, expected := range want {
		if actual != expected {
			t.Fatal("profile path drift")
		}
	}
}

func TestRoleResolutionRejectsForeignPrimaryMembershipInEveryProtectedGroup(t *testing.T) {
	p, g, n := roleFixture()
	for _, gid := range []string{"4201", "4202", "4203"} {
		t.Run(gid, func(t *testing.T) {
			if _, err := resolveIdentities(p+"foreign:x:4999:"+gid+"::/nonexistent:/bin/false\n", g, n); err == nil {
				t.Fatal("foreign primary membership admitted into protected group")
			}
		})
	}
}
