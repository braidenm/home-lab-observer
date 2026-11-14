// Package actions owns the stopped action package's systemd resources.
package actions

import (
	_ "embed"
	"errors"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

//go:embed home-lab-observer-action-worker.service.tmpl
var workerTemplate string

//go:embed home-lab-observer-action-broker.service.tmpl
var brokerTemplate string

// Resources returns the exact unrendered resources shipped with the bundle.
func Resources() map[string][]byte {
	return map[string][]byte{
		"home-lab-observer-action-worker.service.tmpl": []byte(workerTemplate),
		"home-lab-observer-action-broker.service.tmpl": []byte(brokerTemplate),
	}
}

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// RenderUnits renders only fixed unit templates. Egress accepts literal hosts,
// not general networks or systemd keywords. The installer must separately
// verify that the addresses belong to the reviewed resolver and HTTPS origin.
func RenderUnits(digest string, engineGID uint32, egress []netip.Addr) (worker, broker string, err error) {
	invalid := errors.New("invalid action unit inputs")
	if !digestPattern.MatchString(digest) || engineGID == 0 || engineGID == ^uint32(0) || len(egress) == 0 || len(egress) > 16 {
		return "", "", invalid
	}
	hosts := make([]string, 0, len(egress))
	seen := make(map[netip.Addr]bool, len(egress))
	for _, address := range egress {
		if !address.IsValid() || address.Zone() != "" || address.Is4In6() || !address.IsGlobalUnicast() || seen[address] {
			return "", "", invalid
		}
		seen[address] = true
		hosts = append(hosts, netip.PrefixFrom(address, address.BitLen()).String())
	}
	worker = strings.NewReplacer("@MANIFEST_SHA256@", digest, "@WORKER_EGRESS@", strings.Join(hosts, " ")).Replace(workerTemplate)
	broker = strings.NewReplacer("@MANIFEST_SHA256@", digest, "@ENGINE_GID@", strconv.FormatUint(uint64(engineGID), 10)).Replace(brokerTemplate)
	return worker, broker, nil
}
