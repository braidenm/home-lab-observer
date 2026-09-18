// Package connectedactivation defines a bounded volatile startup rendezvous.
// These records are not durable activation authority or release acceptance.
// Filesystem ownership, deadlines, policy checks and fresh random challenges
// belong to the root transaction and the same paused worker invocation.
package connectedactivation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

const (
	MaxBytes        = 2048
	RequestVersion  = "observer-connected-activation-request/v1"
	ResponseVersion = "observer-connected-activation-response/v1"
	CommitVersion   = "observer-connected-activation-commit/v1"
	Pass            = "PASS"
)

var ErrUnsafe = errors.New("connected_activation_refused")

type Request struct {
	Version          string `json:"version"`
	Nonce            string `json:"nonce"`
	ArtifactSHA256   string `json:"artifact_sha256"`
	ConfigSHA256     string `json:"config_sha256"`
	PolicyGeneration uint64 `json:"policy_generation"`
	IPv4Port         uint16 `json:"ipv4_port"`
	IPv6Port         uint16 `json:"ipv6_port"`
}

type Response struct {
	Version       string `json:"version"`
	RequestSHA256 string `json:"request_sha256"`
	InvocationID  string `json:"invocation_id"`
	Challenge     string `json:"challenge"`
	Result        string `json:"result"`
}

type Commit struct {
	Version       string `json:"version"`
	RequestSHA256 string `json:"request_sha256"`
	InvocationID  string `json:"invocation_id"`
	Challenge     string `json:"challenge"`
}

func validHex(s string, length int) bool {
	if len(s) != length {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func EncodeRequest(r Request) ([]byte, error) {
	if r.Version != RequestVersion || !validHex(r.Nonce, 64) || !validHex(r.ArtifactSHA256, 64) || !validHex(r.ConfigSHA256, 64) || r.PolicyGeneration == 0 || r.IPv4Port < 1024 || r.IPv6Port < 1024 {
		return nil, ErrUnsafe
	}
	return encode(r)
}

func EncodeResponse(r Response) ([]byte, error) {
	if r.Version != ResponseVersion || !validHex(r.RequestSHA256, 64) || !validHex(r.InvocationID, 32) || !validHex(r.Challenge, 64) || r.Result != Pass {
		return nil, ErrUnsafe
	}
	return encode(r)
}

func EncodeCommit(c Commit) ([]byte, error) {
	if c.Version != CommitVersion || !validHex(c.RequestSHA256, 64) || !validHex(c.InvocationID, 32) || !validHex(c.Challenge, 64) {
		return nil, ErrUnsafe
	}
	return encode(c)
}

func encode(value any) ([]byte, error) {
	b, err := json.Marshal(value)
	if err != nil || len(b) > MaxBytes {
		return nil, ErrUnsafe
	}
	return b, nil
}

func DecodeRequest(data []byte) (Request, error) {
	var r Request
	if len(data) == 0 || len(data) > MaxBytes || json.Unmarshal(data, &r) != nil {
		return Request{}, ErrUnsafe
	}
	b, err := EncodeRequest(r)
	if err != nil || !bytes.Equal(b, data) {
		return Request{}, ErrUnsafe
	}
	return r, nil
}

func DecodeResponse(data []byte) (Response, error) {
	var r Response
	if len(data) == 0 || len(data) > MaxBytes || json.Unmarshal(data, &r) != nil {
		return Response{}, ErrUnsafe
	}
	b, err := EncodeResponse(r)
	if err != nil || !bytes.Equal(b, data) {
		return Response{}, ErrUnsafe
	}
	return r, nil
}

func DecodeCommit(data []byte) (Commit, error) {
	var c Commit
	if len(data) == 0 || len(data) > MaxBytes || json.Unmarshal(data, &c) != nil {
		return Commit{}, ErrUnsafe
	}
	b, err := EncodeCommit(c)
	if err != nil || !bytes.Equal(b, data) {
		return Commit{}, ErrUnsafe
	}
	return c, nil
}

func RequestDigest(r Request) (string, error) {
	b, err := EncodeRequest(r)
	if err != nil {
		return "", ErrUnsafe
	}
	digest := sha256.Sum256(b)
	return hex.EncodeToString(digest[:]), nil
}

// MatchResponse requires the manager's independently obtained invocation ID.
// It does not verify that probes ran or that their process is still alive.
func MatchResponse(r Request, expectedInvocation string, response Response) bool {
	digest, err := RequestDigest(r)
	if err != nil || !validHex(expectedInvocation, 32) {
		return false
	}
	if _, err := EncodeResponse(response); err != nil {
		return false
	}
	return response.RequestSHA256 == digest && response.InvocationID == expectedInvocation
}

// MatchCommit is for the paused worker with its OWN retained startup response
// and fresh random challenge, never a response reloaded from an old status file.
// The caller must enforce its monotonic deadline and consume success once only.
func MatchCommit(r Request, retained Response, commit Commit) bool {
	if !MatchResponse(r, retained.InvocationID, retained) {
		return false
	}
	if _, err := EncodeCommit(commit); err != nil {
		return false
	}
	return commit.RequestSHA256 == retained.RequestSHA256 && commit.InvocationID == retained.InvocationID && commit.Challenge == retained.Challenge
}

