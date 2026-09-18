package remoteprojection

import (
	"bytes"
	"testing"
)

func TestUploadDispatchIsClosedCanonicalAndBound(t *testing.T) {
	raw, id := fixture()
	for _, encode := range []func() ([]byte, error){
		func() ([]byte, error) { return Encode(raw, id) },
		func() ([]byte, error) { return EncodeNative(raw, id) },
	} {
		body, err := encode()
		if err != nil {
			t.Fatal(err)
		}
		at, err := UploadCollectionTime(body, id.SourceID)
		if err != nil || !at.Equal(raw.ObservedAt) || ValidateUpload(body, id.SourceID) != nil {
			t.Fatal("valid profile not admitted")
		}
		for _, bad := range [][]byte{
			append(bytes.Clone(body), '\n'),
			bytes.Replace(body, []byte(`"schema_version":`), []byte(`"schema_version":"unknown","schema_version":`), 1),
			bytes.Replace(body, []byte(`"source_id":`), []byte(`"excluded":"private-canary","source_id":`), 1),
			bytes.Replace(body, []byte(id.SourceID), []byte("srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), 1),
		} {
			if at, err := UploadCollectionTime(bad, id.SourceID); err != ErrValidation || !at.IsZero() {
				t.Fatal("malformed or mismatched profile exposed collection time")
			}
		}
	}
	for _, bad := range [][]byte{nil, []byte(`{"schema_version":"future/v1"}`), bytes.Repeat([]byte{'x'}, MaxBytes+1)} {
		if ValidateUpload(bad, id.SourceID) != ErrValidation {
			t.Fatal("unknown or oversized profile admitted")
		}
	}
}
