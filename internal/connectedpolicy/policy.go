// Package connectedpolicy validates the installed uploader's fixed endpoint and
// effective systemd IP rules. It is not imported by the collector's package graph.
package connectedpolicy

import (
	"context"
	"errors"
	"net/netip"
	"regexp"
	"slices"
	"strings"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

var ErrUnsafe = errors.New("connected_endpoint_policy_unsafe")

const maxOutput = 16 * 1024

var sliceName = regexp.MustCompile(`^(?:-|[A-Za-z0-9_][A-Za-z0-9_.-]{0,127})\.slice$`)

type properties struct {
	id, parent, allow, deny, dropins string
}

type query func(context.Context, string) ([]byte, error)

func parse(data []byte, expected string) (properties, error) {
	if len(data) == 0 || len(data) > maxOutput || strings.ContainsAny(string(data), "\x00\r") {
		return properties{}, ErrUnsafe
	}
	wantFields := 7
	if ownedUnit(expected) {
		wantFields++
	}
	values := make(map[string]string, wantFields)
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		k, v, found := strings.Cut(line, "=")
		if !found {
			return properties{}, ErrUnsafe
		}
		switch k {
		case "Id", "Slice", "IPAddressAllow", "IPAddressDeny", "DropInPaths", "LoadState", "NeedDaemonReload":
		case "MemoryPressureWatch":
			// Only our services have a worker environment contract. Ancestor
			// slices remain governed by the existing IP inheritance checks.
			if !ownedUnit(expected) || v != "skip" {
				return properties{}, ErrUnsafe
			}
		default:
			return properties{}, ErrUnsafe
		}
		if _, exists := values[k]; exists {
			return properties{}, ErrUnsafe
		}
		values[k] = v
	}
	if len(values) != wantFields || values["Id"] != expected || values["LoadState"] != "loaded" || values["NeedDaemonReload"] != "no" {
		return properties{}, ErrUnsafe
	}
	return properties{expected, values["Slice"], values["IPAddressAllow"], values["IPAddressDeny"], values["DropInPaths"]}, nil
}

func prefixes(raw string) (map[netip.Prefix]bool, error) {
	fields := strings.Fields(raw)
	if len(fields) > 128 || strings.Join(fields, " ") != raw {
		return nil, ErrUnsafe
	}
	result := make(map[netip.Prefix]bool, len(fields))
	for _, field := range fields {
		p, err := netip.ParsePrefix(field)
		if err != nil || p.Addr().Is4In6() || p.Masked() != p || p.String() != field || result[p] {
			return nil, ErrUnsafe
		}
		result[p] = true
	}
	return result, nil
}

func expectedPrefixes(addresses []string) (map[netip.Prefix]bool, error) {
	if len(addresses) == 0 || len(addresses) > 8 || !slices.IsSorted(addresses) {
		return nil, ErrUnsafe
	}
	result := make(map[netip.Prefix]bool, len(addresses))
	for _, raw := range addresses {
		a, err := netip.ParseAddr(raw)
		if err != nil || a.String() != raw || !connectedprofile.PublicAddress(a) {
			return nil, ErrUnsafe
		}
		p := netip.PrefixFrom(a, a.BitLen())
		if result[p] {
			return nil, ErrUnsafe
		}
		result[p] = true
	}
	return result, nil
}

func ownedUnit(name string) bool {
	switch name {
	case "home-lab-observer-connected-uploader.service", "home-lab-observer-connected-enrollment.service":
		return true
	}
	return false
}

// validate walks the actual Slice chain, not inferred directory names. Ancestor
// allows win over a child's deny, so every level must remain within the exact
// approved host addresses. This checks policy meaning, NOT BPF enforcement.
func validate(ctx context.Context, start string, addresses []string, service bool, read query) error {
	if ctx == nil || ctx.Err() != nil || read == nil || (service && !ownedUnit(start)) || (!service && start != "system.slice") {
		return ErrUnsafe
	}
	expected, err := expectedPrefixes(addresses)
	if err != nil {
		return ErrUnsafe
	}
	seen := make(map[string]bool, 8)
	name := start
	for depth := 0; depth < 8; depth++ {
		if seen[name] || ctx.Err() != nil {
			return ErrUnsafe
		}
		seen[name] = true
		data, err := read(ctx, name)
		if err != nil {
			return ErrUnsafe
		}
		p, err := parse(data, name)
		if err != nil {
			return ErrUnsafe
		}
		allows, err := prefixes(p.allow)
		if err != nil {
			return ErrUnsafe
		}
		for prefix := range allows {
			if !expected[prefix] {
				return ErrUnsafe
			}
		}
		denies, err := prefixes(p.deny)
		if err != nil {
			return ErrUnsafe
		}
		if service && depth == 0 {
			if p.dropins != "" || p.parent != "system.slice" || len(allows) != len(expected) || len(denies) != 2 || !denies[netip.MustParsePrefix("0.0.0.0/0")] || !denies[netip.MustParsePrefix("::/0")] {
				return ErrUnsafe
			}
		}
		if name == "-.slice" {
			if p.parent != "" {
				return ErrUnsafe
			}
			return nil
		}
		if !sliceName.MatchString(p.parent) {
			return ErrUnsafe
		}
		name = p.parent
	}
	return ErrUnsafe
}
