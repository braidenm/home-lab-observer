package uploadstate

import (
	"crypto/sha256"
	"testing"
)

func TestPublicRecordValidationAndDetachedClone(t *testing.T) {
	body := bodyAt(t, testNow)
	r := Record{Binding: testBinding, Watermark: 1, Pending: &Pending{
		Sequence: 1, Body: body, Digest: sha256.Sum256(body), CollectedAt: testNow,
	}}
	if ValidateRecord(r, testBinding) != nil || ValidateRecord(Record{Binding: testBinding}, testBinding) != nil {
		t.Fatal("valid record rejected")
	}
	for _, tc := range []struct {
		name    string
		r       Record
		binding Binding
	}{
		{"empty expected", Record{}, Binding{}},
		{"invalid expected", Record{Binding: Binding{"private", "private"}}, Binding{"private", "private"}},
		{"wrong binding", r, Binding{testBinding.ServerID, "agent_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
		{"negative watermark", Record{Binding: testBinding, Watermark: -1}, testBinding},
		{"invalid terminal", Record{Binding: testBinding, Stopped: SourceUnavailable}, testBinding},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if ValidateRecord(tc.r, tc.binding) != ErrRecovery {
				t.Fatal("invalid record not rejected with fixed error")
			}
		})
	}
	copy := CloneRecord(r)
	copy.Pending.Body[0] = '!'
	copy.Pending.Sequence = 2
	if r.Pending.Body[0] == '!' || r.Pending.Sequence != 1 {
		t.Fatal("clone aliases original")
	}
	if ValidateRecord(copy, testBinding) != ErrRecovery {
		t.Fatal("mutated clone accepted")
	}
	copy = CloneRecord(r)
	r.Pending.Body[0] = '!'
	if copy.Pending.Body[0] == '!' {
		t.Fatal("original aliases clone")
	}
	if ValidateRecord(copy, testBinding) != nil {
		t.Fatal("detached clone invalid")
	}
	if CloneRecord(Record{Binding: testBinding}).Pending != nil {
		t.Fatal("clone invents pending state")
	}
}

func TestSourceUnavailableRecoversWithoutLatchingOrSending(t *testing.T) {
	h := newHarness(t)
	h.s.err = privateError
	step(t, h, SourceUnavailable, nil)
	step(t, h, SourceUnavailable, nil)
	if h.l.commits != 0 || len(h.tr.requests) != 0 {
		t.Fatal("failed source changed durable state")
	}
	h.s.err = nil
	step(t, h, Acknowledged, nil)
	step(t, h, Idle, nil)
}
