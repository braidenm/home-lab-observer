//go:build linux

package uploadledger

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
	"github.com/braidenm/home-lab-observer/internal/containerobs/model"
	"github.com/braidenm/home-lab-observer/internal/observation"
	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
	"github.com/braidenm/home-lab-observer/internal/uploadstate"
	"github.com/braidenm/home-lab-observer/internal/workloadprojection"
)

var rich = uploadstate.ProfileBinding{Kind: uploadstate.RichProcessesProfile, PolicyDigest: sha256.Sum256([]byte("synthetic-fixed-policy"))}

func richPending(t *testing.T, seq int64, offset time.Duration) uploadstate.Record {
	t.Helper()
	observed := at.Add(offset)
	processes := make([]observation.Process, workloadprojection.MaxProcesses)
	for i := range processes {
		// Maximum-length names expand under canonical HTML-safe JSON encoding,
		// exercising the section byte budget, not only the 200-row count.
		processes[i] = observation.Process{PID: int32(i + 1), Name: fmt.Sprintf("%03d", i) + strings.Repeat("<", 125), State: "running", CPUPercent: 409600, MemoryBytes: remoteprojection.MaxExactInteger, UptimeSeconds: remoteprojection.MaxExactInteger}
	}
	raw := observation.Snapshot{SchemaVersion: observation.SchemaVersion, ObservedAt: observed,
		Processes: observation.Section[[]observation.Process]{State: observation.Available, Data: &processes, Quality: observation.SectionQuality{Samples: len(processes), Total: len(processes)}}}
	id := remoteprojection.Identity{SourceID: binding.ServerID, Version: "0.2.0-canary.1", OS: "linux"}
	host, err := remoteprojection.EncodeNative(raw, id)
	if err != nil {
		t.Fatal("synthetic host failed")
	}
	workloads, err := workloadprojection.Encode(raw, model.Inventory{}, workloadprojection.Identity{SourceID: binding.ServerID, Version: id.Version}, workloadprojection.Consent{Processes: true})
	if err != nil {
		t.Fatal("synthetic workloads failed")
	}
	body, err := combinedprojection.Encode(host, workloads, binding.ServerID)
	if err != nil || len(body) <= remoteprojection.MaxBytes {
		t.Fatal("large synthetic combined body failed")
	}
	return uploadstate.Record{Binding: binding, Profile: rich, Watermark: seq, Pending: &uploadstate.Pending{Sequence: seq, Body: body, Digest: sha256.Sum256(body), CollectedAt: observed}}
}

func provisionRich(t *testing.T) (*Ledger, string) {
	t.Helper()
	dir := directory(t)
	l, err := ProvisionForProfile(context.Background(), dir, binding, rich)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l, dir
}

func TestInvalidLedgerProfilesRefuseBeforeFilesystemMutation(t *testing.T) {
	for _, profile := range []uploadstate.ProfileBinding{{Kind: uploadstate.ProfileKind(255)}, {Kind: uploadstate.RichProcessesProfile}, {PolicyDigest: rich.PolicyDigest}} {
		dir := directory(t)
		if l, err := ProvisionForProfile(context.Background(), dir, binding, profile); err != ErrRecovery {
			if l != nil {
				l.Close()
			}
			t.Fatal("invalid provision profile admitted")
		}
		if l, err := OpenExistingForProfile(context.Background(), dir, binding, profile); err != ErrRecovery {
			if l != nil {
				l.Close()
			}
			t.Fatal("invalid open profile admitted")
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			t.Fatal("invalid profile touched storage")
		}
	}
}

func TestRichRoundTripSettingsAndMaximumSequence(t *testing.T) {
	if limits(rich).bodyBytes != combinedprojection.MaxBytes {
		t.Fatal("fixed rich storage/wire ceiling drift")
	}
	l, dir := provisionRich(t)
	initial := load(t, l)
	if initial.Profile != rich || initial.Watermark != 0 {
		t.Fatal("initial rich identity missing")
	}
	next := richPending(t, math.MaxInt64, 0)
	if l.Commit(context.Background(), initial, next) != nil || l.configure(context.Background()) != nil {
		t.Fatal("rich pending/settings refused")
	}
	l.Close()
	reopened, err := OpenExistingForProfile(context.Background(), dir, binding, rich)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got := load(t, reopened)
	if !sameRecord(got, next) || !bytes.Equal(got.Pending.Body, next.Pending.Body) {
		t.Fatal("rich pending bytes/identity changed")
	}
	ack := uploadstate.Record{Binding: binding, Profile: rich, Watermark: math.MaxInt64, HasAck: true, LastAck: next.Pending.Digest, Stopped: uploadstate.Exhausted}
	if reopened.Commit(context.Background(), got, ack) != nil || !sameRecord(load(t, reopened), ack) {
		t.Fatal("rich terminal ack failed")
	}
}

func TestRichProfileMutationAndCASNeverAdopted(t *testing.T) {
	for _, name := range []string{"expected-policy", "next-policy", "next-kind", "pending-replace", "stale", "numeric-body", "pending-digest", "pending-time", "pending-source", "pending-group"} {
		t.Run(name, func(t *testing.T) {
			l, _ := provisionRich(t)
			initial := load(t, l)
			first := richPending(t, 1, 0)
			if l.Commit(context.Background(), initial, first) != nil {
				t.Fatal("first admission refused")
			}
			expected, next := uploadstate.CloneRecord(first), richPending(t, 2, time.Second)
			switch name {
			case "expected-policy":
				expected.Profile.PolicyDigest[0]++
			case "next-policy":
				next.Profile.PolicyDigest[0]++
			case "next-kind":
				next.Profile = uploadstate.ProfileBinding{}
			case "pending-replace":
				next = richPending(t, 1, time.Second)
			case "stale":
				expected = initial
			case "numeric-body":
				next = pending(t, 2, time.Second)
				next.Profile = rich
			case "pending-digest":
				next.Pending.Digest[0]++
			case "pending-time":
				next.Pending.CollectedAt = next.Pending.CollectedAt.Add(time.Second)
			case "pending-source":
				next.Pending.Body = bytes.ReplaceAll(next.Pending.Body, []byte(binding.ServerID), []byte("srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
				next.Pending.Digest = sha256.Sum256(next.Pending.Body)
			case "pending-group":
				next.Pending.Body = bytes.Replace(next.Pending.Body, []byte(`"containers_enabled":false`), []byte(`"containers_enabled":true`), 1)
				next.Pending.Digest = sha256.Sum256(next.Pending.Body)
			}
			if l.Commit(context.Background(), expected, next) != ErrRecovery {
				t.Fatal("unsafe rich CAS accepted")
			}
			if _, err := l.Load(context.Background()); err != ErrRecovery {
				t.Fatal("unsafe CAS did not latch")
			}
		})
	}
}

func TestRichStoredPolicyAndLogicalFailuresAreNotAdopted(t *testing.T) {
	for _, name := range []string{"wrong-expected-policy", "stored-policy", "stored-kind", "stored-policy-size", "stored-body-size", "stored-body", "schema", "unknown-kind"} {
		t.Run(name, func(t *testing.T) {
			l, dir := provisionRich(t)
			l.Close()
			expected := rich
			if name == "wrong-expected-policy" {
				expected.PolicyDigest[0]++
			} else if name == "unknown-kind" {
				expected.Kind = uploadstate.ProfileKind(255)
			} else {
				db, err := sql.Open("sqlite", dataSourceForProfile(dir, rich))
				if err != nil {
					t.Fatal(err)
				}
				db.SetMaxOpenConns(1)
				if _, err := db.Exec(`PRAGMA ignore_check_constraints=ON`); err != nil {
					t.Fatal(err)
				}
				statement := map[string]string{
					"stored-policy":      `UPDATE upload_record SET policy_digest=zeroblob(32)`,
					"stored-kind":        `UPDATE upload_record SET profile_kind=257`,
					"stored-policy-size": `UPDATE upload_record SET policy_digest=zeroblob(33)`,
					"stored-body-size":   `UPDATE upload_record SET watermark=1,pending_sequence=1,pending_body=zeroblob(294913),pending_digest=zeroblob(32),pending_at='2026-09-11T12:00:01Z'`,
					"stored-body":        `UPDATE upload_record SET watermark=1,pending_sequence=1,pending_body=x'01',pending_digest=zeroblob(32),pending_at='2026-09-11T12:00:01Z'`,
					"schema":             `CREATE TABLE unexpected(x INTEGER)`,
				}[name]
				if _, err := db.Exec(statement); err != nil {
					t.Fatal(err)
				}
				db.Close()
			}
			before, err := os.ReadFile(filepath.Join(dir, databaseName))
			if err != nil {
				t.Fatal(err)
			}
			if got, err := OpenExistingForProfile(context.Background(), dir, binding, expected); err == nil {
				got.Close()
				t.Fatal("foreign/corrupt rich state accepted")
			}
			after, err := os.ReadFile(filepath.Join(dir, databaseName))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("refused settled state was changed")
			}
		})
	}
}

func TestRichSQLCapacityIsNotWireAcceptanceAndPeakIsBounded(t *testing.T) {
	l, _ := provisionRich(t)
	var peak, largestDB, largestJournal int64
	sample := func() error {
		if err := checkFilesForProfile(l.dir, true, rich); err != nil {
			return err
		}
		entries, err := os.ReadDir(l.dir)
		if err != nil {
			return err
		}
		var total int64
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			total += info.Size()
			if entry.Name() == databaseName && info.Size() > largestDB {
				largestDB = info.Size()
			}
			if entry.Name() == journalName && info.Size() > largestJournal {
				largestJournal = info.Size()
			}
		}
		if total > peak {
			peak = total
		}
		return nil
	}
	l.beforeCommit = sample
	l.afterCommit = sample
	previous := load(t, l)
	for i := int64(1); i <= 25; i++ {
		next := richPending(t, i, time.Duration(i)*time.Nanosecond)
		if l.Commit(context.Background(), previous, next) != nil {
			t.Fatal("bounded maximal eligible admission failed")
		}
		retired := uploadstate.Record{Binding: binding, Profile: rich, Watermark: i}
		if i%2 == 0 {
			retired.HasAck, retired.LastAck = true, next.Pending.Digest
		}
		if l.Commit(context.Background(), next, retired) != nil {
			t.Fatal("bounded ack/expiry failed")
		}
		previous = retired
	}
	// A schema-valid opaque BLOB at the defensive ceiling is not a valid wire
	// document. Exercise SQL capacity separately, and require Load to refuse it.
	for iteration, size := range []int{294912, 294912, 294913} {
		next := richPending(t, 26, 0)
		next.Pending.Body = bytes.Repeat([]byte{byte('x' + iteration)}, size)
		err := l.transaction(context.Background(), func() error {
			_, err := l.conn.ExecContext(context.Background(), richUpdateRecord, argsFor(next)...)
			return err
		})
		if (size == 294912 && err != nil) || (size == 294913 && err == nil) {
			t.Fatal("SQL ceiling not exact")
		}
	}
	if _, err := l.Load(context.Background()); err != ErrRecovery {
		t.Fatal("SQL capacity mistaken for wire acceptance")
	}
	if peak <= 0 || largestJournal == 0 || largestDB+largestJournal >= 2*1024*1024 {
		t.Fatal("rich transactional peak proof absent")
	}
	t.Logf("maximal canonical body=%d, before/after-commit sampled peak=%d, conservative file maxima sum=%d (includes SQL ceiling fixture)", len(richPending(t, 1, 0).Pending.Body), peak, largestDB+largestJournal)
}

type richSource struct {
	body  []byte
	reads int
}

func (s *richSource) Read(context.Context, string) ([]byte, error) { s.reads++; return s.body, nil }

type richClock struct{ at time.Time }

func (c *richClock) Now() time.Time { return c.at }

type richTransport struct {
	requests []uploadstate.Request
	ack      bool
}

func (tr *richTransport) Send(_ context.Context, r uploadstate.Request) (uploadstate.Response, error) {
	tr.requests = append(tr.requests, uploadstate.Request{Binding: r.Binding, Sequence: r.Sequence, Body: bytes.Clone(r.Body)})
	if tr.ack {
		return uploadstate.Response{Outcome: uploadstate.Acknowledged, ServerID: r.Binding.ServerID, Sequence: r.Sequence}, nil
	}
	return uploadstate.Response{Outcome: uploadstate.Retry}, nil
}

func TestRichRealLedgerMachineRetryRestartAckAndExpiry(t *testing.T) {
	l, dir := provisionRich(t)
	source := &richSource{body: richPending(t, 1, 0).Pending.Body}
	clock := &richClock{at: at}
	transport := &richTransport{}
	m, err := uploadstate.NewForProfile(binding, rich, source, clock, transport, l)
	if err != nil {
		t.Fatal(err)
	}
	if outcome, err := m.Step(context.Background()); outcome != uploadstate.Retry || err != nil {
		t.Fatal("first real admission failed")
	}
	first := load(t, l)
	l.Close()
	l, err = OpenExistingForProfile(context.Background(), dir, binding, rich)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	source.body = richPending(t, 2, time.Second).Pending.Body
	transport.ack = true
	m, err = uploadstate.NewForProfile(binding, rich, source, clock, transport, l)
	if err != nil {
		t.Fatal(err)
	}
	if outcome, err := m.Step(context.Background()); outcome != uploadstate.Acknowledged || err != nil {
		t.Fatal("reopened retry failed")
	}
	if source.reads != 1 || len(transport.requests) != 2 || transport.requests[1].Sequence != 1 || !bytes.Equal(transport.requests[1].Body, first.Pending.Body) {
		t.Fatal("reopen changed exact retry")
	}
	clock.at = at.Add(time.Second)
	transport.ack = false
	if outcome, err := m.Step(context.Background()); outcome != uploadstate.Retry || err != nil {
		t.Fatal("second admission failed")
	}
	clock.at = at.Add(uploadstate.MaxAge + 2*time.Second)
	source.body = richPending(t, 3, uploadstate.MaxAge+2*time.Second).Pending.Body
	transport.ack = true
	if outcome, err := m.Step(context.Background()); outcome != uploadstate.Acknowledged || err != nil {
		t.Fatal("expiry replacement failed")
	}
	final := load(t, l)
	if final.Watermark != 3 || final.Profile != rich || final.Pending != nil || transport.requests[3].Sequence != 3 {
		t.Fatal("expiry reused retired sequence/profile")
	}
}

func TestRichCrashRecoveryAndCrossKindHotJournalRefusal(t *testing.T) {
	if phase := os.Getenv("HLO_RICH_LEDGER_CRASH"); phase != "" {
		dir := os.Getenv("HLO_RICH_LEDGER_DIR")
		profile := rich
		if os.Getenv("HLO_RICH_LEDGER_KIND") == "numeric" {
			profile = uploadstate.ProfileBinding{}
		}
		l, err := OpenExistingForProfile(context.Background(), dir, binding, profile)
		if err != nil {
			os.Exit(4)
		}
		initial := load(t, l)
		next := richPending(t, 1, 0)
		if profile.Kind == uploadstate.NumericProfile {
			next = pending(t, 1, 0)
		}
		if phase == "hot" {
			l.Close()
			db, err := sql.Open("sqlite", dataSourceForProfile(dir, profile))
			if err != nil {
				os.Exit(4)
			}
			db.SetMaxOpenConns(1)
			for _, q := range []string{`PRAGMA cache_size=1`, `PRAGMA cache_spill=ON`, `BEGIN IMMEDIATE`} {
				if _, err := db.Exec(q); err != nil {
					os.Exit(4)
				}
			}
			next.Pending.Body = bytes.Repeat([]byte("x"), 16384)
			if _, err := db.Exec(updateFor(profile), argsFor(next)...); err != nil {
				os.Exit(4)
			}
			journal, err := os.ReadFile(filepath.Join(dir, journalName))
			if err != nil || len(journal) < 8 || !bytes.Equal(journal[:8], []byte{0xd9, 0xd5, 0x05, 0xf9, 0x20, 0xa1, 0x63, 0xd7}) {
				os.Exit(5)
			}
			os.Exit(7)
		}
		if phase == "before" {
			l.beforeCommit = func() error { os.Exit(7); return nil }
		} else {
			l.afterCommit = func() error { os.Exit(7); return nil }
		}
		l.Commit(context.Background(), initial, next)
		os.Exit(4)
	}
	for _, kind := range []string{"rich", "numeric"} {
		for _, phase := range []string{"before", "after", "hot"} {
			// Existing numeric crash tests retain their before/after proof. Only
			// add its hot case here for the new cross-kind non-mutation boundary.
			if kind == "numeric" && phase != "hot" {
				continue
			}
			t.Run(kind+"/"+phase, func(t *testing.T) {
				profile := rich
				if kind == "numeric" {
					profile = uploadstate.ProfileBinding{}
				}
				dir := directory(t)
				l, err := ProvisionForProfile(context.Background(), dir, binding, profile)
				if err != nil {
					t.Fatal(err)
				}
				initial := load(t, l)
				l.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRichCrashRecoveryAndCrossKindHotJournalRefusal$")
				command.Env = []string{"HLO_RICH_LEDGER_CRASH=" + phase, "HLO_RICH_LEDGER_DIR=" + dir, "HLO_RICH_LEDGER_KIND=" + kind}
				var exit *exec.ExitError
				if err := command.Run(); !errors.As(err, &exit) || exit.ExitCode() != 7 {
					t.Fatal("crash fixture did not reach exact boundary")
				}
				beforeDB, err := os.ReadFile(filepath.Join(dir, databaseName))
				if err != nil {
					t.Fatal(err)
				}
				beforeJournal, journalErr := os.ReadFile(filepath.Join(dir, journalName))
				if phase == "hot" && journalErr != nil {
					t.Fatal("hot journal missing")
				}
				other := rich
				if kind == "rich" {
					other = uploadstate.ProfileBinding{}
				}
				if got, err := OpenExistingForProfile(context.Background(), dir, binding, other); err == nil {
					got.Close()
					t.Fatal("cross-kind state opened")
				}
				afterDB, err := os.ReadFile(filepath.Join(dir, databaseName))
				if err != nil || !bytes.Equal(beforeDB, afterDB) {
					t.Fatal("cross-kind admission changed DB")
				}
				afterJournal, afterErr := os.ReadFile(filepath.Join(dir, journalName))
				if (journalErr == nil) != (afterErr == nil) || !bytes.Equal(beforeJournal, afterJournal) {
					t.Fatal("cross-kind admission changed journal")
				}
				reopened, err := OpenExistingForProfile(context.Background(), dir, binding, profile)
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				want := initial
				if phase == "after" {
					want = richPending(t, 1, 0)
					if kind == "numeric" {
						want = pending(t, 1, 0)
					}
				}
				if !sameRecord(load(t, reopened), want) {
					t.Fatal("same-kind recovery changed logical atomicity")
				}
			})
		}
	}
}

func TestRichUncertainCommitReopensAuthoritativeState(t *testing.T) {
	for _, after := range []bool{false, true} {
		l, dir := provisionRich(t)
		initial, next := load(t, l), richPending(t, 1, 0)
		if after {
			l.afterCommit = func() error { return syntheticError }
		} else {
			l.beforeCommit = func() error { return syntheticError }
		}
		if l.Commit(context.Background(), initial, next) != ErrRecovery {
			t.Fatal("uncertain rich commit did not fail closed")
		}
		if _, err := l.Load(context.Background()); err != ErrRecovery {
			t.Fatal("uncertainty did not latch")
		}
		l.Close()
		reopened, err := OpenExistingForProfile(context.Background(), dir, binding, rich)
		if err != nil {
			t.Fatal(err)
		}
		want := initial
		if after {
			want = next
		}
		if !sameRecord(load(t, reopened), want) {
			t.Fatal("wrong authoritative rich record")
		}
		reopened.Close()
	}
}
