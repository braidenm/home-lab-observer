//go:build linux

package actionworker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionremote"
	"golang.org/x/sys/unix"
)

type lifecycleRemoteFixture struct {
	prepared, active candidateRecord
	old              activeRecord
	calls            []string
	fail             string
	activated        bool
	oldStillActive   bool
	afterCall        func(string)
}

func (r *lifecycleRemoteFixture) called(call string) {
	r.calls = append(r.calls, call)
	if r.afterCall != nil {
		r.afterCall(call)
	}
}

func metadataFixture(c candidateRecord) actionremote.CredentialMetadata {
	n, _ := strconv.ParseInt(c.Revision, 10, 64)
	at, _ := credentialExpiry(c.ExpiresAt)
	return actionremote.CredentialMetadata{CredentialID: c.CredentialID, Revision: n, State: c.State, ExpiresAt: at}
}
func (r *lifecycleRemoteFixture) Exchange(context.Context, actionremote.EnrollmentSecret) (actionremote.CredentialDisclosure, error) {
	r.called("exchange")
	if r.fail == "exchange" {
		return actionremote.CredentialDisclosure{}, actionremote.ErrUnavailable
	}
	return actionremote.CredentialDisclosure{AuthorizationSecret: r.active.AuthorizationSecret, Metadata: metadataFixture(r.active)}, nil
}
func (r *lifecycleRemoteFixture) Prepare(context.Context, actionremote.Credential) (actionremote.CredentialDisclosure, error) {
	r.called("prepare")
	if r.fail == "prepare" {
		return actionremote.CredentialDisclosure{}, actionremote.ErrUnavailable
	}
	return actionremote.CredentialDisclosure{AuthorizationSecret: r.prepared.AuthorizationSecret, Metadata: metadataFixture(r.prepared)}, nil
}
func (r *lifecycleRemoteFixture) Activate(context.Context, actionremote.Credential) (actionremote.CredentialMetadata, error) {
	r.called("activate")
	r.activated = true
	if r.fail == "activate" {
		return actionremote.CredentialMetadata{}, actionremote.ErrUnavailable
	}
	return metadataFixture(r.active), nil
}
func (r *lifecycleRemoteFixture) Status(_ context.Context, c actionremote.Credential) (actionremote.CredentialMetadata, error) {
	old, _ := actionremote.NewCredential(r.old.AuthorizationSecret)
	if c == old {
		r.called("old-status")
		if r.fail == "old-status" {
			return actionremote.CredentialMetadata{}, actionremote.ErrUnavailable
		}
		if r.activated && !r.oldStillActive {
			return actionremote.CredentialMetadata{State: actionremote.CredentialInactive}, nil
		}
		return metadataFixture(activeCandidate(r.old)), nil
	}
	r.called("new-status")
	if r.fail == "new-status" {
		return actionremote.CredentialMetadata{}, actionremote.ErrUnavailable
	}
	if r.activated {
		return metadataFixture(r.active), nil
	}
	return metadataFixture(r.prepared), nil
}
func lifecycleFixture(t *testing.T, phase handoffPhase) (*credentialLifecycle, *lifecycleRemoteFixture, string) {
	t.Helper()
	s, root := credentialStoreFixture(t)
	s.close()
	b, h, a := renewalStateFixture(t)
	// Real current time, not a raw arbitrary maximum sizing vector.
	a.ExpiresAt = time.Now().UTC().Add(24 * time.Hour).Truncate(time.Microsecond).Format(credentialTime)
	h, _ = readyHandoff(b, a)
	prepared := candidateRecord{"hlac_55555555555555555555555555555555", "2", actionremote.CredentialPrepared, time.Now().UTC().Add(10 * time.Minute).Truncate(time.Microsecond).Format(credentialTime), "hlac_55555555555555555555555555555555.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	active := prepared
	active.State = actionremote.CredentialActive
	active.ExpiresAt = time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Microsecond).Format(credentialTime)
	h.Phase = phase
	if phase != phaseReady {
		h.PredecessorSecret = &a.AuthorizationSecret
		h.PredecessorExpiresAt = &a.ExpiresAt
	}
	if phase == phasePrepared || phase == phaseActivateAttempted {
		h.Candidate = &prepared
	}
	raw, _ := encodeHandoff(h, b)
	ar, _ := json.Marshal(a)
	if os.WriteFile(filepath.Join(root, "worker", "handoff.json"), raw, 0600) != nil || os.WriteFile(filepath.Join(root, "worker", "active.json"), ar, 0600) != nil {
		t.Fatal("renewal fixture")
	}
	fd, _ := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	s, err := openCredentialStoreAt(context.Background(), fd, []protectedPart{{"worker", uint32(os.Geteuid()), uint32(os.Getegid()), 0700}}, b)
	if err != nil {
		t.Fatal("renewal store")
	}
	t.Cleanup(s.close)
	r := &lifecycleRemoteFixture{prepared: prepared, active: active, old: a}
	return &credentialLifecycle{store: s, remote: r, binding: b}, r, root
}
func TestCredentialLifecycleQuietRenewalConsumesActualStore(t *testing.T) {
	l, r, _ := lifecycleFixture(t, phaseReady)
	if l.maintain(context.Background()) != nil {
		t.Fatal("renewal")
	}
	if got := len(r.calls); got != 4 || r.calls[0] != "prepare" || r.calls[1] != "new-status" || r.calls[2] != "old-status" || r.calls[3] != "activate" {
		t.Fatal("wrong lifecycle order/retry")
	}
	h, raw, err := l.store.load(context.Background())
	defer clear(raw)
	if err != nil || h.Phase != phaseReady || h.PredecessorSecret != nil || *h.ActiveCredentialID != r.active.CredentialID {
		t.Fatal("new READY not durable")
	}
	if l.maintain(context.Background()) != nil || len(r.calls) != 4 {
		t.Fatal("renewal repeated outside lead")
	}
	if _, err := l.Credential(context.Background(), l.binding); err != nil {
		t.Fatal("actual READY provider unavailable")
	}
}
func TestCredentialLifecycleAttemptFailureNeverRetriesOrClaims(t *testing.T) {
	for _, fail := range []string{"prepare", "activate", "new-status", "old-status"} {
		t.Run(fail, func(t *testing.T) {
			l, r, _ := lifecycleFixture(t, phaseReady)
			r.fail = fail
			if l.maintain(context.Background()) == nil {
				t.Fatal("uncertain lifecycle accepted")
			}
			if _, err := l.Credential(context.Background(), l.binding); err == nil {
				t.Fatal("claim credential from incomplete phase")
			}
			before := len(r.calls)
			r.fail = ""
			if fail == "prepare" {
				if l.maintain(context.Background()) == nil || len(r.calls) != before {
					t.Fatal("attempted prepare retried")
				}
			} else if fail == "activate" {
				if l.maintain(context.Background()) != nil {
					t.Fatal("positive status resolution")
				}
				for _, call := range r.calls[before:] {
					if call != "new-status" && call != "old-status" {
						t.Fatal("attempted activation repeated")
					}
				}
			}
		})
	}
}
func TestCredentialLifecycleMarkerFailureSendsZeroPOST(t *testing.T) {
	l, r, _ := lifecycleFixture(t, phaseReady)
	l.store.fault = func(stage string) error {
		if stage == "after-file-sync" {
			return ErrUnsafe
		}
		return nil
	}
	if l.maintain(context.Background()) == nil || len(r.calls) != 0 {
		t.Fatal("POST before durable marker")
	}
}
func TestCredentialLifecyclePostRenameRestartUsesRetainedOldSecret(t *testing.T) {
	l, r, root := lifecycleFixture(t, phaseActivateAttempted)
	r.activated = true
	// The ACTIVATE reply was lost; recovery first proves new ACTIVE/old INACTIVE.
	count := 0
	l.store.fault = func(stage string) error {
		if stage == "after-rename" {
			count++
			if count == 2 {
				return ErrUnsafe
			}
		}
		return nil
	}
	if l.maintain(context.Background()) == nil {
		t.Fatal("post active rename ambiguity hidden")
	}
	l.store.close()
	fd, _ := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	s, err := openCredentialStoreAt(context.Background(), fd, []protectedPart{{"worker", uint32(os.Geteuid()), uint32(os.Getegid()), 0700}}, l.binding)
	if err != nil {
		t.Fatal("restart store")
	}
	defer s.close()
	l.store = s
	r.calls = nil
	if l.maintain(context.Background()) != nil || len(r.calls) != 2 || r.calls[0] != "new-status" || r.calls[1] != "old-status" {
		t.Fatal("restart did not use positive new/old proof")
	}
}

func TestCredentialLifecycleEachDurabilityBoundaryStopsTheNextPOST(t *testing.T) {
	for stop := 1; stop <= 6; stop++ {
		t.Run(strconv.Itoa(stop), func(t *testing.T) {
			l, r, _ := lifecycleFixture(t, phaseReady)
			writes := 0
			l.store.fault = func(stage string) error {
				if stage == "before-rename" {
					writes++
					if writes == stop {
						return ErrUnsafe
					}
				}
				return nil
			}
			if l.maintain(context.Background()) == nil {
				t.Fatal("injected durability failure hidden")
			}
			prepare, activate := 0, 0
			for _, call := range r.calls {
				if call == "prepare" {
					prepare++
				}
				if call == "activate" {
					activate++
				}
			}
			if prepare > 1 || activate > 1 || stop == 1 && prepare != 0 || stop <= 3 && activate != 0 {
				t.Fatal("POST preceded required durable phase or retried")
			}
			if _, err := l.Credential(context.Background(), l.binding); err == nil {
				t.Fatal("poisoned failed lifecycle supplied execution credential")
			}
		})
	}
}

func TestCredentialLifecycleBootstrapAndLostDisclosureAreNotReexchangeable(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(strconv.FormatBool(lost), func(t *testing.T) {
			s, _ := credentialStoreFixture(t)
			_, h, a := renewalStateFixture(t)
			candidate := *h.Candidate
			candidate.State = actionremote.CredentialActive
			candidate.ExpiresAt = time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Microsecond).Format(credentialTime)
			r := &lifecycleRemoteFixture{old: a, active: candidate}
			if lost {
				r.fail = "exchange"
			}
			l := &credentialLifecycle{store: s, remote: r, binding: s.binding}
			secret, err := actionremote.NewEnrollmentSecret("hlae_11111111111111111111111111111111.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
			if err != nil {
				t.Fatal("synthetic enrollment")
			}
			op, _ := newCredentialOperation(context.Background())
			err = l.exchange(op, secret)
			op.close()
			if (err == nil) == lost || len(r.calls) != 1 || r.calls[0] != "exchange" {
				t.Fatal("bootstrap disclosure path")
			}
			op, _ = newCredentialOperation(context.Background())
			defer op.close()
			if l.exchange(op, secret) == nil || len(r.calls) != 1 {
				t.Fatal("reexchange/re-enrollment admitted")
			}
			if lost {
				if _, err := l.Credential(context.Background(), s.binding); err == nil {
					t.Fatal("lost disclosure fabricated a credential")
				}
			}
		})
	}
}

func TestCredentialLifecycleRejectsStalePreparedAndUnavailableStatus(t *testing.T) {
	for _, mode := range []string{"prepared-after-attempt", "old-still-active", "unavailable", "changed-revision", "expired-new"} {
		t.Run(mode, func(t *testing.T) {
			l, r, _ := lifecycleFixture(t, phaseActivateAttempted)
			r.activated = mode != "prepared-after-attempt"
			switch mode {
			case "old-still-active":
				r.oldStillActive = true
			case "unavailable":
				r.fail = "new-status"
			case "changed-revision":
				r.active.Revision = "3"
			case "expired-new":
				r.active.ExpiresAt = time.Now().UTC().Add(-time.Second).Truncate(time.Microsecond).Format(credentialTime)
			}
			if l.maintain(context.Background()) == nil {
				t.Fatal("contradictory/unavailable status promoted")
			}
			for _, call := range r.calls {
				if call != "new-status" && call != "old-status" {
					t.Fatal("status uncertainty caused POST")
				}
			}
		})
	}
}

func TestCredentialLifecycleNativeFiveSecondLegWithinOriginalThirty(t *testing.T) {
	for _, call := range []string{"exchange", "prepare", "activate", "new-status"} {
		for _, delay := range []time.Duration{4 * time.Second, 5 * time.Second} {
			t.Run(call+"/"+delay.String(), func(t *testing.T) {
				phase := phaseReady
				if call == "activate" {
					phase = phasePrepared
				} else if call == "new-status" {
					phase = phaseActivateAttempted
				}
				l, r, _ := lifecycleFixture(t, phase)
				if call == "new-status" {
					r.activated = true
				}
				if call == "exchange" {
					s, _ := credentialStoreFixture(t)
					l = &credentialLifecycle{store: s, remote: r, binding: s.binding}
				}
				op, err := newCredentialOperation(context.Background())
				if err != nil {
					t.Fatal("original operation")
				}
				defer op.close()
				advanced := time.Duration(0)
				op.capture = func() (nativeStamp, error) {
					now := op.start
					now.at = now.at.Add(advanced)
					now.boot += advanced
					return now, nil
				}
				r.afterCall = func(current string) {
					if current == call {
						advanced = delay
					}
				}
				if call == "exchange" {
					secret, _ := actionremote.NewEnrollmentSecret("hlae_11111111111111111111111111111111.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
					err = l.exchange(op, secret)
				} else {
					err = l.maintainOperation(op)
				}
				if op.check() != nil {
					t.Fatal("fixture exceeded original30 rather than selected5-second leg")
				}
				if delay < 5*time.Second {
					if err != nil {
						t.Fatal("valid measured leg refused")
					}
				} else {
					if err == nil {
						t.Fatal("native five-second HTTP leg accepted inside original thirty")
					}
					h, raw, loadErr := l.store.load(context.Background())
					clear(raw)
					if loadErr != nil || h.Phase == phaseReady {
						t.Fatal("late leg lost attempted evidence or published READY")
					}
					if _, err := l.Credential(context.Background(), l.binding); err == nil {
						t.Fatal("late leg supplied a claim credential")
					}
				}
			})
		}
	}
}
