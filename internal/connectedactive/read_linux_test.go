//go:build linux

package connectedactive

import (
	"bytes"
	"errors"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedresources"
	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
)

func TestReadOnlyFixedActiveInventory(t *testing.T) {
	seen := make(map[string]bool)
	reader := func(path string, limit int64) ([]byte, error) {
		if seen[path] {
			t.Fatal("resource path read twice")
		}
		seen[path] = true
		for _, item := range fixed {
			if path == item.path {
				if limit != item.limit {
					t.Fatal("resource read limit drift")
				}
				return []byte(path), nil
			}
		}
		t.Fatal("unreviewed resource path")
		return nil, errors.New("unreachable")
	}
	got, err := readWith(reader)
	if err != nil || len(seen) != len(fixed) || string(got.InstalledConfig) != fixed[4].path {
		t.Fatal("fixed active inventory unavailable", err)
	}
	for i, item := range fixed {
		if _, ok := seen[item.path]; !ok {
			t.Fatalf("fixed resource %d skipped", i)
		}
	}
	if _, err := readWith(nil); err != ErrUnavailable {
		t.Fatal("missing reader accepted")
	}
	for _, failure := range []func(string, int64) ([]byte, error){
		func(string, int64) ([]byte, error) { return nil, errors.New("synthetic") },
		func(string, int64) ([]byte, error) { return nil, nil },
		func(_ string, limit int64) ([]byte, error) { return bytes.Repeat([]byte("x"), int(limit)+1), nil },
	} {
		if _, err := readWith(failure); err != ErrUnavailable {
			t.Fatal("failed active resource read accepted")
		}
	}
	for failAt := range fixed {
		calls := 0
		got, err := readWith(func(string, int64) ([]byte, error) {
			index := calls
			calls++
			if index == failAt {
				return nil, errors.New("synthetic late failure")
			}
			return []byte("known"), nil
		})
		if err != ErrUnavailable || got.CA != nil || calls != failAt+1 {
			t.Fatal("partial active inventory escaped refusal")
		}
	}
}

func TestClassifyOnlyPreviousOrNextExactBytes(t *testing.T) {
	previous := connectedresources.Set{
		CA: []byte("old CA"), Hosts: []byte("old hosts"),
		CollectorUnit: []byte("old collector"), UploaderUnit: []byte("old uploader"),
		InstalledConfig: []byte("old config"),
	}
	next := connectedresources.Set{
		CA: []byte("new CA"), Hosts: []byte("new hosts"),
		CollectorUnit: []byte("new collector"), UploaderUnit: []byte("new uploader"),
		InstalledConfig: []byte("new config"),
	}
	pair := connectedresources.Pair{Previous: previous, Next: next}
	mixed := connectedresources.Set{
		CA: next.CA, Hosts: previous.Hosts, CollectorUnit: next.CollectorUnit,
		UploaderUnit: previous.UploaderUnit, InstalledConfig: next.InstalledConfig,
	}
	for _, actual := range []connectedresources.Set{previous, next, mixed} {
		got, err := ClassifyBytes(pair, actual)
		if err != nil || got != connectedresources.Hash(actual) {
			t.Fatal("recognized active byte combination refused", err)
		}
	}
	for _, bad := range []connectedresources.Set{
		{CA: []byte("foreign"), Hosts: previous.Hosts, CollectorUnit: previous.CollectorUnit, UploaderUnit: previous.UploaderUnit, InstalledConfig: previous.InstalledConfig},
		{CA: previous.CA, Hosts: previous.Hosts, CollectorUnit: previous.CollectorUnit, UploaderUnit: previous.UploaderUnit, InstalledConfig: nil},
	} {
		got, err := ClassifyBytes(pair, bad)
		if err != ErrUnavailable || got != (connectedtransition.Resources{}) {
			t.Fatal("unknown or missing active bytes accepted")
		}
	}
}
