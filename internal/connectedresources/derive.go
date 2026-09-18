// Package connectedresources derives exact, code-owned transition resource
// bytes. It has no filesystem, manager, ledger, or activation authority.
package connectedresources

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
)

var ErrInvalid = errors.New("connected_resource_derivation_invalid")

// Set contains exact bytes for the five fixed active resources. It is never a
// path map and does not authorize a write. CA bytes are copied from stage data.
type Set struct {
	CA, Hosts, CollectorUnit, UploaderUnit, InstalledConfig []byte
}

type Pair struct{ Previous, Next Set }

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Hash is a measurement only. The caller must independently establish the
// provenance, ownership and exact path of every observed byte sequence.
func Hash(s Set) connectedtransition.Resources {
	return connectedtransition.Resources{
		CA: digest(s.CA), Hosts: digest(s.Hosts),
		CollectorUnit: digest(s.CollectorUnit), UploaderUnit: digest(s.UploaderUnit),
		InstalledConfig: digest(s.InstalledConfig),
	}
}

func derive(c connectedprofile.Config, ca []byte) (Set, error) {
	if len(ca) == 0 || len(ca) > connectedtransition.MaxCABytes {
		return Set{}, ErrInvalid
	}
	config, err := connectedprofile.Encode(c)
	if err != nil {
		return Set{}, ErrInvalid
	}
	units, err := connectedunits.RenderUnits(c)
	if err != nil {
		return Set{}, ErrInvalid
	}
	var hosts strings.Builder
	for _, address := range c.Addresses {
		hosts.WriteString(address)
		hosts.WriteByte(' ')
		hosts.WriteString(connectedprofile.Hostname)
		hosts.WriteByte('\n')
	}
	return Set{
		CA: append([]byte(nil), ca...), Hosts: []byte(hosts.String()),
		CollectorUnit: units.Collector, UploaderUnit: units.Uploader,
		InstalledConfig: config,
	}, nil
}

// Derive refuses a transition record whose resource hashes do not match the
// reviewed unit renderer and canonical installed configurations. The CA inputs
// must come from independently verified fixed stage members; this function
// does not establish that provenance or prove the active resources.
func Derive(record connectedtransition.Record, oldCA, newCA []byte) (Pair, error) {
	if record.Validate() != nil {
		return Pair{}, ErrInvalid
	}
	previous, err := derive(record.Previous, oldCA)
	if err != nil {
		return Pair{}, ErrInvalid
	}
	next, err := derive(record.Next, newCA)
	if err != nil || Hash(previous) != record.PreviousResources || Hash(next) != record.NextResources {
		return Pair{}, ErrInvalid
	}
	return Pair{Previous: previous, Next: next}, nil
}
