//go:build linux

package actionengine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
	"github.com/braidenm/home-lab-observer/internal/actionjournal"
	"golang.org/x/sys/unix"
)

// Only the separately reviewed Node orchestrator opts into this entry. Normal
// CI never discovers a daemon or mutates anything. No production bypass API.
func TestOwnedNativeFixture(t *testing.T) {
	phase := os.Getenv("HLO_ENGINE_NATIVE_PHASE")
	if phase == "" {
		t.Skip("owned native fixture execution requires separate approval")
	}
	if phase != "version" && phase != "graceful" && phase != "ignore" {
		t.Fatal("native phase refused")
	}
	fmt.Println("HLO_ENGINE_NATIVE_STAGE=VERSION")
	// Fixture-only canonicalization of this fixed Desktop/Linux namespace alias;
	// production Open still accepts only its fixed no-symlink /engine path.
	path, err := filepath.EvalSymlinks("/var/run/docker.sock")
	if err != nil || !(path == "/run/docker.sock" || strings.HasPrefix(path, "/mnt/wsl/docker-desktop/")) {
		t.Fatal("native socket namespace refused")
	}
	var st unix.Stat_t
	if unix.Lstat(path, &st) != nil || st.Uid != 0 || st.Gid == 0 {
		t.Fatal("native socket owner refused")
	}
	a := openFixture(t, socketEndpoint{path: path, owner: 0, approved: SocketWitness{Device: uint64(st.Dev), Inode: st.Ino, GroupID: st.Gid}})
	body, status, err := a.request(context.Background(), "GET", "/version", 4*time.Second, versionLimit)
	if err != nil || status != 200 {
		t.Fatal("native version unavailable")
	}
	fields, err := selectedObject(body, "Version", "ApiVersion", "MinAPIVersion", "Os", "Arch")
	if err != nil {
		t.Fatal("native version shape")
	}
	values := make([]string, 5)
	for n, key := range []string{"Version", "ApiVersion", "MinAPIVersion", "Os", "Arch"} {
		if json.Unmarshal(fields[key], &values[n]) != nil || len(values[n]) > 64 || strings.ContainsAny(values[n], "|\r\n\x00") {
			t.Fatal("native version value")
		}
	}
	if values[3] != "linux" || values[4] != "amd64" {
		t.Fatal("native platform refused")
	}
	tuple := strings.Join(values, "|")
	if phase == "version" {
		fmt.Println("HLO_ENGINE_NATIVE_VERSION=" + tuple)
		return
	}
	if tuple != os.Getenv("HLO_ENGINE_NATIVE_VERSION") {
		t.Fatal("CLI/adapter daemon mismatch")
	}
	id, image, run := os.Getenv("HLO_ENGINE_NATIVE_ID"), os.Getenv("HLO_ENGINE_NATIVE_IMAGE"), os.Getenv("HLO_ENGINE_NATIVE_RUN")
	if !validID(id) || !strings.HasPrefix(image, "sha256:") || !validID(strings.TrimPrefix(image, "sha256:")) || len(run) != 32 || !validID(run+run) {
		t.Fatal("native identity refused")
	}
	owned := &ownedNativeEngine{Engine: a, id: id, image: image, run: run, mode: phase}
	fmt.Println("HLO_ENGINE_NATIVE_STAGE=IDENTITY")
	before := owned.verify(t)
	if before.State.Status != "created" {
		t.Fatal("fixture must be newly created")
	}
	request, binding, _, _ := workerInput(actioncore.Start)
	binding.ContainerID = id
	binding = nativeApproval(binding, phase)
	approved, err := json.Marshal(binding)
	if err != nil {
		t.Fatal("native approval snapshot")
	}
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	j, err := actionjournal.Provision(context.Background(), dir, actionjournal.Binding{ServerID: binding.ServerID, InstallationID: binding.InstallationID, OwnerID: binding.OwnerID})
	if err != nil {
		t.Fatal("native journal provision")
	}
	defer j.Close()
	ops := []actioncore.Operation{actioncore.Start, actioncore.Restart, actioncore.Stop}
	if phase == "ignore" {
		ops = []actioncore.Operation{actioncore.Start, actioncore.Stop}
	}
	for n, op := range ops {
		if n > 0 {
			// Exercise the real journal cooldown, never override its clock or retry.
			<-time.NewTimer(30 * time.Second).C
		}
		now := time.Now().UTC()
		request.ActionID = fmt.Sprintf("hla_%032x", n+1)
		request.Operation = op
		request.CreatedAt, request.ExpiresAt = now.Add(-time.Second), now.Add(4*time.Minute)
		request.GraceSeconds = 0
		if op != actioncore.Start {
			request.GraceSeconds = binding.GraceSeconds
		}
		current, err := json.Marshal(binding)
		if err != nil || !bytes.Equal(current, approved) {
			t.Fatal("native approval changed within fixture")
		}
		delivery := actioncore.Delivery{StartedAt: now, ReceivedAt: now, LeaseDeadline: now.Add(2 * time.Minute)}
		owned.test = t
		worker := actioncore.New(&nativeOwnerAuthority{owned}, j, owned, time.Now)
		previous := owned.verify(t)
		fmt.Println("HLO_ENGINE_NATIVE_STAGE=" + map[actioncore.Operation]string{actioncore.Start: "START", actioncore.Stop: "STOP", actioncore.Restart: "RESTART"}[op])
		want := map[actioncore.Operation]actioncore.Code{actioncore.Start: actioncore.Started, actioncore.Stop: actioncore.Stopped, actioncore.Restart: actioncore.Restarted}[op]
		if worker.Execute(context.Background(), request, binding, delivery) != want {
			t.Fatal("native worker acknowledgement refused")
		}
		after := owned.verify(t)
		if op != actioncore.Stop && after.State.Status != "running" {
			t.Fatal("synthetic fixture did not remain running")
		}
		calls := owned.calls
		fmt.Println("HLO_ENGINE_NATIVE_STAGE=REPLAY")
		if worker.Execute(context.Background(), request, binding, delivery) != want || owned.calls != calls || owned.verify(t).State.StartedAt != after.State.StartedAt {
			t.Fatal("native retained replay changed fixture")
		}
		if op == actioncore.Restart && previous.State.StartedAt == after.State.StartedAt {
			t.Fatal("restart lacked new fixture start evidence")
		}
		if op == actioncore.Stop && (after.State.Status != "exited" || (phase == "graceful" && after.State.ExitCode != 0) || (phase == "ignore" && after.State.ExitCode != 137)) {
			t.Fatal("graceful/forced fixture outcome refused")
		}
	}
	fmt.Println("HLO_ENGINE_NATIVE_CASE_PASS=" + phase)
}

func nativeApproval(b actioncore.Binding, mode string) actioncore.Binding {
	switch mode {
	case "graceful":
		b.Operations = []actioncore.Operation{actioncore.Start, actioncore.Stop, actioncore.Restart}
		b.GraceSeconds = 30
	case "ignore":
		b.Operations = []actioncore.Operation{actioncore.Start, actioncore.Stop}
		b.GraceSeconds = 1
	default:
		return actioncore.Binding{}
	}
	return b
}

func TestNativeFixtureApprovalDoesNotChangePerRequest(t *testing.T) {
	for _, mode := range []string{"graceful", "ignore"} {
		r, base, _, _ := workerInput(actioncore.Start)
		b := nativeApproval(base, mode)
		frozen, err := json.Marshal(b)
		if err != nil {
			t.Fatal("approval fixture")
		}
		for _, op := range b.Operations {
			r.Operation = op
			r.GraceSeconds = 0
			if op != actioncore.Start {
				r.GraceSeconds = b.GraceSeconds
			}
			if _, err := actioncore.NewJournalKey(r, b); err != nil {
				t.Fatal("request no longer matches immutable approval")
			}
			current, _ := json.Marshal(b)
			if !bytes.Equal(current, frozen) || b.PolicyRevision != 1 || b.CapabilityRevision != 1 {
				t.Fatal("approval changed without a revision")
			}
		}
		if mode == "ignore" {
			r.Operation, r.GraceSeconds = actioncore.Restart, b.GraceSeconds
			if _, err := actioncore.NewJournalKey(r, b); err == nil {
				t.Fatal("ignore fixture gained restart approval")
			}
		}
	}
}

type nativeContainer struct {
	ID     string `json:"Id"`
	Image  string
	Config struct {
		User            string
		Entrypoint, Cmd []string
		Labels          map[string]string
	}
	HostConfig struct {
		NetworkMode, PidMode, IpcMode, UTSMode                  string
		ReadonlyRootfs, Privileged, PublishAllPorts, AutoRemove bool
		Memory, MemorySwap, NanoCpus, PidsLimit                 int64
		CapAdd, CapDrop, SecurityOpt, Binds, VolumesFrom        []string
		Devices, DeviceRequests                                 []json.RawMessage
		PortBindings, Tmpfs                                     map[string]json.RawMessage
		RestartPolicy                                           struct{ Name string }
		LogConfig                                               struct{ Type string }
	}
	Mounts []json.RawMessage
	State  struct {
		Status, StartedAt string
		ExitCode          int
	}
}

// This wrapper belongs only to the opted-in test. It checks ownership before
// every real delegate call; labels/image are not production action authority.
type ownedNativeEngine struct {
	*Engine
	id, image, run, mode string
	test                 *testing.T
	calls                int
}

func (a *ownedNativeEngine) verify(t *testing.T) nativeContainer {
	return a.verifyContext(context.Background(), t)
}
func (a *ownedNativeEngine) verifyContext(ctx context.Context, t *testing.T) nativeContainer {
	t.Helper()
	body, status, err := a.request(ctx, "GET", "/v1.45/containers/"+a.id+"/json", 4*time.Second, inspectLimit)
	c, decodeErr := decodeNativeContainer(body)
	if err != nil || status != 200 || decodeErr != nil {
		t.Fatal("native fixture identity unavailable")
	}
	h := c.HostConfig
	if c.ID != a.id || c.Image != a.image || c.Config.Labels["hlo.action-fixture"] != a.run || len(c.Config.Labels) != 1 || c.Config.User != "65532:65532" ||
		strings.Join(c.Config.Entrypoint, "|") != "/fixture" || strings.Join(c.Config.Cmd, "|") != a.mode ||
		h.NetworkMode != "none" || !h.ReadonlyRootfs || h.Privileged || h.PublishAllPorts || h.AutoRemove ||
		h.Memory != 33554432 || h.MemorySwap != 33554432 || h.NanoCpus != 250000000 || h.PidsLimit != 64 ||
		len(h.CapAdd) != 0 || len(h.CapDrop) != 1 || h.CapDrop[0] != "ALL" || len(h.SecurityOpt) != 1 || h.SecurityOpt[0] != "no-new-privileges:true" ||
		len(h.Binds)+len(h.VolumesFrom)+len(h.Devices)+len(h.DeviceRequests)+len(h.PortBindings)+len(h.Tmpfs)+len(c.Mounts) != 0 ||
		h.PidMode != "" || h.IpcMode != "private" || h.UTSMode != "" || h.RestartPolicy.Name != "no" || h.LogConfig.Type != "none" {
		t.Fatal("native fixture ownership/caps refused")
	}
	return c
}

// Test ownership checks use the same exact-key selection, not case folding of
// unknown alias fields into a privileged/identity-bearing fixture field.
func decodeNativeContainer(body []byte) (nativeContainer, error) {
	var c nativeContainer
	f, err := selectedObject(body, "Id", "Image", "Config", "HostConfig", "Mounts", "State")
	if err != nil {
		return c, err
	}
	for _, section := range []struct {
		name string
		keys []string
	}{
		{"Config", []string{"User", "Entrypoint", "Cmd", "Labels"}},
		{"State", []string{"Status", "StartedAt", "ExitCode"}},
		{"HostConfig", []string{"NetworkMode", "PidMode", "IpcMode", "UTSMode", "ReadonlyRootfs", "Privileged", "PublishAllPorts", "AutoRemove",
			"Memory", "MemorySwap", "NanoCpus", "PidsLimit", "CapAdd", "CapDrop", "SecurityOpt", "Binds", "VolumesFrom", "Devices", "DeviceRequests", "PortBindings", "RestartPolicy", "LogConfig"}},
	} {
		fields, err := selectedObject(f[section.name], section.keys...)
		if err != nil {
			return c, err
		}
		if section.name == "State" {
			for _, key := range section.keys {
				if bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
					return c, ErrUnavailable
				}
			}
		}
		if section.name == "HostConfig" {
			for _, key := range []string{"NetworkMode", "PidMode", "IpcMode", "UTSMode", "ReadonlyRootfs", "Privileged", "PublishAllPorts", "AutoRemove", "Memory", "MemorySwap", "NanoCpus", "PidsLimit"} {
				if bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
					return c, ErrUnavailable
				}
			}
			// Moby's fixed API1.45 HostConfig uses omitempty for Tmpfs. Its
			// absence means no configured tmpfs; present data still gets exact
			// key/duplicate checking and remains visible to the empty-caps check.
			var all map[string]json.RawMessage
			if json.Unmarshal(f[section.name], &all) != nil {
				return c, ErrUnavailable
			}
			if _, present := all["Tmpfs"]; present {
				optional, err := selectedObject(f[section.name], "Tmpfs")
				if err != nil {
					return c, err
				}
				fields["Tmpfs"] = optional["Tmpfs"]
			}
			for key, selected := range map[string]string{"RestartPolicy": "Name", "LogConfig": "Type"} {
				inner, err := selectedObject(fields[key], selected)
				if err != nil {
					return c, err
				}
				fields[key], err = json.Marshal(inner)
				if err != nil {
					return c, ErrUnavailable
				}
			}
		}
		f[section.name], err = json.Marshal(fields)
		if err != nil {
			return c, ErrUnavailable
		}
	}
	filtered, err := json.Marshal(f)
	if err != nil || json.Unmarshal(filtered, &c) != nil {
		return c, ErrUnavailable
	}
	return c, nil
}

func TestNativeOwnershipDecoderRejectsMissingAndFoldedFields(t *testing.T) {
	c := nativeContainer{ID: fullID, Image: "sha256:" + fullID}
	body, err := json.Marshal(c)
	if err != nil {
		t.Fatal("native decoder fixture")
	}
	if decoded, err := decodeNativeContainer(body); err != nil || decoded.ID != fullID {
		t.Fatal("native decoder positive")
	}
	withoutTmpfs := []byte(strings.Replace(string(body), `"Tmpfs":null,`, "", 1))
	if decoded, err := decodeNativeContainer(withoutTmpfs); err != nil || len(decoded.HostConfig.Tmpfs) != 0 {
		t.Fatal("schema-optional empty Tmpfs refused")
	}
	withTmpfs := []byte(strings.Replace(string(body), `"Tmpfs":null`, `"Tmpfs":{"/foreign":"rw"}`, 1))
	if decoded, err := decodeNativeContainer(withTmpfs); err != nil || len(decoded.HostConfig.Tmpfs) != 1 {
		t.Fatal("nonempty Tmpfs hidden from cap validation")
	}
	for _, body := range [][]byte{
		[]byte(strings.Replace(string(body), `"Privileged":false`, `"privileged":false`, 1)),
		[]byte(strings.Replace(string(body), `"Privileged":false`, `"Privileged":false,"Privileged":true`, 1)),
		[]byte(strings.Replace(string(body), `"Name":""`, `"name":""`, 1)),
		[]byte(strings.Replace(string(body), `"Id":`, `"ID":`, 1)),
		[]byte(strings.Replace(string(body), `"Tmpfs":null`, `"Tmpfs":{},"Tmpfs":{}`, 1)),
		[]byte(strings.Replace(string(body), `"Tmpfs":null`, `"Tmpfs":true`, 1)),
		[]byte(strings.Replace(string(body), `"Privileged":false`, `"Privileged":null`, 1)),
		[]byte(strings.Replace(string(body), `"PidMode":""`, `"PidMode":null`, 1)),
		[]byte(strings.Replace(string(body), `"ExitCode":0`, `"ExitCode":null`, 1)),
	} {
		if _, err := decodeNativeContainer(body); err != ErrUnavailable {
			t.Fatal("native ownership field omission/folding accepted")
		}
	}
}
func (a *ownedNativeEngine) Start(ctx context.Context, id string) error {
	a.verifyContext(ctx, a.test)
	if id != a.id {
		return ErrInvalid
	}
	a.calls++
	return a.Engine.Start(ctx, id)
}
func (a *ownedNativeEngine) Stop(ctx context.Context, id string, grace uint8) error {
	a.verifyContext(ctx, a.test)
	if id != a.id {
		return ErrInvalid
	}
	a.calls++
	return a.Engine.Stop(ctx, id, grace)
}
func (a *ownedNativeEngine) Restart(ctx context.Context, id string, grace uint8) error {
	a.verifyContext(ctx, a.test)
	if id != a.id {
		return ErrInvalid
	}
	a.calls++
	return a.Engine.Restart(ctx, id, grace)
}

type nativeOwnerAuthority struct{ owned *ownedNativeEngine }

func (a *nativeOwnerAuthority) Check(ctx context.Context, _ actioncore.Request, _ actioncore.Binding) error {
	a.owned.verifyContext(ctx, a.owned.test)
	return nil
}
