//go:build linux

package connectedinstall

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedenroll"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
)

func TestEnrollmentExistingInputRejectsBeforeManager(t *testing.T) {
	valid := connectedenroll.ValidationInput{ServerID: "srv_11111111111111111111111111111111", ConnectorID: "agent_22222222222222222222222222222222", UploaderUID: 1201, UploaderGID: 1202, SharedGID: 1203}
	policy := connectedunits.EnrollmentInput{UploaderUID: valid.UploaderUID, UploaderGID: valid.UploaderGID, SharedGID: valid.SharedGID}
	encode := func(v connectedenroll.ValidationInput) []byte { b, _ := json.Marshal(v); return b }
	canonical := encode(valid)
	cases := map[string][]byte{
		"malformed": []byte("{"), "noncanonical": append(append([]byte{}, canonical...), '\n'),
		"unknown-field":   []byte(strings.Replace(string(canonical), "}", ",\"extra\":true}", 1)),
		"duplicate-field": []byte(strings.Replace(string(canonical), "}", ",\"shared_gid\":1203}", 1)),
	}
	for name, mutate := range map[string]func(*connectedenroll.ValidationInput){
		"wrong-uid":         func(v *connectedenroll.ValidationInput) { v.UploaderUID++ },
		"wrong-gid":         func(v *connectedenroll.ValidationInput) { v.UploaderGID++ },
		"wrong-shared":      func(v *connectedenroll.ValidationInput) { v.SharedGID++ },
		"missing-server":    func(v *connectedenroll.ValidationInput) { v.ServerID = "" },
		"invalid-connector": func(v *connectedenroll.ValidationInput) { v.ConnectorID = "bad\nidentifier" },
	} {
		v := valid
		mutate(&v)
		cases[name] = encode(v)
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			called := false
			output, err := runEnrollmentWithPreflight(context.Background(), nil, "/bin/observer-connected-uploader", "validate-existing-ledger", input, policy, func(context.Context) (map[string]string, error) { called = true; return nil, ErrUnsafe })
			if called || err != ErrUnsafe || output != nil {
				t.Fatal("invalid request reached manager or was not refused")
			}
		})
	}
	called := false
	_, err := runEnrollmentWithPreflight(context.Background(), nil, "/bin/observer-connected-uploader", "validate-existing-ledger", canonical, policy, func(context.Context) (map[string]string, error) { called = true; return nil, ErrUnsafe })
	if !called || err != ErrUnsafe {
		t.Fatal("valid request did not reach the fixed preflight boundary")
	}
}
