package actionworker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actionremote"
)

func renewalStateFixture(t *testing.T) (actionremote.Binding, handoffDocument, activeRecord) {
	t.Helper()
	s, _ := decodeSettings([]byte(settingsFixture))
	a, err := decodeActiveRecord([]byte(activeFixture()), s.Binding)
	if err != nil {
		t.Fatal("old active fixture")
	}
	h, err := readyHandoff(s.Binding, a)
	if err != nil {
		t.Fatal("ready fixture")
	}
	h.Phase = phasePrepared
	h.PredecessorSecret = &a.AuthorizationSecret
	h.PredecessorExpiresAt = &a.ExpiresAt
	h.Candidate = &candidateRecord{CredentialID: "hlac_55555555555555555555555555555555", Revision: "2", State: actionremote.CredentialPrepared,
		ExpiresAt: "2030-01-02T03:14:05.000000Z", AuthorizationSecret: "hlac_55555555555555555555555555555555.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	return s.Binding, h, a
}

func TestCredentialHandoffCanonicalRecordsAndPredecessorHash(t *testing.T) {
	b, h, a := renewalStateFixture(t)
	raw, err := encodeHandoff(h, b)
	if err != nil || len(raw) > 4096 {
		t.Fatal("canonical prepared state")
	}
	want := sha256.Sum256([]byte(activeFixture()))
	if *h.ActiveSHA256 != hex.EncodeToString(want[:]) {
		t.Fatal("old hash not actual nine-field bytes")
	}
	decoded, err := decodeHandoff(raw, b)
	if err != nil || decoded.Phase != phasePrepared || validateActiveWitness(decoded, []byte(activeFixture()), b) != nil {
		t.Fatal("stored prepared interpretation")
	}
	if restored, err := predecessorRecord(decoded, b); err != nil || restored != a {
		t.Fatal("predecessor secret/expiry unavailable after active rename")
	}
	for _, changed := range []string{
		strings.Replace(string(raw), `"phase":"PREPARED"`, `"phase":"READY"`, 1),
		strings.Replace(string(raw), `"phase":`, `"phase":"PREPARED","phase":`, 1),
		strings.Replace(string(raw), `"phase":`, `"Phase":`, 1),
		strings.Replace(string(raw), `"phase":`, `"unknown":0,"phase":`, 1),
		strings.Replace(string(raw), `"revision":"2"`, `"revision":2`, 1),
		string(raw) + `{}`, " " + string(raw),
	} {
		if _, err := decodeHandoff([]byte(changed), b); err == nil {
			t.Fatal("noncanonical/foreign phase admitted")
		}
	}
	corrupt := h
	wrong := strings.Replace(*h.PredecessorSecret, ".A", ".B", 1)
	corrupt.PredecessorSecret = &wrong
	if _, err := encodeHandoff(corrupt, b); err == nil {
		t.Fatal("retained old secret not bound to old hash")
	}
	if strings.Contains(fmt.Sprintf("%v %#v", h, h), "AAAAAAAA") || strings.Contains(fmt.Sprintf("%v %#v", a, a), "AAAAAAAA") {
		t.Fatal("file secret formatting")
	}
}

func TestCredentialHandoffFilesystemWitnessMatrixAfterActiveRename(t *testing.T) {
	b, h, _ := renewalStateFixture(t)
	if validateActiveWitness(h, nil, b) == nil {
		t.Fatal("missing renewal active file adopted")
	}
	h.Phase = phaseActivateAttempted
	prepared := *h.Candidate
	newActive := prepared
	newActive.State = actionremote.CredentialActive
	newActive.ExpiresAt = "2030-02-01T03:04:05.000000Z"
	newBytes, err := candidateActiveBytes(newActive, b)
	if err != nil {
		t.Fatal("successor record")
	}
	if validateActiveWitness(h, newBytes, b) == nil {
		t.Fatal("rename preceded durable active candidate")
	}
	h.Candidate = &newActive
	for _, raw := range [][]byte{[]byte(activeFixture()), newBytes} {
		if validateActiveWitness(h, raw, b) != nil {
			t.Fatal("valid old/new interrupted promotion refused")
		}
	}
	if validateActiveWitness(h, nil, b) == nil {
		t.Fatal("renewal missing file bypass")
	}
	unknown := strings.Replace(string(newBytes), `"credential_revision":"2"`, `"credential_revision":"3"`, 1)
	if validateActiveWitness(h, []byte(unknown), b) == nil {
		t.Fatal("unrelated successor adopted")
	}
	first := freshHandoff(b)
	if validateActiveWitness(first, nil, b) != nil || validateActiveWitness(first, []byte(activeFixture()), b) == nil {
		t.Fatal("unenrolled absence boundary")
	}
	first.Phase = phaseExchangeAttempted
	first.Candidate = &newActive
	if validateActiveWitness(first, nil, b) != nil || validateActiveWitness(first, newBytes, b) != nil {
		t.Fatal("first exchange interrupted promotion")
	}
}

func TestCredentialDisclosureTranslationRetainsActualExpiryWithoutRounding(t *testing.T) {
	d := actionremote.CredentialDisclosure{AuthorizationSecret: "hlac_55555555555555555555555555555555.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Metadata: actionremote.CredentialMetadata{CredentialID: "hlac_55555555555555555555555555555555", Revision: 2, State: actionremote.CredentialPrepared,
			ExpiresAt: time.Date(2030, 1, 2, 3, 14, 5, 123456000, time.UTC)}}
	c, err := candidateFromDisclosure(d)
	if err != nil || c.Revision != "2" || c.ExpiresAt != "2030-01-02T03:14:05.123456Z" {
		t.Fatal("lossless local translation")
	}
	d.Metadata.ExpiresAt = d.Metadata.ExpiresAt.Add(time.Nanosecond)
	if _, err := candidateFromDisclosure(d); err == nil {
		t.Fatal("submicro value silently rounded")
	}
}
