//go:build linux

package main

import (
	"context"
	"errors"
	"testing"
)

func TestBrokerLaunchConsumesOnlyOriginalContextAndClosedEnvironment(t *testing.T) {
	for _, scenario := range []string{"valid", "argument", "missing-name", "override", "runtime-error", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			args := []string{"observer-action-broker"}
			environ := []string{"GODEBUG=netdns=go", "CREDENTIALS_DIRECTORY=/inert/manager/metadata"}
			switch scenario {
			case "argument":
				args = append(args, "--provider=unapproved")
			case "missing-name":
				args = nil
			case "override":
				environ = append(environ, "DOCKER_HOST=tcp://example.invalid:2375")
			case "canceled":
				cancel()
			}
			calls := 0
			code := run(ctx, args, environ, func(got context.Context) error {
				calls++
				if got != ctx || scenario == "canceled" && got.Err() == nil {
					t.Fatal("original cancellation scope replaced")
				}
				if scenario == "runtime-error" {
					return errors.New("private-failure-canary")
				}
				return nil
			})
			valid := scenario == "valid" || scenario == "canceled"
			if valid && (code != 0 || calls != 1) {
				t.Fatal("valid fixed launch refused")
			}
			if scenario == "runtime-error" && (code != 22 || calls != 1) {
				t.Fatal("runtime failure ignored or retried")
			}
			if !valid && scenario != "runtime-error" && (code != 22 || calls != 0) {
				t.Fatal("override reached broker")
			}
		})
	}
}

func TestBrokerLaunchRejectsMissingBoundary(t *testing.T) {
	args, env := []string{"observer-action-broker"}, []string{"GODEBUG=netdns=go"}
	if run(nil, args, env, func(context.Context) error { return nil }) != 22 || run(context.Background(), args, env, nil) != 22 {
		t.Fatal("missing process boundary accepted")
	}
}
