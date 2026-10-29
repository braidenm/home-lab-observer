package connectedprofile

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestDockerReaderStartupConfigurationHasClosedRoleDomain(t *testing.T) {
	c := dockerConfigFixture(t)
	raw, err := EncodeRichProcessesDockerConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := EncodeRichProcessesDockerReaderStartup(c)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte(`{"version":"observer-connected-docker-reader-startup/v1","role":"rich-docker-reader","configuration":`), raw...)
	want = append(want, '}')
	if !bytes.Equal(got, want) {
		t.Fatal("reader startup envelope is not exact canonical config with fixed role/version")
	}
	for _, foreign := range [][]byte{raw, bytes.Replace(got, []byte(`"role":"rich-docker-reader"`), []byte(`"role":"rich-docker-uploader"`), 1), bytes.Replace(got, []byte("reader-startup/v1"), []byte("reader-startup/v2"), 1)} {
		if sha256.Sum256(foreign) == sha256.Sum256(got) {
			t.Fatal("reader startup approval aliases another configuration domain")
		}
	}
	got[0] = 'x'
	again, err := EncodeRichProcessesDockerReaderStartup(c)
	if err != nil || !bytes.Equal(again, want) {
		t.Fatal("returned startup bytes alias mutable config storage")
	}
	c.Profile = RichProcessesProfile
	if b, err := EncodeRichProcessesDockerReaderStartup(c); err != ErrUnsafe || b != nil {
		t.Fatal("foreign profile encoded as reader startup authority")
	}
}
