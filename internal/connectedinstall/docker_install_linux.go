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

func CheckRichProcessesDockerRequest(ctx context.Context, bundle, digest, server string) error {
	if ctx == nil || ctx.Err() != nil || os.Getuid() != 0 || os.Geteuid() != 0 || !connectedprofile.ValidServerID(server) {
		return ErrUnsafe
	}
	if _, err := connectedbundle.VerifyRichProcessesDockerDirectory(bundle, digest); err != nil {
		return ErrUnsafe
	}
	if preflight(ctx) != nil || absentDockerTargets(ctx) != nil {
		return ErrUnsafe
	}
	socket, err := discoverDockerSocket()
	if err != nil {
		return ErrUnsafe
	}
	defer socket.Close()
	addresses, err := connectedpolicy.Resolve(ctx)
	if err != nil || connectedpolicy.ValidateParent(ctx, addresses) != nil {
		return ErrUnsafe
	}
	return nil
}

// InstallRichProcessesDocker is a new, separately enrolled VM proof installation.
// It never adopts numeric state, enables services or changes collection policy.
func InstallRichProcessesDocker(ctx context.Context, request Request) (result error) {
	defer clear(request.Grant)
	phase := "PREFLIGHT_REFUSED"
	defer func() {
		if result != nil {
			result = &PhaseError{Phase: phase}
		}
	}()
	if ctx == nil || ctx.Err() != nil || os.Getuid() != 0 || os.Geteuid() != 0 || !connectedprofile.ValidServerID(request.ServerID) || len(request.Grant) != 47 || !grantPattern.Match(request.Grant) {
		return ErrUnsafe
	}
	if _, err := connectedbundle.VerifyRichProcessesDockerDirectory(request.BundleDirectory, request.ManifestSHA256); err != nil {
		return ErrUnsafe
	}
	lease, err := connectedinstalllease.Acquire()
	if err != nil {
		return err
	}
	defer lease.Close()
	if _, err := connectedbundle.VerifyRichProcessesDockerDirectory(request.BundleDirectory, request.ManifestSHA256); err != nil {
		return ErrUnsafe
	}
	if preflight(ctx) != nil || absentDockerTargets(ctx) != nil {
		return ErrUnsafe
	}
	socket, err := discoverDockerSocket()
	if err != nil {
		return ErrUnsafe
	}
	defer socket.Close()
	addresses, err := connectedpolicy.Resolve(ctx)
	if err != nil || connectedpolicy.ValidateParent(ctx, addresses) != nil {
		return ErrUnsafe
	}
	phase = "LOCAL_SETUP_INCOMPLETE"
	layout := dockerInstallLayout{richInstallLayout: richInstallLayout{installLayout{digest: request.ManifestSHA256}}}
	if layout.createDirectory(richConfigRoot) != nil {
		return ErrRecovery
	}
	preparing, _ := dockerPreparing(request.ServerID, request.ManifestSHA256)
	if layout.publishFile(richPreparingFile, preparing) != nil || layout.createDirectory(richStateRoot) != nil {
		return ErrRecovery
	}
	for _, role := range []directoryRole{dirOptRoot, dirReleases} {
		if layout.createDirectory(role) != nil {
			return ErrRecovery
		}
	}
	ids, err := createDockerPrincipals(ctx, socket.gid)
	if err != nil {
		return ErrRecovery
	}
	layout.collectorUID, layout.uploaderUID, layout.uploaderGID, layout.sharedGID = ids.collectorUID, ids.uploaderUID, ids.uploaderGID, ids.sharedGID
	layout.readerUID, layout.frameGID, layout.socketGID = ids.readerUID, ids.frameGID, ids.socketGID
	// No UID-owned enrollment state or one-use grant is released before exact
	// local/NSS identity admission. The placeholder connector is only a private
	// operational binding for principal checks; it is never persisted/enrolled.
	allocated, _, err := dockerConfigForPrincipals(request.ServerID, "agent_00000000000000000000000000000000", layout, addresses)
	if err != nil {
		return ErrRecovery
	}
	if auditDockerPrincipals(ctx, allocated) != nil {
		return ErrRecovery
	}
	if layout.createDirectory(dirRelease) != nil {
		return ErrRecovery
	}
	destination, err := layout.path(dirRelease)
	if err != nil || copyBundleFiles(request.BundleDirectory, destination, layout.digest, connectedbundle.RichProcessesDockerNames(), connectedbundle.VerifyRichProcessesDockerDirectory) != nil {
		return ErrRecovery
	}
	if prepareDockerRootFiles(layout, addresses) != nil {
		return ErrRecovery
	}
	input := connectedunits.EnrollmentInput{UploaderUID: ids.uploaderUID, UploaderGID: ids.uploaderGID, SharedGID: ids.sharedGID, ArtifactSHA256: layout.digest, Addresses: addresses}
	if !sameDockerSocket(socket) {
		return ErrRecovery
	}
	grant, _ := json.Marshal(connectedenroll.RichDockerGrantInput{ServerID: request.ServerID, Grant: string(request.Grant), CollectorUID: ids.collectorUID, UploaderUID: ids.uploaderUID, UploaderGID: ids.uploaderGID, SharedGID: ids.sharedGID, ReaderUID: ids.readerUID, FrameGID: ids.frameGID, SocketGID: ids.socketGID, DefinitionSHA256: connectedprofile.RichProcessesDockerDefinitionSHA256})
	clear(request.Grant)
	phase = "ENROLLMENT_RECOVERY_REQUIRED"
	out, err := runDockerEnrollment(ctx, input, connectedunits.Enroll, grant)
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
	policy, err := connectedprofile.NewRichProcessesDockerPolicy(binding.ServerID, binding.ConnectorID, ids.collectorUID, ids.uploaderUID, ids.uploaderGID, ids.sharedGID, ids.readerUID, ids.frameGID, ids.socketGID)
	if err != nil {
		return ErrRecovery
	}
	digest, err := connectedprofile.DigestRichProcessesDockerPolicy(policy)
	if err != nil {
		return ErrRecovery
	}
	c := connectedprofile.RichProcessesDockerConfig{Version: connectedprofile.RichProcessesDockerVersion, State: "INSTALLED_READY", Profile: connectedprofile.RichProcessesDockerProfile, ServerID: binding.ServerID, ConnectorID: binding.ConnectorID, CollectorUID: ids.collectorUID, UploaderUID: ids.uploaderUID, UploaderGID: ids.uploaderGID, SharedGID: ids.sharedGID, ReaderUID: ids.readerUID, FrameGID: ids.frameGID, SocketGID: ids.socketGID, DefinitionSHA256: connectedprofile.RichProcessesDockerDefinitionSHA256, PolicySHA256: hex.EncodeToString(digest[:]), ArtifactSHA256: layout.digest, PolicyGeneration: 1, Addresses: addresses}
	if connectedprofile.VerifyRichProcessesDockerBinding(c, policy) != nil || auditDockerPrincipals(ctx, c) != nil {
		return ErrRecovery
	}
	validation, _ := json.Marshal(connectedenroll.RichDockerValidationInput{ServerID: c.ServerID, ConnectorID: c.ConnectorID, CollectorUID: c.CollectorUID, UploaderUID: c.UploaderUID, UploaderGID: c.UploaderGID, SharedGID: c.SharedGID, ReaderUID: c.ReaderUID, FrameGID: c.FrameGID, SocketGID: c.SocketGID, DefinitionSHA256: c.DefinitionSHA256})
	secret, err := runDockerEnrollment(ctx, input, connectedunits.ValidateEnrollment, validation)
	defer clear(secret)
	if err != nil {
		return ErrRecovery
	}
	if _, err := connectedcredential.DecodeCredential(secret, c.ServerID, c.ConnectorID); err != nil {
		return ErrRecovery
	}
	if promoteFiles(layout, connectedprofile.RichProcessesDockerStateDirectory, c.UploaderUID, c.UploaderGID, secret) != nil {
		return ErrRecovery
	}
	check, err := runDockerEnrollment(ctx, input, connectedunits.ValidateLedger, validation)
	if err != nil || string(check) != "VALID" {
		return ErrRecovery
	}
	units, err := connectedunits.RenderRichProcessesDockerUnits(c)
	if err != nil {
		return ErrRecovery
	}
	if layout.publishFile(fileCollectorUnit, units.Collector) != nil || layout.publishFile(fileUploaderUnit, units.Uploader) != nil || layout.publishFile(fileDockerReaderUnit, units.Reader) != nil {
		return ErrRecovery
	}
	if _, err := runTool(ctx, toolSystemctl, []string{"daemon-reload"}, nil, 1024); err != nil {
		return ErrRecovery
	}
	if connectedpolicy.ValidateEffective(ctx, connectedunits.RichProcessesDockerUploaderUnit, c.Addresses) != nil {
		return ErrRecovery
	}
	for _, unit := range []string{connectedunits.RichProcessesDockerCollectorUnit, connectedunits.RichProcessesDockerReaderUnit, connectedunits.RichProcessesDockerUploaderUnit} {
		state, err := unitRuntimeState(ctx, unit)
		if err != nil || !stoppedDisabledUnit(state, unit) {
			return ErrRecovery
		}
	}
	policyBytes, _ := connectedprofile.EncodeRichProcessesDockerPolicy(policy)
	configBytes, _ := connectedprofile.EncodeRichProcessesDockerConfig(c)
	if !sameDockerSocket(socket) {
		return ErrRecovery
	}
	if layout.publishFile(richPolicyFile, policyBytes) != nil || layout.publishFile(fileInstalledConfig, configBytes) != nil {
		return ErrRecovery
	}
	return nil
}

func runDockerEnrollment(ctx context.Context, policy connectedunits.EnrollmentInput, mode connectedunits.EnrollmentMode, input []byte) ([]byte, error) {
	invocation, err := connectedunits.RenderRichProcessesDockerEnrollmentProperties(connectedunits.RichProcessesDockerEnrollmentInput(policy), mode)
	if err != nil {
		return nil, ErrUnsafe
	}
	return runEnrollmentForPair(ctx, invocation.Properties, invocation.Executable, string(mode), input, policy, func(ctx context.Context) (map[string]string, error) {
		return enrollmentStateForPair(ctx, richProcessesDockerPair)
	}, richProcessesDockerPair)
}

func dockerPreparing(server, digest string) ([]byte, error) {
	return json.Marshal(struct {
		Version string `json:"version"`
		State   string `json:"state"`
		Server  string `json:"server_id"`
		Digest  string `json:"manifest_sha256"`
	}{"observer-connected-rich-docker-preparing/v1", "PREPARING", server, digest})
}

func dockerConfigForPrincipals(server, connector string, l dockerInstallLayout, addresses []string) (connectedprofile.RichProcessesDockerConfig, connectedprofile.RichProcessesDockerPolicy, error) {
	p, err := connectedprofile.NewRichProcessesDockerPolicy(server, connector, l.collectorUID, l.uploaderUID, l.uploaderGID, l.sharedGID, l.readerUID, l.frameGID, l.socketGID)
	if err != nil {
		return connectedprofile.RichProcessesDockerConfig{}, p, ErrUnsafe
	}
	d, err := connectedprofile.DigestRichProcessesDockerPolicy(p)
	if err != nil {
		return connectedprofile.RichProcessesDockerConfig{}, p, ErrUnsafe
	}
	c := connectedprofile.RichProcessesDockerConfig{Version: connectedprofile.RichProcessesDockerVersion, State: "INSTALLED_READY", Profile: p.Profile, ServerID: server, ConnectorID: connector, CollectorUID: l.collectorUID, UploaderUID: l.uploaderUID, UploaderGID: l.uploaderGID, SharedGID: l.sharedGID, ReaderUID: l.readerUID, FrameGID: l.frameGID, SocketGID: l.socketGID, DefinitionSHA256: p.DefinitionSHA256, PolicySHA256: hex.EncodeToString(d[:]), ArtifactSHA256: l.digest, PolicyGeneration: 1, Addresses: addresses}
	if connectedprofile.VerifyRichProcessesDockerBinding(c, p) != nil {
		return connectedprofile.RichProcessesDockerConfig{}, p, ErrUnsafe
	}
	return c, p, nil
}

func absentDockerTargets(ctx context.Context) error {
	// Same rich roots make old process installation an explicit pre-mutation
	// refusal, not a migration or a second rich installation.
	if absentRichTargets(ctx) != nil {
		return ErrUnsafe
	}
	c, u, e, _ := richProcessesDockerPair.units()
	for _, unit := range []string{c, u, e, connectedunits.RichProcessesDockerReaderUnit} {
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
	for _, name := range []string{connectedprofile.RichProcessesDockerCollectorName, connectedprofile.RichProcessesDockerUploaderName, connectedprofile.RichProcessesDockerReaderName} {
		if _, err := user.Lookup(name); !missingUser(err) {
			return ErrUnsafe
		}
	}
	for _, name := range []string{connectedprofile.RichProcessesDockerUploaderName, connectedprofile.RichProcessesDockerSharedName, connectedprofile.RichProcessesDockerFrameName, connectedprofile.RichProcessesDockerCollectorName, connectedprofile.RichProcessesDockerReaderName} {
		if _, err := user.LookupGroup(name); !missingGroup(err) {
			return ErrUnsafe
		}
	}
	return nil
}

func createDockerPrincipals(ctx context.Context, socketGID uint32) (dockerInstallLayout, error) {
	collector, uploader, shared, _ := richProcessesDockerPair.principalNames()
	reader, frame := connectedprofile.RichProcessesDockerReaderName, connectedprofile.RichProcessesDockerFrameName
	for _, name := range []string{shared, uploader, frame} {
		if _, err := runTool(ctx, toolGroupadd, []string{"--system", name}, nil, 1024); err != nil {
			return dockerInstallLayout{}, ErrRecovery
		}
	}
	for _, p := range []struct {
		name, group   string
		supplementary []string
	}{{collector, shared, nil}, {uploader, uploader, []string{shared}}, {reader, frame, nil}} {
		args := []string{"--system", "--no-create-home", "--home-dir", "/nonexistent", "--shell", "/usr/sbin/nologin", "--gid", p.group}
		if len(p.supplementary) > 0 {
			args = append(args, "--groups", p.supplementary[0])
		}
		args = append(args, p.name)
		if _, err := runTool(ctx, toolUseradd, args, nil, 1024); err != nil {
			return dockerInstallLayout{}, ErrRecovery
		}
	}
	cu, e1 := user.Lookup(collector)
	uu, e2 := user.Lookup(uploader)
	ru, e3 := user.Lookup(reader)
	sg, e4 := user.LookupGroup(shared)
	fg, e5 := user.LookupGroup(frame)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil {
		return dockerInstallLayout{}, ErrRecovery
	}
	parse := func(v string) (uint32, error) { n, err := strconv.ParseUint(v, 10, 32); return uint32(n), err }
	cid, e1 := parse(cu.Uid)
	uid, e2 := parse(uu.Uid)
	ug, e3 := parse(uu.Gid)
	sharedGID, e4 := parse(sg.Gid)
	rid, e5 := parse(ru.Uid)
	frameGID, e6 := parse(fg.Gid)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || e6 != nil {
		return dockerInstallLayout{}, ErrRecovery
	}
	if _, err := connectedprofile.NewRichProcessesDockerPolicy("srv_00000000000000000000000000000000", "agent_00000000000000000000000000000000", cid, uid, ug, sharedGID, rid, frameGID, socketGID); err != nil {
		return dockerInstallLayout{}, ErrRecovery
	}
	return dockerInstallLayout{richInstallLayout: richInstallLayout{installLayout{collectorUID: cid, uploaderUID: uid, uploaderGID: ug, sharedGID: sharedGID}}, readerUID: rid, frameGID: frameGID, socketGID: socketGID}, nil
}
