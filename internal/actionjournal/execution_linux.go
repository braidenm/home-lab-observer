//go:build linux

package actionjournal

import (
	"bytes"
	"context"
	"math"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionbudget"
	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

type executionCommit struct {
	action string
	digest [32]byte
}

func (g *executionCommit) check(ctx context.Context) error {
	if g == nil || actionbudget.CheckExecution(ctx, g.action, g.digest) != nil {
		return ErrRecovery
	}
	return nil
}

type boundExecution struct {
	journal *Journal
	key     actioncore.Key
	guard   executionCommit
}

// BindExecution is only timing/key correlation for a freshly validated live
// receipt. The digest is NOT persisted full-wire identity or remote permission.
// Raw v1 APIs remain unchanged; the runnable broker never falls back to them.
func (j *Journal) BindExecution(k actioncore.Key, digest [32]byte) (actioncore.Journal, error) {
	r := k.Metadata.Request
	if j == nil || r.OwnerID != r.ActorID || r.PolicyRevision > math.MaxInt64 || r.CapabilityRevision > math.MaxInt64 ||
		r.ExpiresAt.Sub(r.CreatedAt) != 300*time.Second || r.CreatedAt.Nanosecond()%1000 != 0 || r.ExpiresAt.Nanosecond()%1000 != 0 ||
		(Binding{r.ServerID, r.InstallationID, r.OwnerID}) != j.binding {
		return nil, actioncore.ErrIdentityConflict
	}
	if _, err := actioncore.CanonicalJournalMetadata(k); err != nil {
		return nil, actioncore.ErrIdentityConflict
	}
	return &boundExecution{j, k, executionCommit{k.ActionID, digest}}, nil
}

func (b *boundExecution) Acquire(ctx context.Context, k actioncore.Key) (actioncore.Attempt, error) {
	want, e1 := actioncore.CanonicalJournalMetadata(b.key)
	got, e2 := actioncore.CanonicalJournalMetadata(k)
	if e1 != nil || e2 != nil || !bytes.Equal(want, got) {
		return nil, actioncore.ErrIdentityConflict
	}
	return b.journal.acquire(ctx, k, &b.guard)
}
