package actionremote

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/braidenm/home-lab-observer/internal/actionprotocol"
)

func (c *Client) SnapshotHead(ctx context.Context) (actionprotocol.SnapshotView, error) {
	status, raw, err := c.request(ctx, http.MethodGet, "/approvals", nil)
	if err != nil {
		return actionprotocol.SnapshotView{}, err
	}
	if status != http.StatusOK {
		return actionprotocol.SnapshotView{}, ErrUnavailable
	}
	v, err := actionprotocol.DecodeSnapshotView(raw)
	if err != nil {
		return actionprotocol.SnapshotView{}, ErrUnavailable
	}
	if v.ServerID != c.binding.ServerID || v.OwnerUserID != c.binding.OwnerUserID || v.ConnectorInstanceID != c.binding.ConnectorInstanceID {
		return actionprotocol.SnapshotView{}, ErrConflict
	}
	return v, nil
}

func (c *Client) Publish(ctx context.Context, p actionprotocol.Publication) (actionprotocol.SnapshotReceipt, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return actionprotocol.SnapshotReceipt{}, ErrInvalid
	}
	if _, err = actionprotocol.DecodePublication(body); err != nil {
		return actionprotocol.SnapshotReceipt{}, ErrInvalid
	}
	status, raw, err := c.request(ctx, http.MethodPost, "/approvals", body)
	if err != nil {
		return actionprotocol.SnapshotReceipt{}, err
	}
	if status != http.StatusOK {
		return actionprotocol.SnapshotReceipt{}, ErrUnavailable
	}
	r, err := actionprotocol.DecodeSnapshotReceipt(raw)
	if err != nil || r.PublicationID != p.PublicationID {
		return actionprotocol.SnapshotReceipt{}, ErrUnavailable
	}
	// An equal publication retry returns its original receipt, whose revision
	// still equals that publication's expected head plus one. A valid digest
	// alone cannot turn a stale/jumped head into successful CAS.
	expected := int64(0)
	if p.ExpectedSnapshotRevision != nil {
		expected, err = strconv.ParseInt(*p.ExpectedSnapshotRevision, 10, 64)
		if err != nil || expected == math.MaxInt64 {
			return actionprotocol.SnapshotReceipt{}, ErrUnavailable
		}
	}
	if r.SnapshotRevision != strconv.FormatInt(expected+1, 10) {
		return actionprotocol.SnapshotReceipt{}, ErrUnavailable
	}
	digest, err := actionprotocol.SnapshotDigest(c.binding.ServerID, c.binding.OwnerUserID, c.binding.ConnectorInstanceID, r, p.Entries)
	if err != nil || digest != r.SnapshotDigest {
		return actionprotocol.SnapshotReceipt{}, ErrUnavailable
	}
	return r, nil
}

// RefreshSnapshot has ONE total budget and one POST at most. Missing/error and
// uncertain publication are never interpreted as a fresh head or retried here.
func (c *Client) RefreshSnapshot(parent context.Context, q actionprotocol.SnapshotQuery) (actionprotocol.SnapshotReply, error) {
	if c == nil || parent == nil || actionprotocol.ValidateSnapshotQuery(q) != nil {
		return actionprotocol.SnapshotReply{Outcome: "UNAVAILABLE"}, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(parent, requestTimeout)
	defer cancel()
	v, err := c.SnapshotHead(ctx)
	if err == nil {
		err = actionprotocol.MatchSnapshotHead(q, v)
	}
	if err == nil {
		r, e := c.Publish(ctx, actionprotocol.Publication{PublicationID: q.PublicationID, ExpectedSnapshotRevision: v.SnapshotRevision, Entries: q.Entries})
		err = e
		if err == nil {
			reply := actionprotocol.SnapshotReply{Outcome: "OK", Receipt: &r}
			if actionprotocol.MatchSnapshotReply(q, reply) == nil && ctx.Err() == nil {
				return reply, nil
			}
			err = ErrUnavailable
		}
	}
	outcome := "UNAVAILABLE"
	if errors.Is(err, ErrDenied) || errors.Is(err, ErrCredential) {
		outcome = "DENIED"
	}
	if errors.Is(err, ErrConflict) {
		outcome = "CONFLICT"
	}
	return actionprotocol.SnapshotReply{Outcome: outcome}, err
}
