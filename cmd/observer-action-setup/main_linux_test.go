//go:build linux

package main

import (
	"context"
	"testing"
)

func TestSetupClosedExchangeLaunch(t *testing.T) {
	for _, mode := range []string{"valid", "extra", "root", "secret", "proxy", "trust", "error"} {
		t.Run(mode, func(t *testing.T) {
			args := []string{"observer-action-setup", "exchange"}
			env := []string{"GODEBUG=netdns=go"}
			switch mode {
			case "extra":
				args = append(args, "--path=/tmp/secret")
			case "root":
				args[1] = "install"
			case "secret":
				env = append(env, "ACTION_TOKEN=synthetic-canary")
			case "proxy":
				env = append(env, "HTTPS_PROXY=https://example.invalid")
			case "trust":
				env = append(env, "SSL_CERT_FILE=/tmp/ca")
			}
			calls := 0
			code := run(context.Background(), args, env, func(context.Context) error {
				calls++
				if mode == "error" {
					return context.Canceled
				}
				return nil
			}, nil, nil)
			if mode == "valid" {
				if code != 0 || calls != 1 {
					t.Fatal("closed exchange refused")
				}
			} else if mode == "error" {
				if code != 22 || calls != 1 {
					t.Fatal("failure escaped")
				}
			} else if code != 22 || calls != 0 {
				t.Fatal("override reached input/secret consumer")
			}
		})
	}
}

func TestSetupProvisionDispatch(t *testing.T) {
	for _, verb := range []string{"provision-journal", "install"} {
		t.Run(verb, func(t *testing.T) {
			calls := 0
			code := run(context.Background(), []string{"observer-action-setup", verb}, []string{"GODEBUG=netdns=go"},
				func(context.Context) error { t.Fatal("unexpected exchange"); return nil },
				func(context.Context) error { calls++; return nil }, nil)
			if verb == "provision-journal" {
				if code != 0 || calls != 1 {
					t.Fatal("closed provisioning command was not dispatched")
				}
			} else if code != 22 || calls != 0 {
				t.Fatal("unsupported command reached provisioning")
			}
		})
	}
}
