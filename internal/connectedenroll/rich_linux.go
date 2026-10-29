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

type RichGrantInput struct {
	ServerID         string `json:"server_id"`
	Grant            string `json:"grant"`
	CollectorUID     uint32 `json:"collector_uid"`
	UploaderUID      uint32 `json:"uploader_uid"`
	UploaderGID      uint32 `json:"uploader_gid"`
	SharedGID        uint32 `json:"shared_gid"`
	DefinitionSHA256 string `json:"collection_definition_sha256"`
}

type RichValidationInput struct {
	ServerID         string `json:"server_id"`
	ConnectorID      string `json:"connector_id"`
	CollectorUID     uint32 `json:"collector_uid"`
	UploaderUID      uint32 `json:"uploader_uid"`
	UploaderGID      uint32 `json:"uploader_gid"`
	SharedGID        uint32 `json:"shared_gid"`
	DefinitionSHA256 string `json:"collection_definition_sha256"`
}

type richGrantSource struct{ server, grant string }

func (g richGrantSource) Read(context.Context) (enrollmentcoord.Grant, error) {
	return enrollmentcoord.Grant{ExpectedServerID: g.server, Secret: []byte(g.grant)}, nil
}

func richPrincipals(collector, uploader, uploaderGID, sharedGID uint32) enrollmentstore.RichProcessesPrincipals {
	return enrollmentstore.RichProcessesPrincipals{CollectorUID: collector, UploaderUID: uploader, UploaderGID: uploaderGID, SharedGID: sharedGID}
}

func validRichGrant(g RichGrantInput) bool {
	if g.DefinitionSHA256 != connectedprofile.RichProcessesDefinitionSHA256 || !principal(g.UploaderUID, g.UploaderGID, g.SharedGID) {
		return false
	}
	_, err := connectedprofile.NewRichProcessesPolicy("srv_00000000000000000000000000000000", "agent_00000000000000000000000000000000", g.CollectorUID, g.UploaderUID, g.UploaderGID, g.SharedGID)
	return err == nil
}

func richProfile(v RichValidationInput) (uploadstate.ProfileBinding, enrollmentstore.RichProcessesPrincipals, bool) {
	principals := richPrincipals(v.CollectorUID, v.UploaderUID, v.UploaderGID, v.SharedGID)
	if v.DefinitionSHA256 != connectedprofile.RichProcessesDefinitionSHA256 || !principal(v.UploaderUID, v.UploaderGID, v.SharedGID) {
		return uploadstate.ProfileBinding{}, principals, false
	}
	policy, err := connectedprofile.NewRichProcessesPolicy(v.ServerID, v.ConnectorID, v.CollectorUID, v.UploaderUID, v.UploaderGID, v.SharedGID)
	if err != nil {
		return uploadstate.ProfileBinding{}, principals, false
	}
	digest, err := connectedprofile.DigestRichProcessesPolicy(policy)
	return uploadstate.ProfileBinding{Kind: uploadstate.RichProcessesProfile, PolicyDigest: digest}, principals, err == nil
}

// RunRichProcesses owns only the fixed process-only enrollment modes. It does
// not accept numeric input, dynamic profiles, paths or a precomputed digest.
func RunRichProcesses(ctx context.Context, mode string, in io.Reader, out io.Writer) int {
	if ctx == nil || ctx.Err() != nil || in == nil || out == nil {
		return 22
	}
	switch mode {
	case "enroll":
		return enrollRichProcesses(ctx, in, out)
	case "validate-enrollment":
		return validateRichProcessesEnrollment(ctx, in, out)
	case "validate-ledger":
		return validateRichProcessesLedger(ctx, in, out)
	default:
		return 22
	}
}

func enrollRichProcesses(ctx context.Context, in io.Reader, out io.Writer) int {
	var g RichGrantInput
	if input(in, &g) != nil || !validRichGrant(g) {
		return 22
	}
	principals := richPrincipals(g.CollectorUID, g.UploaderUID, g.UploaderGID, g.SharedGID)
	store, err := enrollmentstore.OpenNewRichProcesses(ctx, Directory, principals)
	if err != nil {
		return 22
	}
	defer store.Close()
	attempt := &capture{Setup: store}
	transport, err := platformtransport.NewRichProcessesEnrollment(connectedprofile.Origin)
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

func validateRichProcessesLedger(ctx context.Context, in io.Reader, out io.Writer) int {
	var expected RichValidationInput
	if input(in, &expected) != nil {
		return 22
	}
	profile, _, ok := richProfile(expected)
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

func validateRichProcessesEnrollment(ctx context.Context, in io.Reader, out io.Writer) int {
	var expected RichValidationInput
	if input(in, &expected) != nil {
		return 22
	}
	profile, principals, ok := richProfile(expected)
	if !ok {
		return 22
	}
	binding := uploadstate.Binding{ServerID: expected.ServerID, ConnectorID: expected.ConnectorID}
	ready, err := enrollmentstore.OpenReadyRichProcesses(ctx, Directory, binding, principals)
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
