package actions

import (
	"errors"
	"net/netip"
	"strings"
)

// BootstrapProperties returns only code-owned systemd setup properties. The
// installer still has to check the stopped units and wait for the transient
// setup process to exit. Enrollment uses the worker's HTTPS allowlist.
func BootstrapProperties(digest, verb string, engineGID uint32, egress []netip.Addr) ([]string, error) {
	worker, broker, err := RenderUnits(digest, engineGID, egress)
	if err != nil {
		return nil, err
	}
	var source, binary string
	switch verb {
	case "seed-enrollment", "exchange", "verify-enrollment":
		source, binary = worker, "observer-action-worker"
	case "provision-journal":
		source, binary = broker, "observer-action-broker"
	default:
		return nil, errors.New("unsupported action bootstrap operation")
	}
	// Use only overlaid, rendered resources derived from the matching type.
	// This package never parses installed unit files or caller-provided flags.
	return setupProperties(source, binary, digest, verb), nil
}

func setupProperties(source, binary, digest, verb string) []string {
	var props, readOnly []string
	needsHTTPS := verb == "exchange" || verb == "verify-enrollment"
	for _, line := range strings.Split(strings.SplitN(source, "[Service]\n", 2)[1], "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "ExecStart", "ExecPaths", "StandardInput", "StandardOutput", "StandardError":
			continue
		case "BindReadOnlyPaths":
			if strings.Contains(value, "/"+binary+":") || strings.Contains(value, "/engine/docker.sock") ||
				value == "/run/home-lab-observer-actions/worker" {
				continue
			}
			readOnly = append(readOnly, value)
			continue
		case "BindPaths", "ReadWritePaths":
			value = strings.ReplaceAll(value, " /run/home-lab-observer-actions/worker", "")
			line = key + "=" + value
		case "IPAddressAllow", "IPAddressDeny", "RestrictAddressFamilies", "PrivateNetwork":
			if !needsHTTPS {
				continue
			}
		}
		props = append(props, line)
	}
	readOnly = append(readOnly, "/opt/home-lab-observer-actions/releases/"+digest+"/observer-action-setup:/bin/observer-action-setup")
	props = append(props, "ExecPaths=/bin/observer-action-setup",
		"BindReadOnlyPaths="+strings.Join(readOnly, " "), "RuntimeMaxSec=30s")
	if !needsHTTPS {
		props = append(props, "PrivateNetwork=yes", "RestrictAddressFamilies=AF_UNIX")
	}
	return props
}
