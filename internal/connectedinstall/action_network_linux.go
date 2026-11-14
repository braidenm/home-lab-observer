//go:build linux

package connectedinstall

import (
	"context"
	"net/netip"
	"strings"

	"github.com/braidenm/home-lab-observer/internal/connectedpolicy"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/packaging/actions"
)

type actionNetworkResources struct {
	workerUnit string
	brokerUnit string
	hosts      string
}

func resolveActionNetwork(ctx context.Context, digest string, engineGID uint32) (actionNetworkResources, error) {
	addresses, err := connectedpolicy.Resolve(ctx)
	if err != nil || connectedpolicy.ValidateParent(ctx, addresses) != nil {
		return actionNetworkResources{}, ErrUnsafe
	}
	return renderActionNetwork(digest, engineGID, addresses)
}

func renderActionNetwork(digest string, engineGID uint32, addresses []string) (actionNetworkResources, error) {
	if len(addresses) == 0 || len(addresses) > 8 {
		return actionNetworkResources{}, ErrUnsafe
	}
	var hosts strings.Builder
	ips := make([]netip.Addr, 0, len(addresses))
	for _, raw := range addresses {
		ip, err := netip.ParseAddr(raw)
		if err != nil || ip.String() != raw || !connectedprofile.PublicAddress(ip) {
			return actionNetworkResources{}, ErrUnsafe
		}
		ips = append(ips, ip)
		hosts.WriteString(raw + " " + connectedprofile.Hostname + "\n")
	}
	worker, broker, err := actions.RenderUnits(digest, engineGID, ips)
	if err != nil {
		return actionNetworkResources{}, ErrUnsafe
	}
	return actionNetworkResources{workerUnit: worker, brokerUnit: broker, hosts: hosts.String()}, nil
}
