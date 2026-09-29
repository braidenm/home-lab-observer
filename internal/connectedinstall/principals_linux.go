//go:build linux

package connectedinstall

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

type principalSnapshot struct {
	users   map[string]string
	groups  map[string]string
	members map[string]string
}

// auditPrincipals checks the supported local-files-first identity profile.
// Effective fixed lookups confirm these roles, not universal NSS enumeration.
// Root-managed systemd providers and existing process identity admission remain
// trusted host responsibilities; this function never edits NSS or accounts.
func auditPrincipals(parent context.Context, c connectedprofile.Config) error {
	if parent == nil || os.Getuid() != 0 || os.Geteuid() != 0 || c.Validate() != nil {
		return ErrUnsafe
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	nss, err := connectedprofile.ReadRootFile("/etc/nsswitch.conf", 64<<10)
	if err != nil {
		return ErrUnsafe
	}
	passwd, err := connectedprofile.ReadRootFile("/etc/passwd", 1<<20)
	if err != nil {
		return ErrUnsafe
	}
	groups, err := connectedprofile.ReadRootFile("/etc/group", 1<<20)
	if err != nil {
		return ErrUnsafe
	}
	snapshot, err := parsePrincipals(nss, passwd, groups, c)
	if err != nil {
		return ErrUnsafe
	}
	for _, path := range []string{"/run/systemd/userdb", "/etc/userdb", "/run/userdb", "/usr/lib/userdb"} {
		d, err := trustedDirectory(path, true)
		if err != nil {
			return ErrUnsafe
		}
		if d != nil && d.Close() != nil {
			return ErrUnsafe
		}
	}
	// /run/host is optional on the supported host profile. A missing parent
	// must be checked explicitly rather than treated as the final userdb child.
	host, err := trustedDirectory("/run/host", true)
	if err != nil {
		return ErrUnsafe
	}
	if host != nil {
		if host.Close() != nil {
			return ErrUnsafe
		}
		userdb, err := trustedDirectory("/run/host/userdb", true)
		if err != nil {
			return ErrUnsafe
		}
		if userdb != nil && userdb.Close() != nil {
			return ErrUnsafe
		}
	}
	for _, user := range []struct {
		name     string
		uid, gid uint32
		groups   []uint32
	}{
		// glibc getent initgroups supplies no primary GID to getgrouplist.
		// Primary GIDs are checked independently in the exact passwd records.
		{collectorName, c.CollectorUID, c.SharedGID, nil},
		{uploaderName, c.UploaderUID, c.UploaderGID, []uint32{c.SharedGID}},
	} {
		for _, key := range []string{user.name, strconv.FormatUint(uint64(user.uid), 10)} {
			out, err := runTool(ctx, toolGetent, []string{"passwd", key}, nil, 4096)
			if err != nil || string(out) != snapshot.users[user.name]+"\n" {
				return ErrUnsafe
			}
		}
		out, err := runTool(ctx, toolGetent, []string{"initgroups", user.name}, nil, 4096)
		if err != nil || !exactInitgroups(out, user.name, user.groups) {
			return ErrUnsafe
		}
		out, err = runTool(ctx, toolPasswd, []string{"--status", "--", user.name}, nil, 1024)
		if err != nil || !lockedStatus(out, user.name) {
			return ErrUnsafe
		}
	}
	for _, group := range []struct {
		name string
		gid  uint32
	}{{sharedName, c.SharedGID}, {uploaderName, c.UploaderGID}} {
		for _, key := range []string{group.name, strconv.FormatUint(uint64(group.gid), 10)} {
			out, err := runTool(ctx, toolGetent, []string{"group", key}, nil, 4096)
			if err != nil || string(out) != snapshot.groups[group.name]+"\n" {
				return ErrUnsafe
			}
		}
		// Query only our two dedicated groups, never the host's full shadow data.
		out, err := runTool(ctx, toolGetent, []string{"gshadow", group.name}, nil, 4096)
		valid := err == nil && lockedGroup(out, group.name, snapshot.members[group.name])
		clear(out)
		if !valid {
			return ErrUnsafe
		}
	}
	if ctx.Err() != nil {
		return ErrUnsafe
	}
	return nil
}

func supportedNSS(data []byte) bool {
	if len(data) == 0 || len(data) > 64<<10 || bytes.ContainsAny(data, "\r\x00") {
		return false
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line, _, _ = strings.Cut(line, "#")
		key, value, ok := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		if key == "initgroups" {
			return false
		}
		if key != "passwd" && key != "group" && key != "shadow" && key != "gshadow" {
			continue
		}
		if !ok || seen[key] {
			return false
		}
		seen[key] = true
		fields := strings.Fields(value)
		if len(fields) < 1 || len(fields) > 2 || fields[0] != "files" || len(fields) == 2 && fields[1] != "systemd" {
			return false
		}
	}
	return seen["passwd"] && seen["group"] && seen["shadow"] && seen["gshadow"]
}

func parsePrincipals(nss, passwd, groups []byte, c connectedprofile.Config) (principalSnapshot, error) {
	result := principalSnapshot{users: map[string]string{}, groups: map[string]string{}, members: map[string]string{}}
	if c.Validate() != nil || !supportedNSS(nss) || len(passwd) > 1<<20 || len(groups) > 1<<20 || bytes.ContainsAny(passwd, "\r\x00") || bytes.ContainsAny(groups, "\r\x00") {
		return result, ErrUnsafe
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(passwd), "\n"), "\n") {
		f := strings.Split(line, ":")
		if len(f) != 7 || len(line) > 4096 {
			return result, ErrUnsafe
		}
		uid, ok := numericID(f[2])
		if !ok {
			return result, ErrUnsafe
		}
		gid, ok := numericID(f[3])
		if !ok {
			return result, ErrUnsafe
		}
		if f[0] != collectorName && f[0] != uploaderName {
			if uid == c.CollectorUID || uid == c.UploaderUID || gid == c.SharedGID || gid == c.UploaderGID {
				return result, ErrUnsafe
			}
			continue
		}
		if _, seen := result.users[f[0]]; seen {
			return result, ErrUnsafe
		}
		wantUID, wantGID := c.CollectorUID, c.SharedGID
		if f[0] == uploaderName {
			wantUID, wantGID = c.UploaderUID, c.UploaderGID
		}
		if uid != wantUID || gid != wantGID || f[1] != "x" || f[5] != "/nonexistent" || f[6] != "/usr/sbin/nologin" {
			return result, ErrUnsafe
		}
		result.users[f[0]] = line
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(groups), "\n"), "\n") {
		f := strings.Split(line, ":")
		if len(f) != 4 || len(line) > 4096 {
			return result, ErrUnsafe
		}
		gid, ok := numericID(f[2])
		if !ok {
			return result, ErrUnsafe
		}
		if f[0] != sharedName && f[0] != uploaderName {
			if gid == c.SharedGID || gid == c.UploaderGID {
				return result, ErrUnsafe
			}
			for _, member := range strings.Split(f[3], ",") {
				if member == collectorName || member == uploaderName {
					return result, ErrUnsafe
				}
			}
			continue
		}
		if _, seen := result.groups[f[0]]; seen {
			return result, ErrUnsafe
		}
		wantGID, wantMembers := c.SharedGID, uploaderName
		if f[0] == uploaderName {
			wantGID, wantMembers = c.UploaderGID, ""
		}
		if gid != wantGID || f[1] != "x" || f[3] != wantMembers {
			return result, ErrUnsafe
		}
		result.groups[f[0]] = line
		result.members[f[0]] = wantMembers
	}
	if len(result.users) != 2 || len(result.groups) != 2 {
		return result, ErrUnsafe
	}
	return result, nil
}

func numericID(s string) (uint32, bool) {
	n, err := strconv.ParseUint(s, 10, 32)
	return uint32(n), err == nil && n < uint64(^uint32(0)) && strconv.FormatUint(n, 10) == s
}

func exactInitgroups(data []byte, name string, groups []uint32) bool {
	if bytes.ContainsAny(data, "\r\x00") || bytes.Count(data, []byte("\n")) != 1 || !bytes.HasSuffix(data, []byte("\n")) {
		return false
	}
	fields := strings.Fields(string(data))
	if len(fields) != len(groups)+1 || fields[0] != name {
		return false
	}
	want := map[uint32]bool{}
	for _, gid := range groups {
		want[gid] = true
	}
	for _, field := range fields[1:] {
		gid, ok := numericID(field)
		if !ok || !want[gid] {
			return false
		}
		delete(want, gid)
	}
	return len(want) == 0
}

func lockedStatus(data []byte, name string) bool {
	if bytes.ContainsAny(data, "\r\x00") || bytes.Count(data, []byte("\n")) != 1 || !bytes.HasSuffix(data, []byte("\n")) {
		return false
	}
	fields := strings.Fields(string(data))
	return len(fields) == 7 && fields[0] == name && fields[1] == "L"
}

func lockedGroup(data []byte, name, members string) bool {
	if bytes.ContainsAny(data, "\r\x00") || bytes.Count(data, []byte("\n")) != 1 || !bytes.HasSuffix(data, []byte("\n")) {
		return false
	}
	f := strings.Split(strings.TrimSuffix(string(data), "\n"), ":")
	return len(f) == 4 && f[0] == name && (strings.HasPrefix(f[1], "!") || strings.HasPrefix(f[1], "*")) && f[2] == "" && f[3] == members
}
