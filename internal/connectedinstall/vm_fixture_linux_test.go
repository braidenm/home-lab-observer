//go:build linux && connected_vm_fixture

package connectedinstall

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/connectedbundle"
	"github.com/braidenm/home-lab-observer/internal/connectedcredential"
	"github.com/braidenm/home-lab-observer/internal/connectedenroll"
	"github.com/braidenm/home-lab-observer/internal/connectedpolicy"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
	"github.com/braidenm/home-lab-observer/internal/enrollmentcoord"
	"github.com/braidenm/home-lab-observer/internal/enrollmentstore"
	"github.com/braidenm/home-lab-observer/internal/sharedhandoff"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

const vmFixtureRoot = "/opt/observer-fixture"
const vmFixtureServer = "srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const vmFixtureConnector = "agent_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

// This test is deliberately excluded from normal builds and tests. It creates
// only synthetic state, but uses fixed installed paths inside an admitted VM.
func vmAdmission(t *testing.T, root bool) string {
	t.Helper()
	uuid := os.Getenv("HLO_DISPOSABLE_VM_UUID")
	if os.Getenv("HLO_DISPOSABLE_VM_CONFIRM") != "observer-connected-f1-synthetic-only" || len(uuid) != 36 {
		t.Fatal("VM_FIXTURE_ADMISSION_REQUIRED")
	}
	actual, err := os.ReadFile("/sys/class/dmi/id/product_uuid")
	if err != nil || strings.ToLower(strings.TrimSpace(string(actual))) != strings.ToLower(uuid) {
		t.Fatal("VM_FIXTURE_IDENTITY_REFUSED")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	virtual, err := command(ctx, "/usr/bin/systemd-detect-virt", []string{"--vm"}, nil, 64)
	if err != nil || (string(virtual) != "kvm\n" && string(virtual) != "qemu\n") || (root && (os.Getuid() != 0 || os.Geteuid() != 0)) {
		t.Fatal("VM_FIXTURE_PLATFORM_REFUSED")
	}
	return uuid
}

func TestConnectedVMOfflineAndCollector(t *testing.T) {
	uuid := vmAdmission(t, true)
	if connectedprofile.HardenSecretProcess() != nil {
		t.Fatal("VM_FIXTURE_HARDENING_FAILED")
	}
	trustedRoot, err := rootDirectory(vmFixtureRoot)
	if err != nil {
		t.Fatal("VM_FIXTURE_ROOT_REFUSED")
	}
	if trustedRoot.Close() != nil {
		t.Fatal("VM_FIXTURE_ROOT_CLOSE_FAILED")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	digest := os.Getenv("HLO_DISPOSABLE_BUNDLE_SHA256")
	bundle := vmFixtureRoot + "/bundle"
	manifest, err := connectedbundle.VerifyDirectory(bundle, digest)
	if err != nil || CheckRequest(ctx, bundle, digest, vmFixtureServer) != nil {
		t.Fatal("VM_FIXTURE_PREFLIGHT_FAILED")
	}
	lock, err := acquireLease()
	if err != nil {
		t.Fatal("VM_FIXTURE_LEASE_FAILED")
	}
	defer func() {
		if lock.Close() != nil {
			t.Error("VM_FIXTURE_LEASE_CLOSE_FAILED")
		}
	}()
	for _, path := range []string{connectedprofile.ConfigDirectory, connectedprofile.StateDirectory, "/opt/home-lab-observer-connected", activationParent,
		"/etc/systemd/system/" + collectorUnit, "/etc/systemd/system/" + uploaderUnit, "/etc/systemd/system/" + collectorUnit + ".d", "/etc/systemd/system/" + uploaderUnit + ".d"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatal("VM_FIXTURE_EXISTING_RESOURCE_REFUSED")
		}
	}
	for _, name := range []string{collectorName, uploaderName} {
		if _, err := user.Lookup(name); !missingUser(err) {
			t.Fatal("VM_FIXTURE_EXISTING_PRINCIPAL_REFUSED")
		}
	}
	for _, name := range []string{sharedName, uploaderName} {
		if _, err := user.LookupGroup(name); !missingGroup(err) {
			t.Fatal("VM_FIXTURE_EXISTING_GROUP_REFUSED")
		}
	}
	for _, unit := range []string{collectorUnit, uploaderUnit, enrollmentUnit} {
		out, e := command(ctx, "/usr/bin/systemctl", []string{"show", "--property=LoadState", "--value", unit}, nil, 64)
		if e != nil || string(out) != "not-found\n" {
			t.Fatal("VM_FIXTURE_EXISTING_UNIT_REFUSED")
		}
	}
	addresses, err := connectedpolicy.Resolve(ctx)
	if err != nil || connectedpolicy.ValidateParent(ctx, addresses) != nil {
		t.Fatal("VM_FIXTURE_ENDPOINT_PREFLIGHT_FAILED")
	}
	for _, path := range []string{connectedprofile.ConfigDirectory, connectedprofile.StateDirectory, "/opt/home-lab-observer-connected", connectedprofile.ReleaseDirectory} {
		if mkdirNew(path, 0, 0, 0755) != nil {
			t.Fatal("VM_FIXTURE_DIRECTORY_FAILED")
		}
	}
	for _, name := range []string{sharedName, uploaderName} {
		if _, err := command(ctx, "/usr/sbin/groupadd", []string{"--system", name}, nil, 1024); err != nil {
			t.Fatal("VM_FIXTURE_GROUP_FAILED")
		}
	}
	for _, entry := range []struct {
		name, primary string
		supplementary bool
	}{{collectorName, sharedName, false}, {uploaderName, uploaderName, true}} {
		args := []string{"--system", "--no-create-home", "--home-dir", "/nonexistent", "--shell", "/usr/sbin/nologin", "--gid", entry.primary}
		if entry.supplementary {
			args = append(args, "--groups", sharedName)
		}
		args = append(args, entry.name)
		if _, err := command(ctx, "/usr/sbin/useradd", args, nil, 1024); err != nil {
			t.Fatal("VM_FIXTURE_USER_FAILED")
		}
	}
	collector, e1 := user.Lookup(collectorName)
	uploader, e2 := user.Lookup(uploaderName)
	shared, e3 := user.LookupGroup(sharedName)
	if e1 != nil || e2 != nil || e3 != nil {
		t.Fatal("VM_FIXTURE_LOOKUP_FAILED")
	}
	cu, ok1 := numericID(collector.Uid)
	uu, ok2 := numericID(uploader.Uid)
	ug, ok3 := numericID(uploader.Gid)
	sg, ok4 := numericID(shared.Gid)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		t.Fatal("VM_FIXTURE_ID_FAILED")
	}
	c := connectedprofile.Config{Version: connectedprofile.Version, State: "INSTALLED_READY", ServerID: vmFixtureServer, ConnectorID: vmFixtureConnector, CollectorUID: cu, UploaderUID: uu, UploaderGID: ug, SharedGID: sg, ArtifactSHA256: digest, PolicyGeneration: 1, Addresses: addresses}
	if auditPrincipals(ctx, c) != nil {
		t.Fatal("VM_FIXTURE_PRINCIPAL_FAILED")
	}
	release := connectedprofile.ReleaseDirectory + "/" + digest
	if mkdirNew(release, 0, 0, 0755) != nil || copyBundle(bundle, release, manifest, digest) != nil || prepareRoot(release, cu, uu, ug, sg, addresses) != nil {
		t.Fatal("VM_FIXTURE_LAYOUT_FAILED")
	}
	configBytes, _ := connectedprofile.Encode(c)
	fixtureRoot, err := os.OpenRoot(vmFixtureRoot)
	if err != nil {
		t.Fatal("VM_FIXTURE_ROOT_FAILED")
	}
	defer fixtureRoot.Close()
	if publishNew(fixtureRoot, "binding.json", configBytes, 0644) != nil || publishNew(fixtureRoot, "admission", []byte(uuid), 0644) != nil {
		t.Fatal("VM_FIXTURE_BINDING_FAILED")
	}
	runVMChild(t, ctx, uuid, c, "seed")
	validation, _ := json.Marshal(connectedenroll.ValidationInput{ServerID: c.ServerID, ConnectorID: c.ConnectorID, UploaderUID: uu, UploaderGID: ug, SharedGID: sg})
	policy := connectedunits.EnrollmentInput{UploaderUID: uu, UploaderGID: ug, SharedGID: sg, ArtifactSHA256: digest, Addresses: addresses}
	invoke, err := connectedunits.RenderEnrollmentProperties(policy, "validate-enrollment")
	if err != nil {
		t.Fatal("VM_FIXTURE_RENDER_FAILED")
	}
	secret, err := runEnrollment(ctx, invoke.Properties, invoke.Executable, "validate-enrollment", validation, policy)
	defer clear(secret)
	if err != nil {
		t.Fatal("VM_FIXTURE_OFFLINE_ENROLLMENT_FAILED")
	}
	decoded, err := connectedcredential.DecodeCredential(secret, c.ServerID, c.ConnectorID)
	if err != nil || decoded.Secret != "hlc_"+strings.Repeat("x", 43) {
		t.Fatal("VM_FIXTURE_SECRET_VALIDATION_FAILED")
	}
	decoded.Secret = ""
	before, err := os.Stat(connectedprofile.StateDirectory + "/enrollment/ledger")
	if err != nil {
		t.Fatal("VM_FIXTURE_LEDGER_MISSING")
	}
	beforeDB, err := os.Stat(connectedprofile.StateDirectory + "/enrollment/ledger/upload.sqlite")
	if err != nil {
		t.Fatal("VM_FIXTURE_DATABASE_MISSING")
	}
	if promote(secret, uu, ug) != nil {
		t.Fatal("VM_FIXTURE_PROMOTION_FAILED")
	}
	clear(secret)
	after, err := os.Stat(connectedprofile.StateDirectory + "/ledger")
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("VM_FIXTURE_LEDGER_REPLACED")
	}
	afterDB, err := os.Stat(connectedprofile.StateDirectory + "/ledger/upload.sqlite")
	if err != nil || !os.SameFile(beforeDB, afterDB) {
		t.Fatal("VM_FIXTURE_DATABASE_REPLACED")
	}
	invoke, err = connectedunits.RenderEnrollmentProperties(policy, "validate-ledger")
	if err != nil {
		t.Fatal("VM_FIXTURE_RENDER_FAILED")
	}
	result, err := runEnrollment(ctx, invoke.Properties, invoke.Executable, "validate-ledger", validation, policy)
	if err != nil || string(result) != "VALID" {
		t.Fatal("VM_FIXTURE_OFFLINE_LEDGER_FAILED")
	}
	units, err := connectedunits.RenderUnits(c)
	if err != nil {
		t.Fatal("VM_FIXTURE_RENDER_FAILED")
	}
	unitRoot, err := os.OpenRoot("/etc/systemd/system")
	if err != nil {
		t.Fatal("VM_FIXTURE_UNIT_ROOT_FAILED")
	}
	defer unitRoot.Close()
	configRoot, err := os.OpenRoot(connectedprofile.ConfigDirectory)
	if err != nil {
		t.Fatal("VM_FIXTURE_CONFIG_ROOT_FAILED")
	}
	defer configRoot.Close()
	if publishNew(unitRoot, collectorUnit, units.Collector, 0644) != nil || publishNew(unitRoot, uploaderUnit, units.Uploader, 0644) != nil || publishNew(configRoot, "installed.json", configBytes, 0644) != nil {
		t.Fatal("VM_FIXTURE_PUBLICATION_FAILED")
	}
	if _, err := command(ctx, "/usr/bin/systemctl", []string{"daemon-reload"}, nil, 1024); err != nil {
		t.Fatal("VM_FIXTURE_RELOAD_FAILED")
	}
	assertVMUploaderStopped(t, ctx)
	// No steady uploader start, request or commit exists in this fixture.
	owned := map[string]workerInstance{}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		want, attempted := owned[collectorUnit]
		state, e := inspectWorker(cleanup, collectorUnit)
		if e != nil {
			t.Error("VM_FIXTURE_CLEANUP_IDENTITY_FAILED")
			return
		}
		if state.PID == 0 && (state.Active == "inactive" || state.Active == "failed") {
			assertVMUploaderStopped(t, cleanup)
			return
		}
		if !attempted || want.Invocation == "" || state.Invocation != want.Invocation || (want.PID != 0 && state.PID != want.PID) {
			t.Error("VM_FIXTURE_CLEANUP_FOREIGN_REFUSED")
			return
		}
		if service(cleanup, "stop", collectorUnit) != nil {
			t.Error("VM_FIXTURE_COLLECTOR_STOP_FAILED")
			return
		}
		state, e = inspectWorker(cleanup, collectorUnit)
		if e != nil || state.PID != 0 || state.Active != "inactive" {
			t.Error("VM_FIXTURE_COLLECTOR_JOIN_FAILED")
		}
		assertVMUploaderStopped(t, cleanup)
	}()
	if startActivationWorker(ctx, collectorUnit, owned) != nil {
		t.Fatal("VM_FIXTURE_COLLECTOR_START_FAILED")
	}
	collectorState := owned[collectorUnit]
	if !sameWorker(ctx, collectorUnit, collectorState) {
		t.Fatal("VM_FIXTURE_COLLECTOR_IDENTITY_FAILED")
	}
	collectCtx, end := context.WithTimeout(ctx, 20*time.Second)
	defer end()
	for {
		if !sameWorker(collectCtx, collectorUnit, collectorState) {
			t.Fatal("VM_FIXTURE_COLLECTOR_CHANGED")
		}
		record, e := readStatusRecord("collector-status", cu)
		if e == nil && record.InvocationID == collectorState.Invocation && record.State == "COLLECTING" {
			break
		}
		if activationWait(collectCtx) != nil {
			t.Fatal("VM_FIXTURE_COLLECTOR_NOT_READY")
		}
	}
	runVMChild(t, collectCtx, uuid, c, "read")
	if !sameWorker(collectCtx, collectorUnit, collectorState) {
		t.Fatal("VM_FIXTURE_COLLECTOR_CHANGED")
	}
	t.Log("VM_FIXTURE_STOPPED_OFFLINE_AND_COLLECTOR_OK")
}

func assertVMUploaderStopped(t *testing.T, ctx context.Context) {
	t.Helper()
	state, err := inspectWorker(ctx, uploaderUnit)
	if err != nil || state.Active != "inactive" || state.PID != 0 {
		t.Fatal("VM_FIXTURE_UPLOADER_NOT_STOPPED")
	}
	for _, path := range []string{activationParent, activationPath + "/request.json", activationPath + "/commit.json"} {
		if _, e := os.Lstat(path); !os.IsNotExist(e) {
			t.Fatal("VM_FIXTURE_ACTIVATION_PRESENT")
		}
	}
}

func runVMChild(t *testing.T, ctx context.Context, uuid string, c connectedprofile.Config, mode string) {
	t.Helper()
	if mode != "seed" && mode != "read" {
		t.Fatal("VM_FIXTURE_CHILD_MODE_REFUSED")
	}
	args := []string{"--reuid=" + strconv.FormatUint(uint64(c.UploaderUID), 10), "--regid=" + strconv.FormatUint(uint64(c.UploaderGID), 10), "--groups=" + strconv.FormatUint(uint64(c.SharedGID), 10), "--inh-caps=-all", "--ambient-caps=-all", "--bounding-set=-all", "--no-new-privs", vmFixtureRoot + "/fixture.test", "-test.run=^TestConnectedVMChild$"}
	child := exec.CommandContext(ctx, "/usr/bin/setpriv", args...)
	child.WaitDelay = time.Second
	child.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C", "HLO_DISPOSABLE_VM_UUID=" + uuid, "HLO_DISPOSABLE_VM_CONFIRM=observer-connected-f1-synthetic-only", "HLO_VM_CHILD=" + mode}
	output := &boundedOutput{limit: 2048}
	child.Stdout = output
	child.Stderr = output
	if child.Run() != nil {
		clear(output.data)
		t.Fatal("VM_FIXTURE_CHILD_FAILED")
	}
	clear(output.data)
}

func TestConnectedVMChild(t *testing.T) {
	mode := os.Getenv("HLO_VM_CHILD")
	if mode == "" {
		t.Skip("child only")
	}
	// Admission is established before the parent drops privilege; the child also
	// checks QEMU and the root-owned fixture binding before synthetic operations.
	if os.Getuid() == 0 || connectedprofile.HardenSecretProcess() != nil {
		t.Fatal("VM_FIXTURE_CHILD_IDENTITY_FAILED")
	}
	admission, err := connectedprofile.ReadRootFile(vmFixtureRoot+"/admission", 36)
	if err != nil || len(admission) != 36 || string(admission) != os.Getenv("HLO_DISPOSABLE_VM_UUID") || os.Getenv("HLO_DISPOSABLE_VM_CONFIRM") != "observer-connected-f1-synthetic-only" {
		t.Fatal("VM_FIXTURE_CHILD_ADMISSION_FAILED")
	}
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer probeCancel()
	virtual, err := command(probeCtx, "/usr/bin/systemd-detect-virt", []string{"--vm"}, nil, 64)
	if err != nil || (string(virtual) != "kvm\n" && string(virtual) != "qemu\n") {
		t.Fatal("VM_FIXTURE_CHILD_PLATFORM_REFUSED")
	}
	data, err := connectedprofile.ReadRootFile(vmFixtureRoot+"/binding.json", 4096)
	if err != nil {
		t.Fatal("VM_FIXTURE_CHILD_BINDING_FAILED")
	}
	c, err := connectedprofile.Decode(data)
	if err != nil || c.ServerID != vmFixtureServer || c.ConnectorID != vmFixtureConnector || connectedprofile.CheckIdentity(c, false) != nil {
		t.Fatal("VM_FIXTURE_CHILD_IDENTITY_FAILED")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if mode == "seed" {
		setup, err := enrollmentstore.OpenNew(ctx, connectedprofile.StateDirectory+"/enrollment")
		if err != nil {
			t.Fatal("VM_FIXTURE_SEED_OPEN_FAILED")
		}
		defer setup.Close()
		binding := uploadstate.Binding{ServerID: c.ServerID, ConnectorID: c.ConnectorID}
		secret := []byte("hlc_" + strings.Repeat("x", 43))
		defer clear(secret)
		if setup.Begin(ctx, binding) != nil || setup.Create(ctx, enrollmentcoord.Credential{Binding: binding, Secret: secret}) != nil || setup.Provision(ctx, binding) != nil || setup.MarkReady(ctx, binding) != nil || setup.Close() != nil {
			t.Fatal("VM_FIXTURE_SEED_FAILED")
		}
	} else if mode == "read" {
		r, err := sharedhandoff.OpenReader(connectedprofile.StateDirectory+"/handoff", sharedhandoff.Policy{CollectorUID: c.CollectorUID, UploaderUID: c.UploaderUID, SharedGID: c.SharedGID, ServerID: c.ServerID})
		if err != nil {
			t.Fatal("VM_FIXTURE_HANDOFF_OPEN_FAILED")
		}
		defer r.Close()
		body, err := r.Read(ctx, c.ServerID)
		if err != nil {
			t.Fatal("VM_FIXTURE_HANDOFF_READ_FAILED")
		}
		var doc struct {
			Sections struct {
				Overview json.RawMessage `json:"overview"`
			} `json:"sections"`
		}
		if json.Unmarshal(body, &doc) != nil || !bytes.Equal(doc.Sections.Overview, []byte(`{"available":false,"reason_code":"HOST_DATA_UNAVAILABLE"}`)) || r.Close() != nil {
			t.Fatal("VM_FIXTURE_COVERAGE_CLAIM_FAILED")
		}
	} else {
		t.Fatal("VM_FIXTURE_CHILD_MODE_REFUSED")
	}
}

// Diagnostic rerun keeps stdin empty: no synthetic credential can be returned.
func TestConnectedVMOfflineEmptyInput(t *testing.T) {
	vmAdmission(t, true)
	lock, err := acquireLease()
	if err != nil {
		t.Fatal("DIAGNOSTIC_LEASE_FAILED")
	}
	defer func() {
		if lock.Close() != nil {
			t.Error("DIAGNOSTIC_LEASE_CLOSE_FAILED")
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	data, err := connectedprofile.ReadRootFile(vmFixtureRoot+"/binding.json", 4096)
	if err != nil {
		t.Fatal("DIAGNOSTIC_BINDING_FAILED")
	}
	c, err := connectedprofile.Decode(data)
	if err != nil {
		t.Fatal("DIAGNOSTIC_BINDING_FAILED")
	}
	p := connectedunits.EnrollmentInput{UploaderUID: c.UploaderUID, UploaderGID: c.UploaderGID, SharedGID: c.SharedGID, ArtifactSHA256: c.ArtifactSHA256, Addresses: c.Addresses}
	inv, err := connectedunits.RenderEnrollmentProperties(p, "validate-enrollment")
	if err != nil {
		t.Fatal("DIAGNOSTIC_RENDER_FAILED")
	}
	state, err := enrollmentState(ctx)
	if err != nil || state["LoadState"] != "not-found" {
		t.Fatal("DIAGNOSTIC_EXISTING_REFUSED")
	}
	marker := "observer-connected-enrollment-fixture-empty-input"
	args := []string{"--quiet", "--collect", "--pipe", "--wait", "--service-type=exec", "--unit=" + enrollmentUnit, "--description=" + marker}
	for _, property := range inv.Properties {
		args = append(args, "--property="+property)
	}
	args = append(args, "--", inv.Executable, "validate-enrollment")
	child := exec.CommandContext(ctx, "/usr/bin/systemd-run", args...)
	child.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	child.WaitDelay = time.Second
	pipe, err := child.StdinPipe()
	if err != nil {
		t.Fatal("DIAGNOSTIC_PIPE_FAILED")
	}
	if child.Start() != nil {
		pipe.Close()
		t.Fatal("DIAGNOSTIC_START_FAILED")
	}
	initial, initialErr := enrollmentState(ctx)
	if initialErr != nil {
		t.Log("DIAGNOSTIC_INITIAL_READ_FAILED")
	} else {
		if initial["LoadState"] == "loaded" {
			t.Log("DIAGNOSTIC_INITIAL_LOADED")
		}
		if initial["Description"] == marker && initial["Transient"] == "yes" {
			t.Log("DIAGNOSTIC_INITIAL_MARKER_MATCH")
		}
		if initial["InvocationID"] == "" {
			t.Log("DIAGNOSTIC_INITIAL_INVOCATION_EMPTY")
		}
		for _, state := range []string{"inactive", "activating", "active", "failed"} {
			if initial["ActiveState"] == state {
				t.Log("DIAGNOSTIC_INITIAL_" + strings.ToUpper(state))
			}
		}
	}
	id, awaitErr := awaitEnrollment(ctx, marker)
	if awaitErr != nil {
		t.Error("DIAGNOSTIC_AWAIT_FAILED")
	} else {
		t.Log("DIAGNOSTIC_AWAIT_PASSED")
		time.Sleep(250 * time.Millisecond)
		current, e := enrollmentState(ctx)
		if e != nil || !ownedEnrollment(current, marker) || current["InvocationID"] != id || current["ActiveState"] != "active" {
			t.Error("DIAGNOSTIC_DWELL_FAILED")
		}
		if connectedpolicy.ValidateOffline(ctx, enrollmentUnit, connectedpolicy.OfflineExpectation{UploaderUID: c.UploaderUID, UploaderGID: c.UploaderGID, SharedGID: c.SharedGID, ArtifactSHA256: c.ArtifactSHA256, Mode: "validate-enrollment"}) != nil {
			t.Error("DIAGNOSTIC_POLICY_FAILED")
		} else {
			t.Log("DIAGNOSTIC_POLICY_PASSED")
		}
	}
	pipe.Close()
	cancel()
	_ = child.Wait()
	if stopEnrollment(marker, id) != nil {
		t.Fatal("DIAGNOSTIC_CLEANUP_FAILED")
	}
}

func TestConnectedVMProbeChild(t *testing.T) {
	if os.Getuid() == 0 {
		t.Fatal("PROBE_ROOT_REFUSED")
	}
	if connectedprofile.HardenSecretProcess() != nil {
		t.Fatal("PROBE_HARDEN_FAILED")
	}
	t.Log("PROBE_HARDEN_OK")
	if len(os.Args) < 4 {
		t.Fatal("PROBE_IDENTITY_FAILED")
	}
	u, ue := strconv.ParseUint(os.Args[len(os.Args)-3], 10, 32)
	g, ge := strconv.ParseUint(os.Args[len(os.Args)-2], 10, 32)
	s, se := strconv.ParseUint(os.Args[len(os.Args)-1], 10, 32)
	if ue != nil || ge != nil || se != nil || connectedprofile.CheckIdentity(connectedprofile.Config{UploaderUID: uint32(u), UploaderGID: uint32(g), SharedGID: uint32(s)}, false) != nil {
		t.Fatal("PROBE_IDENTITY_FAILED")
	}
	t.Log("PROBE_IDENTITY_OK")
	if connectedprofile.CheckEnvironment(os.Environ()) != nil {
		t.Error("PROBE_ENV_FAILED")
		unknown := 0
		for _, entry := range os.Environ() {
			key, _, _ := strings.Cut(entry, "=")
			switch key {
			case "GODEBUG", "PATH", "USER", "LOGNAME", "HOME", "SHELL", "LANG", "LC_ALL", "INVOCATION_ID", "SYSTEMD_EXEC_PID", "CREDENTIALS_DIRECTORY":
			case "MEMORY_PRESSURE_WATCH":
				t.Log("PROBE_MEMORY_WATCH_PRESENT")
			case "MEMORY_PRESSURE_WRITE":
				t.Log("PROBE_MEMORY_WRITE_PRESENT")
			case "LANGUAGE":
				t.Log("PROBE_LANGUAGE_PRESENT")
			default:
				unknown++
			}
		}
		if unknown > 0 {
			t.Log("PROBE_OTHER_ENV_PRESENT")
		}
	} else {
		t.Log("PROBE_ENV_OK")
	}
}

func TestConnectedVMPreInputProbe(t *testing.T) {
	vmAdmission(t, true)
	lock, err := acquireLease()
	if err != nil {
		t.Fatal("PROBE_LEASE_FAILED")
	}
	defer lock.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	data, err := connectedprofile.ReadRootFile(vmFixtureRoot+"/binding.json", 4096)
	if err != nil {
		t.Fatal("PROBE_BINDING_FAILED")
	}
	c, err := connectedprofile.Decode(data)
	if err != nil {
		t.Fatal("PROBE_BINDING_FAILED")
	}
	p := connectedunits.EnrollmentInput{UploaderUID: c.UploaderUID, UploaderGID: c.UploaderGID, SharedGID: c.SharedGID, ArtifactSHA256: c.ArtifactSHA256, Addresses: c.Addresses}
	inv, err := connectedunits.RenderEnrollmentProperties(p, "validate-enrollment")
	if err != nil {
		t.Fatal("PROBE_RENDER_FAILED")
	}
	const unit = enrollmentUnit
	const marker = "observer-connected-enrollment-preinput-probe"
	state, err := command(ctx, "/usr/bin/systemctl", []string{"show", "--property=LoadState", "--value", unit}, nil, 64)
	if err != nil || string(state) != "not-found\n" {
		t.Fatal("PROBE_EXISTING_REFUSED")
	}
	const probeRoot = vmFixtureRoot + "/probe-root-v3"
	if mkdirNew(probeRoot, 0, 0, 0755) != nil || mkdirNew(probeRoot+"/bin", 0, 0, 0755) != nil {
		t.Fatal("PROBE_ROOT_CREATE_FAILED")
	}
	directory, err := rootDirectory(probeRoot + "/bin")
	if err != nil {
		t.Fatal("PROBE_ROOT_FAILED")
	}
	directory.Close()
	root, err := os.OpenRoot(probeRoot + "/bin")
	if err != nil {
		t.Fatal("PROBE_ROOT_FAILED")
	}
	defer root.Close()
	if publishNew(root, "fixture-probe", []byte("probe"), 0555) != nil {
		t.Fatal("PROBE_PLACEHOLDER_FAILED")
	}
	state, err = command(ctx, "/usr/bin/systemctl", []string{"show", "--property=LoadState", "--value", unit}, nil, 64)
	if err != nil || string(state) != "not-found\n" {
		t.Fatal("PROBE_EXISTING_REFUSED")
	}
	args := []string{"--quiet", "--collect", "--pipe", "--wait", "--service-type=exec", "--unit=" + unit, "--description=" + marker}
	for _, property := range inv.Properties {
		key, _, _ := strings.Cut(property, "=")
		switch key {
		case "RootDirectory":
			property = "RootDirectory=" + probeRoot
		case "ExecPaths":
			property = "ExecPaths=/bin/fixture-probe"
		case "BindReadOnlyPaths":
			property = "BindReadOnlyPaths=" + vmFixtureRoot + "/fixture.test:/bin/fixture-probe"
		case "BindPaths":
			continue
		case "ReadWritePaths":
			continue
		}
		args = append(args, "--property="+property)
	}
	args = append(args, "--", "/bin/fixture-probe", "-test.run=^TestConnectedVMProbeChild$", "-test.v", "--", strconv.FormatUint(uint64(c.UploaderUID), 10), strconv.FormatUint(uint64(c.UploaderGID), 10), strconv.FormatUint(uint64(c.SharedGID), 10))
	child := exec.CommandContext(ctx, "/usr/bin/systemd-run", args...)
	child.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	child.WaitDelay = time.Second
	output := &boundedOutput{limit: 2048}
	child.Stdout = output
	stderr := &boundedOutput{limit: 2048}
	child.Stderr = stderr
	err = child.Run()
	// Only report closed tokens, never arbitrary test panic/runtime output.
	for _, code := range []string{"PROBE_ROOT_REFUSED", "PROBE_HARDEN_FAILED", "PROBE_HARDEN_OK", "PROBE_IDENTITY_FAILED", "PROBE_IDENTITY_OK", "PROBE_ENV_FAILED", "PROBE_MEMORY_WATCH_PRESENT", "PROBE_MEMORY_WRITE_PRESENT", "PROBE_LANGUAGE_PRESENT", "PROBE_OTHER_ENV_PRESENT", "PROBE_ENV_OK"} {
		if bytes.Contains(output.data, []byte(code)) {
			t.Log(code)
		}
	}
	clear(output.data)
	if err != nil {
		t.Error("PROBE_PROCESS_FAILED")
	}
	clear(stderr.data)
	if stopEnrollment(marker, "") != nil {
		t.Error("PROBE_CLEANUP_FAILED")
	}
	state, e := command(context.Background(), "/usr/bin/systemctl", []string{"show", "--property=MainPID", "--value", unit}, nil, 64)
	if e != nil || string(state) != "0\n" {
		t.Error("PROBE_NOT_JOINED")
	}
}
