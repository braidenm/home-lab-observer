//go:build linux

package main

import (
	"context"
	"errors"
	"testing"
)

func TestFixedLaunch(t *testing.T) {
	for _, mode := range []string{"valid", "argument", "missing-name", "proxy", "trust", "docker", "secret", "duplicate", "missing-godebug", "debug", "worker-error"} {
		t.Run(mode, func(t *testing.T) {
			args := []string{"observer-action-worker"}
			env := []string{"GODEBUG=netdns=go", "INVOCATION_ID=synthetic"}
			switch mode {
			case "argument":
				args = append(args, "--accept-any-owner")
			case "missing-name":
				args = nil
			case "proxy":
				env = append(env, "HTTPS_PROXY=https://example.invalid")
			case "trust":
				env = append(env, "SSL_CERT_FILE=/tmp/not-trusted")
			case "docker":
				env = append(env, "DOCKER_HOST=tcp://example.invalid:2375")
			case "secret":
				env = append(env, "ACTION_TOKEN=secret-canary")
			case "duplicate":
				env = append(env, "GODEBUG=netdns=go")
			case "missing-godebug":
				env = nil
			case "debug":
				env[0] = "GODEBUG=http2debug=2"
			}
			calls := 0
			code := run(context.Background(), args, env, func(context.Context) error {
				calls++
				if mode == "worker-error" {
					return errors.New("private-rejected-value-canary")
				}
				return nil
			})
			if mode == "valid" {
				if code != 0 || calls != 1 {
					t.Fatal("valid fixed launch refused")
				}
			} else if mode == "worker-error" {
				if code != 22 || calls != 1 {
					t.Fatal("unsafe profile accepted")
				}
			} else if code != 22 || calls != 0 {
				t.Fatal("override reached worker")
			}
		})
	}
}

func TestFixedLaunchPreservesCancellationAndInertManagerMetadata(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	code := run(ctx, []string{"observer-action-worker"}, []string{"GODEBUG=netdns=go", "CREDENTIALS_DIRECTORY=/not/a/credential/source"}, func(got context.Context) error {
		called = true
		if got != ctx || got.Err() == nil {
			t.Fatal("launch replaced cancellation scope")
		}
		return nil
	})
	if code != 0 || !called {
		t.Fatal("fixed context not consumed")
	}
	if run(nil, []string{"observer-action-worker"}, []string{"GODEBUG=netdns=go"}, func(context.Context) error { return nil }) != 22 || run(ctx, []string{"observer-action-worker"}, []string{"GODEBUG=netdns=go"}, nil) != 22 {
		t.Fatal("missing boundary accepted")
	}
}
