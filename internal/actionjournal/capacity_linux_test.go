//go:build linux

package actionjournal

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

func rebuild(t *testing.T, k actioncore.Key) actioncore.Key {
	t.Helper()
	r := k.Metadata.Request
	b := actioncore.Binding{ServerID: r.ServerID, InstallationID: r.InstallationID, OwnerID: r.OwnerID, TargetID: r.TargetID, EngineGeneration: k.Metadata.EngineGeneration, ContainerID: k.Metadata.ContainerID, PolicyRevision: r.PolicyRevision, CapabilityRevision: r.CapabilityRevision, GraceSeconds: r.GraceSeconds, Operations: []actioncore.Operation{r.Operation}}
	v, e := actioncore.NewJournalKey(r, b)
	if e != nil {
		t.Fatal("key rebuild")
	}
	return v
}
func maxKey(t *testing.T, n int) actioncore.Key {
	k := key(t, n, n)
	k.Metadata.Request.Operation = actioncore.Restart
	k.Metadata.Request.GraceSeconds = 30
	k.Metadata.Request.PolicyRevision = math.MaxUint64
	k.Metadata.Request.CapabilityRevision = math.MaxUint64
	k.Metadata.Request.CreatedAt = epoch.Add(123456789)
	k.Metadata.Request.ExpiresAt = k.Metadata.Request.CreatedAt.Add(5 * time.Minute)
	return rebuild(t, k)
}
func rowCount(t *testing.T, j *Journal) int {
	t.Helper()
	var n int
	if j.conn.QueryRowContext(ctx, `SELECT count(*) FROM actions`).Scan(&n) != nil {
		t.Fatal("count")
	}
	return n
}

func TestMaximumCapacityCompletionAndSamples(t *testing.T) {
	j, dir := provision(t)
	j.now = func() time.Time { return epoch.Add(time.Second) }
	var sampledMaximum int64
	sample := func() error {
		var size int64
		for _, name := range []string{databaseName, journalName} {
			i, e := os.Lstat(filepath.Join(dir, name))
			if errors.Is(e, os.ErrNotExist) {
				continue
			}
			if e != nil || !privateFile(i, totalBytes) {
				return ErrUnsafe
			}
			size += i.Size()
		}
		if size > sampledMaximum {
			sampledMaximum = size
		}
		if size > totalBytes {
			return ErrRecovery
		}
		return nil
	}
	j.beforeCommit = sample
	j.afterCommit = sample
	for n := 1; n <= maxRecords; n++ {
		a := acquire(t, j, maxKey(t, n))
		if a.Intent(ctx) != nil || a.Finish(ctx, actioncore.Restarted) != nil {
			t.Fatal("reserved completion failed")
		}
		a.Close()
	}
	if _, e := j.Acquire(ctx, maxKey(t, 1001)); !errors.Is(e, ErrFull) || j.failed {
		t.Fatal("logical capacity")
	}
	a := acquire(t, j, maxKey(t, 1))
	if a.Existing() != actioncore.Restarted {
		t.Fatal("full store hid retained result")
	}
	a.Close()
	derived := int64(databaseBytes + (databaseBytes/4096)*(4096+8) + 2*65536)
	if rowCount(t, j) != 1000 || derived != 25321472 || derived >= totalBytes || sampledMaximum <= 0 || sampledMaximum > derived {
		t.Fatal("capacity/derived peak")
	}
	// These samples do not observe every COMMIT-time intermediate. The separate
	// conservative derivation, not sampling, supplies the global file-size bound.
	t.Logf("DB+journal before/after-COMMIT sample maximum=%d derived_ceiling=25321472 cap=33554432", sampledMaximum)
	j.Close()
	re, e := OpenExisting(ctx, dir, owner)
	if e != nil {
		t.Fatal("maximum reopen")
	}
	defer re.Close()
	re.now = func() time.Time { return epoch.Add(31 * 24 * time.Hour) }
	if re.Prune(ctx) != nil || rowCount(t, re) != 950 {
		t.Fatal("prune not bounded to fifty")
	}
	re.now = func() time.Time { return epoch.Add(time.Second) }
	if _, e := re.Acquire(ctx, maxKey(t, 1)); !errors.Is(e, actioncore.ErrIdentityConflict) {
		t.Fatal("clock rollback resurrected pruned key")
	}
}

func TestPruneAtomicityAndLiveExclusion(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "committed-uncertain"}[after], func(t *testing.T) {
			j, dir := provision(t)
			a := acquire(t, j, key(t, 1, 1))
			if a.Finish(ctx, actioncore.AlreadyRunning) != nil {
				t.Fatal("terminal")
			}
			a.Close()
			j.now = func() time.Time { return epoch.Add(31 * 24 * time.Hour) }
			fault := func() error { return ErrRecovery }
			if after {
				j.afterCommit = fault
			} else {
				j.beforeCommit = fault
			}
			if j.Prune(ctx) == nil || !j.failed {
				t.Fatal("prune uncertainty")
			}
			j.Close()
			re, e := OpenExisting(ctx, dir, owner)
			if e != nil {
				t.Fatal("reopen")
			}
			defer re.Close()
			floor, e := re.floor(ctx)
			if e != nil {
				t.Fatal("floor")
			}
			if after {
				if rowCount(t, re) != 0 || !floor.Equal(epoch.Add(24*time.Hour)) {
					t.Fatal("floor/delete not atomic")
				}
			} else if rowCount(t, re) != 1 || !floor.IsZero() {
				t.Fatal("rolled back prune changed floor")
			}
		})
	}
	j, _ := provision(t)
	a := acquire(t, j, key(t, 1, 1))
	j.now = func() time.Time { return epoch.Add(31 * 24 * time.Hour) }
	if j.Prune(ctx) != nil {
		t.Fatal("prune live")
	}
	r, e := j.read(ctx, key(t, 1, 1).ActionID)
	if e != nil || !r.terminal.IsZero() {
		t.Fatal("live admission expired by maintenance")
	}
	a.Close()
	if j.Prune(ctx) != nil {
		t.Fatal("expire undispatched")
	}
	r, e = j.read(ctx, key(t, 1, 1).ActionID)
	if e != nil || r.code != actioncore.Denied || r.reason != "EXPIRED_UNDISPATCHED" || !r.intent.IsZero() {
		t.Fatal("expired admission evidence")
	}
}

func TestSQLitePageExhaustionIsNotLogicalCapacity(t *testing.T) {
	j, _ := provision(t)
	var pages int
	j.conn.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pages)
	j.conn.ExecContext(ctx, `PRAGMA max_page_count=`+strconv.Itoa(pages))
	refused := false
	for n := 1; n < maxRecords; n++ {
		a, e := j.Acquire(ctx, key(t, n, n))
		if e != nil {
			if !errors.Is(e, ErrRecovery) || !j.failed {
				t.Fatal("SQL capacity not poisoned")
			}
			refused = true
			break
		}
		a.Close()
	}
	if !refused {
		t.Fatal("SQL page limit not reached")
	}
}

func TestRollbackKeepsActualThirtyDayEligibilitySeparateFromFloor(t *testing.T) {
	j, _ := provision(t)
	for n := 1; n <= 51; n++ {
		a := acquire(t, j, key(t, n, n))
		if a.Finish(ctx, actioncore.AlreadyRunning) != nil {
			t.Fatal("terminal")
		}
		a.Close()
	}
	j.now = func() time.Time { return epoch.Add(31 * 24 * time.Hour) }
	if j.Prune(ctx) != nil || rowCount(t, j) != 1 {
		t.Fatal("bounded forward prune")
	}
	j.now = func() time.Time { return epoch.Add(time.Hour) }
	if j.Prune(ctx) != nil || rowCount(t, j) != 1 {
		t.Fatal("floor erased terminal younger than thirty days")
	}
	a := acquire(t, j, key(t, 51, 51))
	defer a.Close()
	if a.Existing() != actioncore.AlreadyRunning {
		t.Fatal("retained terminal hidden")
	}
}
