package connectedactivation

import (
	"bytes"
	"strings"
	"testing"
)

func requestFixture() Request {
	return Request{RequestVersion, strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64), 1, 32001, 32002}
}

func responseFixture(r Request) Response {
	digest, _ := RequestDigest(r)
	return Response{ResponseVersion, digest, strings.Repeat("d", 32), strings.Repeat("e", 64), Pass}
}

func TestRoundTripAndBinding(t *testing.T) {
	r := requestFixture()
	response := responseFixture(r)
	commit := Commit{CommitVersion, response.RequestSHA256, response.InvocationID, response.Challenge}
	b, err := EncodeRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeRequest(b); err != nil || got != r {
		t.Fatal("request round trip")
	}
	b, err = EncodeResponse(response)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeResponse(b); err != nil || got != response {
		t.Fatal("response round trip")
	}
	b, err = EncodeCommit(commit)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeCommit(b); err != nil || got != commit {
		t.Fatal("commit round trip")
	}
	if !MatchResponse(r, response.InvocationID, response) || !MatchCommit(r, response, commit) {
		t.Fatal("valid binding refused")
	}
	for _, mutate := range []func(*Request){
		func(v *Request) { v.Nonce = strings.Repeat("f", 64) }, func(v *Request) { v.ArtifactSHA256 = strings.Repeat("f", 64) },
		func(v *Request) { v.ConfigSHA256 = strings.Repeat("f", 64) }, func(v *Request) { v.PolicyGeneration++ },
		func(v *Request) { v.IPv4Port++ }, func(v *Request) { v.IPv6Port++ },
	} {
		changed := r
		mutate(&changed)
		if MatchResponse(changed, response.InvocationID, response) || MatchCommit(changed, response, commit) {
			t.Fatal("stale request accepted")
		}
	}
	if MatchResponse(r, strings.Repeat("f", 32), response) {
		t.Fatal("wrong manager invocation")
	}
	for _, mutate := range []func(*Commit){
		func(v *Commit) { v.RequestSHA256 = strings.Repeat("f", 64) }, func(v *Commit) { v.InvocationID = strings.Repeat("f", 32) }, func(v *Commit) { v.Challenge = strings.Repeat("f", 64) },
	} {
		changed := commit
		mutate(&changed)
		if MatchCommit(r, response, changed) {
			t.Fatal("stale commit accepted")
		}
	}
	fresh := response
	fresh.Challenge = strings.Repeat("f", 64)
	if MatchCommit(r, fresh, commit) {
		t.Fatal("commit authorized a new process challenge")
	}
}

func TestRequestRejectsInvalidFields(t *testing.T) {
	for _, mutate := range []func(*Request){
		func(v *Request) { v.Version = "" }, func(v *Request) { v.Nonce = "" }, func(v *Request) { v.Nonce = strings.Repeat("A", 64) },
		func(v *Request) { v.ArtifactSHA256 = "bad" }, func(v *Request) { v.ConfigSHA256 = strings.Repeat("g", 64) },
		func(v *Request) { v.PolicyGeneration = 0 }, func(v *Request) { v.IPv4Port = 1023 }, func(v *Request) { v.IPv6Port = 0 },
	} {
		r := requestFixture()
		mutate(&r)
		if _, err := EncodeRequest(r); err != ErrUnsafe {
			t.Fatal("invalid request accepted")
		}
	}
	r := requestFixture()
	r.IPv4Port = 1024
	r.IPv6Port = 65535
	r.PolicyGeneration = ^uint64(0)
	if _, err := EncodeRequest(r); err != nil {
		t.Fatal("valid numeric edge refused")
	}
}

func TestResponseAndCommitRejectInvalidFields(t *testing.T) {
	for _, mutate := range []func(*Response){
		func(v *Response) { v.Version = "" }, func(v *Response) { v.RequestSHA256 = "bad" }, func(v *Response) { v.InvocationID = strings.Repeat("D", 32) },
		func(v *Response) { v.Challenge = "" }, func(v *Response) { v.Result = "FAILED: raw secret" },
	} {
		r := responseFixture(requestFixture())
		mutate(&r)
		if _, err := EncodeResponse(r); err != ErrUnsafe {
			t.Fatal("invalid response accepted")
		}
	}
	for _, mutate := range []func(*Commit){
		func(v *Commit) { v.Version = "" }, func(v *Commit) { v.RequestSHA256 = "bad" }, func(v *Commit) { v.InvocationID = "" }, func(v *Commit) { v.Challenge = "bad" },
	} {
		r := responseFixture(requestFixture())
		c := Commit{CommitVersion, r.RequestSHA256, r.InvocationID, r.Challenge}
		mutate(&c)
		if _, err := EncodeCommit(c); err != ErrUnsafe {
			t.Fatal("invalid commit accepted")
		}
	}
}

func TestDecodersRequireClosedCanonicalBoundedRecords(t *testing.T) {
	r := requestFixture()
	response := responseFixture(r)
	c := Commit{CommitVersion, response.RequestSHA256, response.InvocationID, response.Challenge}
	rb, _ := EncodeRequest(r)
	sb, _ := EncodeResponse(response)
	cb, _ := EncodeCommit(c)
	for _, test := range []struct {
		data   []byte
		decode func([]byte) error
	}{
		{rb, func(b []byte) error { _, e := DecodeRequest(b); return e }},
		{sb, func(b []byte) error { _, e := DecodeResponse(b); return e }},
		{cb, func(b []byte) error { _, e := DecodeCommit(b); return e }},
	} {
		for _, bad := range [][]byte{
			nil, []byte("null"), []byte("{}"), bytes.Repeat([]byte("x"), MaxBytes+1),
			append(append([]byte(nil), test.data...), '\n'), append(append([]byte(nil), test.data...), []byte("{}")...),
			append([]byte(`{"unknown":true,`), test.data[1:]...),
			append([]byte(`{"version":"duplicate",`), test.data[1:]...),
			bytes.Replace(test.data, []byte(`"version"`), []byte(`"Version"`), 1),
			append([]byte{0xef, 0xbb, 0xbf}, test.data...),
			append([]byte{0xff}, test.data...),
		} {
			if err := test.decode(bad); err != ErrUnsafe {
				t.Fatal("noncanonical record accepted")
			}
		}
	}
	for _, value := range []string{"null", "-1", "65536", "32001.0", "3.2001e4", "\"32001\""} {
		bad := bytes.Replace(rb, []byte(`"ipv4_port":32001`), []byte(`"ipv4_port":`+value), 1)
		if _, err := DecodeRequest(bad); err != ErrUnsafe {
			t.Fatal("unsafe port representation accepted")
		}
	}
	bad := bytes.Replace(rb, []byte(`"policy_generation":1`), []byte(`"policy_generation":18446744073709551616`), 1)
	if _, err := DecodeRequest(bad); err != ErrUnsafe {
		t.Fatal("generation overflow accepted")
	}
}

