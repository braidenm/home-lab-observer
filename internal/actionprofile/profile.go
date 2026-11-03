// Package actionprofile owns fixed local identity/path checks, not credentials,
// network clients, Engine operations, remote authority or installed-unit proof.
package actionprofile

import (
	"errors"
	"strconv"
	"strings"
)

const (
	WorkerName            = "home-lab-observer-action-worker"
	BrokerName            = "home-lab-observer-action-broker"
	IPCName               = "home-lab-observer-action-ipc"
	WorkerSettingsPath    = "/etc/home-lab-observer-actions/worker.json"
	ActiveCredentialPath  = "/var/lib/home-lab-observer-actions/worker/active.json"
	WorkerSocketPath      = "/run/home-lab-observer-actions/worker/worker.sock"
	AdminSocketPath       = "/run/home-lab-observer-actions/admin/recovery.sock"
	JournalPath           = "/var/lib/home-lab-observer-actions/journal"
	PolicyPath            = "/etc/home-lab-observer-actions/policy.json"
	EngineSettingsPath    = "/etc/home-lab-observer-actions/engine.json"
	HostUserNamespacePath = "/run/home-lab-observer-actions/host-userns"
)

var ErrProfile = errors.New("action_profile_unavailable")

type Identity struct{ WorkerUID, WorkerGID, BrokerUID, BrokerGID, IPCGID uint32 }
type localRoles struct {
	identity          Identity
	brokerMemberships []uint32
}
type account struct {
	name, shell string
	uid, gid    uint32
}
type group struct {
	name    string
	gid     uint32
	members []string
}

func decimalID(s string) (uint32, bool) {
	n, e := strconv.ParseUint(s, 10, 32)
	return uint32(n), e == nil && n < 1<<32-1 && strconv.FormatUint(n, 10) == s
}
func accountText(s string) bool {
	if len(s) == 0 || len(s) > 256*1024 {
		return false
	}
	for _, b := range []byte(s) {
		if b == 0 || b > 127 || b == '\r' {
			return false
		}
	}
	return true
}
func localSources(nss string) bool {
	if !accountText(nss) {
		return false
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(nss, "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 || parts[0] != "passwd" && parts[0] != "group" {
			continue
		}
		if seen[parts[0]] {
			return false
		}
		seen[parts[0]] = true
		providers := strings.Fields(parts[1])
		if len(providers) < 1 || len(providers) > 2 || providers[0] != "files" || len(providers) == 2 && providers[1] != "systemd" {
			return false
		}
	}
	return seen["passwd"] && seen["group"]
}
func resolveIdentities(passwd, groups, nss string) (localRoles, error) {
	if !accountText(passwd) || !accountText(groups) || !localSources(nss) {
		return localRoles{}, ErrProfile
	}
	users := map[string]account{}
	uidCounts := map[uint32]int{}
	for _, line := range strings.Split(passwd, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := strings.Split(line, ":")
		if len(p) != 7 || p[0] == "" {
			return localRoles{}, ErrProfile
		}
		uid, u := decimalID(p[2])
		gid, g := decimalID(p[3])
		if _, duplicate := users[p[0]]; duplicate || !u || !g {
			return localRoles{}, ErrProfile
		}
		users[p[0]] = account{p[0], p[6], uid, gid}
		uidCounts[uid]++
	}
	byName := map[string]group{}
	gidCounts := map[uint32]int{}
	all := []group{}
	for _, line := range strings.Split(groups, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := strings.Split(line, ":")
		if len(p) != 4 || p[0] == "" {
			return localRoles{}, ErrProfile
		}
		gid, ok := decimalID(p[2])
		if _, duplicate := byName[p[0]]; duplicate || !ok {
			return localRoles{}, ErrProfile
		}
		members := []string{}
		if p[3] != "" {
			members = strings.Split(p[3], ",")
		}
		seen := map[string]bool{}
		for _, member := range members {
			if member == "" || seen[member] {
				return localRoles{}, ErrProfile
			}
			seen[member] = true
		}
		g := group{p[0], gid, members}
		byName[p[0]] = g
		all = append(all, g)
		gidCounts[gid]++
	}
	w, wok := users[WorkerName]
	b, bok := users[BrokerName]
	wg, wgok := byName[WorkerName]
	bg, bgok := byName[BrokerName]
	ipc, iok := byName[IPCName]
	if !wok || !bok || !wgok || !bgok || !iok || w.uid == 0 || b.uid == 0 || w.uid == b.uid ||
		w.gid == 0 || b.gid == 0 || ipc.gid == 0 || w.gid != wg.gid || b.gid != bg.gid || w.gid == b.gid || w.gid == ipc.gid || b.gid == ipc.gid ||
		uidCounts[w.uid] != 1 || uidCounts[b.uid] != 1 || gidCounts[w.gid] != 1 || gidCounts[b.gid] != 1 || gidCounts[ipc.gid] != 1 {
		return localRoles{}, ErrProfile
	}
	for _, a := range []account{w, b} {
		if a.shell != "/usr/sbin/nologin" && a.shell != "/sbin/nologin" && a.shell != "/bin/false" {
			return localRoles{}, ErrProfile
		}
	}
	// Primary membership is stored in passwd, not the group's member list.
	// No foreign account may inherit either private file group or IPC access.
	for _, a := range users {
		if a.gid == w.gid && a.name != WorkerName || a.gid == b.gid && a.name != BrokerName || a.gid == ipc.gid {
			return localRoles{}, ErrProfile
		}
	}
	for _, g := range []group{wg, bg} {
		for _, member := range g.members {
			if member != g.name {
				return localRoles{}, ErrProfile
			}
		}
	}
	if len(ipc.members) != 2 || !(ipc.members[0] == WorkerName && ipc.members[1] == BrokerName || ipc.members[1] == WorkerName && ipc.members[0] == BrokerName) {
		return localRoles{}, ErrProfile
	}
	result := localRoles{identity: Identity{w.uid, w.gid, b.uid, b.gid, ipc.gid}}
	for _, g := range all {
		for _, member := range g.members {
			if member == WorkerName && g.gid != w.gid && g.gid != ipc.gid {
				return localRoles{}, ErrProfile
			}
			if member == BrokerName {
				result.brokerMemberships = append(result.brokerMemberships, g.gid)
			}
		}
	}
	return result, nil
}
