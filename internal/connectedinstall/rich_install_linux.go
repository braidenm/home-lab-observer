//go:build linux

package connectedinstall

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/user"
	"strconv"

	"github.com/braidenm/home-lab-observer/internal/connectedbundle"
	"github.com/braidenm/home-lab-observer/internal/connectedcredential"
	"github.com/braidenm/home-lab-observer/internal/connectedenroll"
	"github.com/braidenm/home-lab-observer/internal/connectedinstalllease"
	"github.com/braidenm/home-lab-observer/internal/connectedpolicy"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
)

func CheckRichProcessesRequest(ctx context.Context, bundle, digest, server string) error {
	if ctx == nil || ctx.Err() != nil || os.Getuid() != 0 || os.Geteuid() != 0 || !connectedprofile.ValidServerID(server) {
		return ErrUnsafe
	}
	if _, err := connectedbundle.VerifyRichProcessesDirectory(bundle, digest); err != nil {
		return ErrUnsafe
	}
	if preflight(ctx) != nil || absentRichTargets(ctx) != nil {
		return ErrUnsafe
	}
	addresses, err := connectedpolicy.Resolve(ctx)
	if err != nil || connectedpolicy.ValidateParent(ctx, addresses) != nil {
		return ErrUnsafe
	}
	return nil
}

// InstallRichProcesses is a new, separately enrolled VM proof installation.
// It never adopts numeric state, enables services or changes collection policy.
func InstallRichProcesses(ctx context.Context, request Request) (result error) {
	defer clear(request.Grant)
	phase := "PREFLIGHT_REFUSED"
	defer func() {
		if result != nil {
			result = &PhaseError{Phase: phase}
		}
	}()
	if ctx == nil || ctx.Err() != nil || !connectedprofile.ValidServerID(request.ServerID) || len(request.Grant) != 47 || !grantPattern.Match(request.Grant) {
		return ErrUnsafe
	}
	if _, err := connectedbundle.VerifyRichProcessesDirectory(request.BundleDirectory, request.ManifestSHA256); err != nil {
		return ErrUnsafe
	}
	lease, err := connectedinstalllease.Acquire()
	if err != nil {
		return err
	}
	defer lease.Close()
	if _, err := connectedbundle.VerifyRichProcessesDirectory(request.BundleDirectory, request.ManifestSHA256); err != nil {
		return ErrUnsafe
	}
	if preflight(ctx) != nil || absentRichTargets(ctx) != nil {
		return ErrUnsafe
	}
	addresses, err := connectedpolicy.Resolve(ctx)
	if err != nil || connectedpolicy.ValidateParent(ctx, addresses) != nil {
		return ErrUnsafe
	}
	phase = "LOCAL_SETUP_INCOMPLETE"
	layout := richInstallLayout{installLayout{digest: request.ManifestSHA256}}
	if layout.createDirectory(richConfigRoot) != nil {
		return ErrRecovery
	}
	preparing, _ := richPreparing(request.ServerID, request.ManifestSHA256)
	if layout.publishFile(richPreparingFile, preparing) != nil || layout.createDirectory(richStateRoot) != nil {
		return ErrRecovery
	}
	for _, role := range []directoryRole{dirOptRoot, dirReleases} {
		if layout.createDirectory(role) != nil {
			return ErrRecovery
		}
	}
	ids, err := createRichPrincipals(ctx)
	if err != nil {
		return ErrRecovery
	}
	layout.collectorUID, layout.uploaderUID, layout.uploaderGID, layout.sharedGID = ids.collectorUID, ids.uploaderUID, ids.uploaderGID, ids.sharedGID
	// No UID-owned enrollment state or one-use grant is released before exact
	// local/NSS identity admission. The placeholder connector is only a private
	// operational binding for principal checks; it is never persisted/enrolled.
	allocated := connectedprofile.Config{Version: connectedprofile.Version, State: "INSTALLED_READY", ServerID: request.ServerID, ConnectorID: "agent_00000000000000000000000000000000", CollectorUID: ids.collectorUID, UploaderUID: ids.uploaderUID, UploaderGID: ids.uploaderGID, SharedGID: ids.sharedGID, ArtifactSHA256: layout.digest, PolicyGeneration: 1, Addresses: addresses}
	if auditPrincipalsForPair(ctx, allocated, richProcessesPair) != nil {
		return ErrRecovery
	}
	if layout.createDirectory(dirRelease) != nil {
		return ErrRecovery
	}
	destination, err := layout.path(dirRelease)
	if err != nil || copyBundleFiles(request.BundleDirectory, destination, layout.digest, connectedbundle.RichProcessesNames(), connectedbundle.VerifyRichProcessesDirectory) != nil {
		return ErrRecovery
	}
	if prepareRootFiles(layout, addresses) != nil {
		return ErrRecovery
	}
	input := connectedunits.EnrollmentInput{UploaderUID: ids.uploaderUID, UploaderGID: ids.uploaderGID, SharedGID: ids.sharedGID, ArtifactSHA256: layout.digest, Addresses: addresses}
	grant, _ := json.Marshal(connectedenroll.RichGrantInput{ServerID: request.ServerID, Grant: string(request.Grant), CollectorUID: ids.collectorUID, UploaderUID: ids.uploaderUID, UploaderGID: ids.uploaderGID, SharedGID: ids.sharedGID, DefinitionSHA256: connectedprofile.RichProcessesDefinitionSHA256})
	clear(request.Grant)
	phase = "ENROLLMENT_RECOVERY_REQUIRED"
	out, err := runRichEnrollment(ctx, input, connectedunits.Enroll, grant)
	clear(grant)
	if err != nil {
		if err == ErrUnsafe {
			phase = "LOCAL_SETUP_INCOMPLETE"
		}
		return ErrRecovery
	}
	var binding connectedenroll.Binding
	if json.Unmarshal(out, &binding) != nil || binding.ServerID != request.ServerID || !connectedprofile.ValidConnectorID(binding.ConnectorID) {
		return ErrRecovery
	}
	canonical, _ := json.Marshal(binding)
	if !bytes.Equal(out, canonical) {
		return ErrRecovery
	}
	policy, err := connectedprofile.NewRichProcessesPolicy(binding.ServerID, binding.ConnectorID, ids.collectorUID, ids.uploaderUID, ids.uploaderGID, ids.sharedGID)
	if err != nil {
		return ErrRecovery
	}
	digest, err := connectedprofile.DigestRichProcessesPolicy(policy)
	if err != nil {
		return ErrRecovery
	}
	c := connectedprofile.RichProcessesConfig{Version: connectedprofile.RichProcessesVersion, State: "INSTALLED_READY", Profile: connectedprofile.RichProcessesProfile, ServerID: binding.ServerID, ConnectorID: binding.ConnectorID, CollectorUID: ids.collectorUID, UploaderUID: ids.uploaderUID, UploaderGID: ids.uploaderGID, SharedGID: ids.sharedGID, DefinitionSHA256: connectedprofile.RichProcessesDefinitionSHA256, PolicySHA256: hex.EncodeToString(digest[:]), ArtifactSHA256: layout.digest, PolicyGeneration: 1, Addresses: addresses}
	if connectedprofile.VerifyRichProcessesBinding(c, policy) != nil || auditPrincipalsForPair(ctx, richWorkerBinding(c), richProcessesPair) != nil {
		return ErrRecovery
	}
	validation, _ := json.Marshal(connectedenroll.RichValidationInput{ServerID: c.ServerID, ConnectorID: c.ConnectorID, CollectorUID: c.CollectorUID, UploaderUID: c.UploaderUID, UploaderGID: c.UploaderGID, SharedGID: c.SharedGID, DefinitionSHA256: c.DefinitionSHA256})
	secret, err := runRichEnrollment(ctx, input, connectedunits.ValidateEnrollment, validation)
	defer clear(secret)
	if err != nil {
		return ErrRecovery
	}
	if _, err := connectedcredential.DecodeCredential(secret, c.ServerID, c.ConnectorID); err != nil {
		return ErrRecovery
	}
	if promoteFiles(layout, connectedprofile.RichProcessesStateDirectory, c.UploaderUID, c.UploaderGID, secret) != nil {
		return ErrRecovery
	}
	check, err := runRichEnrollment(ctx, input, connectedunits.ValidateLedger, validation)
	if err != nil || string(check) != "VALID" {
		return ErrRecovery
	}
	units, err := connectedunits.RenderRichProcessesUnits(c)
	if err != nil {
		return ErrRecovery
	}
	if layout.publishFile(fileCollectorUnit, units.Collector) != nil || layout.publishFile(fileUploaderUnit, units.Uploader) != nil {
		return ErrRecovery
	}
	if _, err := runTool(ctx, toolSystemctl, []string{"daemon-reload"}, nil, 1024); err != nil {
		return ErrRecovery
	}
	if connectedpolicy.ValidateEffective(ctx, connectedunits.RichProcessesUploaderUnit, c.Addresses) != nil {
		return ErrRecovery
	}
	for _, unit := range []string{connectedunits.RichProcessesCollectorUnit, connectedunits.RichProcessesUploaderUnit} {
		state, err := unitRuntimeState(ctx, unit)
		if err != nil || !stoppedDisabledUnit(state, unit) {
			return ErrRecovery
		}
	}
	policyBytes, _ := connectedprofile.EncodeRichProcessesPolicy(policy)
	configBytes, _ := connectedprofile.EncodeRichProcessesConfig(c)
	if layout.publishFile(richPolicyFile, policyBytes) != nil || layout.publishFile(fileInstalledConfig, configBytes) != nil {
		return ErrRecovery
	}
	return nil
}

func runRichEnrollment(ctx context.Context, policy connectedunits.EnrollmentInput, mode connectedunits.EnrollmentMode, input []byte) ([]byte, error) {
	invocation, err := connectedunits.RenderRichProcessesEnrollmentProperties(connectedunits.RichProcessesEnrollmentInput(policy), mode)
	if err != nil {
		return nil, ErrUnsafe
	}
	return runEnrollmentForPair(ctx, invocation.Properties, invocation.Executable, string(mode), input, policy, func(ctx context.Context) (map[string]string, error) {
		return enrollmentStateForPair(ctx, richProcessesPair)
	}, richProcessesPair)
}

func richPreparing(server, digest string) ([]byte, error) {
	return json.Marshal(struct {
		Version string `json:"version"`
		State   string `json:"state"`
		Server  string `json:"server_id"`
		Digest  string `json:"manifest_sha256"`
	}{"observer-connected-rich-preparing/v1", "PREPARING", server, digest})
}

// This private adapter carries only the common operational worker binding into
// existing principal/startup algorithms. It is never persisted or used as a
// numeric collection approval. Rich policy is independently verified by callers.
func richWorkerBinding(c connectedprofile.RichProcessesConfig) connectedprofile.Config {
	return connectedprofile.Config{Version: connectedprofile.Version, State: c.State, ServerID: c.ServerID, ConnectorID: c.ConnectorID, CollectorUID: c.CollectorUID, UploaderUID: c.UploaderUID, UploaderGID: c.UploaderGID, SharedGID: c.SharedGID, ArtifactSHA256: c.ArtifactSHA256, PolicyGeneration: c.PolicyGeneration, Addresses: c.Addresses}
}

func absentRichTargets(ctx context.Context) error {
	for _, path := range []string{connectedprofile.RichProcessesConfigDirectory, connectedprofile.RichProcessesStateDirectory, "/opt/home-lab-observer-connected-rich", "/var/lib/home-lab-observer-connected-rich-retired", "/run/home-lab-observer-connected-rich"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return ErrUnsafe
		}
	}
	collector, uploader, enrollment, _ := richProcessesPair.units()
	for _, unit := range []string{collector, uploader, enrollment} {
		for _, suffix := range []string{"", ".d"} {
			if _, err := os.Lstat("/etc/systemd/system/" + unit + suffix); !os.IsNotExist(err) {
				return ErrUnsafe
			}
		}
		state, err := unitRuntimeState(ctx, unit)
		if err != nil || state["LoadState"] != "not-found" {
			return ErrUnsafe
		}
	}
	for _, name := range []string{connectedprofile.RichProcessesCollectorName, connectedprofile.RichProcessesUploaderName} {
		if _, err := user.Lookup(name); !missingUser(err) {
			return ErrUnsafe
		}
	}
	for _, name := range []string{connectedprofile.RichProcessesSharedName, connectedprofile.RichProcessesUploaderName} {
		if _, err := user.LookupGroup(name); !missingGroup(err) {
			return ErrUnsafe
		}
	}
	return nil
}

func createRichPrincipals(ctx context.Context) (installLayout, error) {
	collector, uploader, shared, _ := richProcessesPair.principalNames()
	for _, name := range []string{shared, uploader} {
		if _, err := runTool(ctx, toolGroupadd, []string{"--system", name}, nil, 1024); err != nil {
			return installLayout{}, ErrRecovery
		}
	}
	if _, err := runTool(ctx, toolUseradd, []string{"--system", "--no-create-home", "--home-dir", "/nonexistent", "--shell", "/usr/sbin/nologin", "--gid", shared, collector}, nil, 1024); err != nil {
		return installLayout{}, ErrRecovery
	}
	if _, err := runTool(ctx, toolUseradd, []string{"--system", "--no-create-home", "--home-dir", "/nonexistent", "--shell", "/usr/sbin/nologin", "--gid", uploader, "--groups", shared, uploader}, nil, 1024); err != nil {
		return installLayout{}, ErrRecovery
	}
	cu, e1 := user.Lookup(collector)
	uu, e2 := user.Lookup(uploader)
	sg, e3 := user.LookupGroup(shared)
	if e1 != nil || e2 != nil || e3 != nil {
		return installLayout{}, ErrRecovery
	}
	parse := func(value string) (uint32, error) {
		id, err := strconv.ParseUint(value, 10, 32)
		return uint32(id), err
	}
	cid, e1 := parse(cu.Uid)
	uid, e2 := parse(uu.Uid)
	gid, e3 := parse(uu.Gid)
	sharedID, e4 := parse(sg.Gid)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		return installLayout{}, ErrRecovery
	}
	if _, err := connectedprofile.NewRichProcessesPolicy("srv_00000000000000000000000000000000", "agent_00000000000000000000000000000000", cid, uid, gid, sharedID); err != nil {
		return installLayout{}, ErrRecovery
	}
	return installLayout{collectorUID: cid, uploaderUID: uid, uploaderGID: gid, sharedGID: sharedID}, nil
}
