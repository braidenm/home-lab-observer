//go:build linux

// Package connectedenroll composes one explicit private enrollment invocation.
package connectedenroll

import (
	"bytes"
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

const Directory = "/state/enrollment"

type GrantInput struct {
	ServerID    string `json:"server_id"`
	Grant       string `json:"grant"`
	UploaderUID uint32 `json:"uploader_uid"`
	UploaderGID uint32 `json:"uploader_gid"`
	SharedGID   uint32 `json:"shared_gid"`
}
type Binding struct {
	ServerID    string `json:"server_id"`
	ConnectorID string `json:"connector_id"`
}

type ValidationInput struct {
	ServerID    string `json:"server_id"`
	ConnectorID string `json:"connector_id"`
	UploaderUID uint32 `json:"uploader_uid"`
	UploaderGID uint32 `json:"uploader_gid"`
	SharedGID   uint32 `json:"shared_gid"`
}

func principal(uid, gid, shared uint32) bool {
	if uid == 0 || gid == 0 || shared == 0 || gid == shared || uid == ^uint32(0) || gid == ^uint32(0) || shared == ^uint32(0) {
		return false
	}
	return connectedprofile.CheckIdentity(connectedprofile.Config{UploaderUID: uid, UploaderGID: gid, SharedGID: shared}, false) == nil
}

type grantSource struct{ g GrantInput }

func (g grantSource) Read(context.Context) (enrollmentcoord.Grant, error) {
	return enrollmentcoord.Grant{ExpectedServerID: g.g.ServerID, Secret: []byte(g.g.Grant)}, nil
}

type capture struct {
	*enrollmentstore.Setup
	binding uploadstate.Binding
}

func (c *capture) Begin(ctx context.Context, b uploadstate.Binding) error {
	if err := c.Setup.Begin(ctx, b); err != nil {
		return err
	}
	c.binding = b
	return nil
}

func input(r io.Reader, target any) error {
	b, err := io.ReadAll(io.LimitReader(r, 513))
	defer clear(b)
	if err != nil || len(b) > 512 || json.Unmarshal(b, target) != nil {
		return connectedprofile.ErrUnsafe
	}
	canonical, err := json.Marshal(target)
	defer clear(canonical)
	if err != nil || !bytes.Equal(b, canonical) {
		return connectedprofile.ErrUnsafe
	}
	return nil
}

// Run reads/writes only parent-owned anonymous pipes. It never prints errors,
// retries an exchange, repairs a partial directory, or starts either worker.
func Run(ctx context.Context, mode string, in io.Reader, out io.Writer) int {
	if ctx == nil || ctx.Err() != nil {
		return 22
	}
	if mode == "validate-enrollment" {
		return validate(ctx, in, out)
	}
	if mode == "validate-ledger" {
		return validateLedger(ctx, in, out)
	}
	if mode != "enroll" {
		return 22
	}
	var g GrantInput
	if input(in, &g) != nil || !principal(g.UploaderUID, g.UploaderGID, g.SharedGID) {
		return 22
	}
	store, err := enrollmentstore.OpenNew(ctx, Directory)
	if err != nil {
		return 22
	}
	defer store.Close()
	attempt := &capture{Setup: store}
	transport, err := platformtransport.NewEnrollment(connectedprofile.Origin)
	if err != nil {
		return 22
	}
	defer transport.CloseIdleConnections()
	coordinator, err := enrollmentcoord.New(grantSource{g}, attempt, transport, store, store)
	if err != nil {
		return 22
	}
	result, err := coordinator.Run(ctx)
	g.Grant = ""
	if err != nil || result != enrollmentcoord.Ready {
		return 22
	}
	if store.Close() != nil {
		return 22
	}
	b, _ := json.Marshal(Binding{attempt.binding.ServerID, attempt.binding.ConnectorID})
	n, err := out.Write(b)
	if err != nil || n != len(b) {
		return 22
	}
	return 0
}

func validateLedger(ctx context.Context, in io.Reader, out io.Writer) int {
	var expected ValidationInput
	if input(in, &expected) != nil || !principal(expected.UploaderUID, expected.UploaderGID, expected.SharedGID) {
		return 22
	}
	ledger, err := uploadledger.OpenExisting(ctx, "/state/ledger", uploadstate.Binding{ServerID: expected.ServerID, ConnectorID: expected.ConnectorID})
	if err != nil {
		return 22
	}
	defer ledger.Close()
	r, err := ledger.Load(ctx)
	if err != nil || r.Watermark != 0 || r.HasAck || r.Pending != nil || r.Stopped != "" || ledger.Close() != nil {
		return 22
	}
	n, err := out.Write([]byte("VALID"))
	if err != nil || n != 5 {
		return 22
	}
	return 0
}

func validate(ctx context.Context, in io.Reader, out io.Writer) int {
	var expected ValidationInput
	if input(in, &expected) != nil || !principal(expected.UploaderUID, expected.UploaderGID, expected.SharedGID) {
		return 22
	}
	binding := uploadstate.Binding{ServerID: expected.ServerID, ConnectorID: expected.ConnectorID}
	ready, err := enrollmentstore.OpenReady(ctx, Directory, binding)
	if err != nil {
		return 22
	}
	defer ready.Close()
	record, err := ready.Ledger().Load(ctx)
	if err != nil || record.Watermark != 0 || record.HasAck || record.Pending != nil || record.Stopped != "" {
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
