//go:build linux

package connectedenroll

import (
	"context"
	"encoding/json"
	"io"

	"github.com/braidenm/home-lab-observer/internal/connectedcredential"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/enrollmentcoord"
	"github.com/braidenm/home-lab-observer/internal/enrollmentstore"
	"github.com/braidenm/home-lab-observer/internal/platformtransport"
	"github.com/braidenm/home-lab-observer/internal/uploadledger"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
)

type RichDockerGrantInput struct {
	ServerID         string `json:"server_id"`
	Grant            string `json:"grant"`
	CollectorUID     uint32 `json:"collector_uid"`
	UploaderUID      uint32 `json:"uploader_uid"`
	UploaderGID      uint32 `json:"uploader_gid"`
	SharedGID        uint32 `json:"shared_gid"`
	ReaderUID        uint32 `json:"reader_uid"`
	FrameGID         uint32 `json:"frame_gid"`
	SocketGID        uint32 `json:"socket_gid"`
	DefinitionSHA256 string `json:"collection_definition_sha256"`
}

type RichDockerValidationInput struct {
	ServerID         string `json:"server_id"`
	ConnectorID      string `json:"connector_id"`
	CollectorUID     uint32 `json:"collector_uid"`
	UploaderUID      uint32 `json:"uploader_uid"`
	UploaderGID      uint32 `json:"uploader_gid"`
	SharedGID        uint32 `json:"shared_gid"`
	ReaderUID        uint32 `json:"reader_uid"`
	FrameGID         uint32 `json:"frame_gid"`
	SocketGID        uint32 `json:"socket_gid"`
	DefinitionSHA256 string `json:"collection_definition_sha256"`
}

func richDockerPrincipals(collector, uploader, uploaderGID, sharedGID, readerUID, frameGID, socketGID uint32) enrollmentstore.RichProcessesDockerPrincipals {
	return enrollmentstore.RichProcessesDockerPrincipals{CollectorUID: collector, UploaderUID: uploader, UploaderGID: uploaderGID, SharedGID: sharedGID, ReaderUID: readerUID, FrameGID: frameGID, SocketGID: socketGID}
}

func validRichDockerGrant(g RichDockerGrantInput) bool {
	if g.DefinitionSHA256 != connectedprofile.RichProcessesDockerDefinitionSHA256 || !principal(g.UploaderUID, g.UploaderGID, g.SharedGID) {
		return false
	}
	_, err := connectedprofile.NewRichProcessesDockerPolicy("srv_00000000000000000000000000000000", "agent_00000000000000000000000000000000", g.CollectorUID, g.UploaderUID, g.UploaderGID, g.SharedGID, g.ReaderUID, g.FrameGID, g.SocketGID)
	return err == nil
}

func richDockerProfile(v RichDockerValidationInput) (uploadstate.ProfileBinding, enrollmentstore.RichProcessesDockerPrincipals, bool) {
	principals := richDockerPrincipals(v.CollectorUID, v.UploaderUID, v.UploaderGID, v.SharedGID, v.ReaderUID, v.FrameGID, v.SocketGID)
	if v.DefinitionSHA256 != connectedprofile.RichProcessesDockerDefinitionSHA256 || !principal(v.UploaderUID, v.UploaderGID, v.SharedGID) {
		return uploadstate.ProfileBinding{}, principals, false
	}
	policy, err := connectedprofile.NewRichProcessesDockerPolicy(v.ServerID, v.ConnectorID, v.CollectorUID, v.UploaderUID, v.UploaderGID, v.SharedGID, v.ReaderUID, v.FrameGID, v.SocketGID)
	if err != nil {
		return uploadstate.ProfileBinding{}, principals, false
	}
	digest, err := connectedprofile.DigestRichProcessesDockerPolicy(policy)
	return uploadstate.ProfileBinding{Kind: uploadstate.RichProcessesDockerProfile, PolicyDigest: digest}, principals, err == nil
}

// RunRichProcessesDocker owns only the fixed process and Docker enrollment modes. It does
// not accept numeric input, dynamic profiles, paths or a precomputed digest.
func RunRichProcessesDocker(ctx context.Context, mode string, in io.Reader, out io.Writer) int {
	if ctx == nil || ctx.Err() != nil || in == nil || out == nil {
		return 22
	}
	switch mode {
	case "enroll":
		return enrollRichProcessesDocker(ctx, in, out)
	case "validate-enrollment":
		return validateRichProcessesDockerEnrollment(ctx, in, out)
	case "validate-ledger":
		return validateRichProcessesDockerLedger(ctx, in, out)
	default:
		return 22
	}
}

func enrollRichProcessesDocker(ctx context.Context, in io.Reader, out io.Writer) int {
	var g RichDockerGrantInput
	if input(in, &g) != nil || !validRichDockerGrant(g) {
		return 22
	}
	principals := richDockerPrincipals(g.CollectorUID, g.UploaderUID, g.UploaderGID, g.SharedGID, g.ReaderUID, g.FrameGID, g.SocketGID)
	store, err := enrollmentstore.OpenNewRichProcessesDocker(ctx, Directory, principals)
	if err != nil {
		return 22
	}
	defer store.Close()
	attempt := &capture{Setup: store}
	transport, err := platformtransport.NewRichProcessesDockerEnrollment(connectedprofile.Origin)
	if err != nil {
		return 22
	}
	defer transport.CloseIdleConnections()
	coordinator, err := enrollmentcoord.New(richGrantSource{server: g.ServerID, grant: g.Grant}, attempt, transport, store, store)
	if err != nil {
		return 22
	}
	result, err := coordinator.Run(ctx)
	g.Grant = ""
	if err != nil || result != enrollmentcoord.Ready || store.Close() != nil {
		return 22
	}
	b, err := json.Marshal(Binding{attempt.binding.ServerID, attempt.binding.ConnectorID})
	if err != nil {
		return 22
	}
	n, err := out.Write(b)
	if err != nil || n != len(b) {
		return 22
	}
	return 0
}

func validateRichProcessesDockerLedger(ctx context.Context, in io.Reader, out io.Writer) int {
	var expected RichDockerValidationInput
	if input(in, &expected) != nil {
		return 22
	}
	profile, _, ok := richDockerProfile(expected)
	if !ok {
		return 22
	}
	binding := uploadstate.Binding{ServerID: expected.ServerID, ConnectorID: expected.ConnectorID}
	ledger, err := uploadledger.OpenExistingForProfile(ctx, "/state/ledger", binding, profile)
	if err != nil {
		return 22
	}
	defer ledger.Close()
	r, err := ledger.Load(ctx)
	if err != nil || r != (uploadstate.Record{Binding: binding, Profile: profile}) || ledger.Close() != nil {
		return 22
	}
	n, err := out.Write([]byte("VALID"))
	if err != nil || n != 5 {
		return 22
	}
	return 0
}

func validateRichProcessesDockerEnrollment(ctx context.Context, in io.Reader, out io.Writer) int {
	var expected RichDockerValidationInput
	if input(in, &expected) != nil {
		return 22
	}
	profile, principals, ok := richDockerProfile(expected)
	if !ok {
		return 22
	}
	binding := uploadstate.Binding{ServerID: expected.ServerID, ConnectorID: expected.ConnectorID}
	ready, err := enrollmentstore.OpenReadyRichProcessesDocker(ctx, Directory, binding, principals)
	if err != nil {
		return 22
	}
	defer ready.Close()
	record, err := ready.Ledger().Load(ctx)
	if err != nil || record != (uploadstate.Record{Binding: binding, Profile: profile}) {
		return 22
	}
	credential, err := ready.Credential()
	if err != nil {
		return 22
	}
	defer clear(credential.Secret)
	data, err := json.Marshal(connectedcredential.CredentialRecord{Version: "observer-connected-credential/v1", ServerID: expected.ServerID, ConnectorID: expected.ConnectorID, Secret: string(credential.Secret)})
	defer clear(data)
	if err != nil || ready.Close() != nil {
		return 22
	}
	n, err := out.Write(data)
	if err != nil || n != len(data) {
		return 22
	}
	return 0
}
