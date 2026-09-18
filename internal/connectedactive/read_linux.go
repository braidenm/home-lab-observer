//go:build linux

// Package connectedactive reads only the five fixed root-owned active resource
// paths for an interrupted connected transition. It never writes or repairs.
package connectedactive

import (
	"bytes"
	"errors"
	"os"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedresources"
	"github.com/braidenm/home-lab-observer/internal/connectedtransition"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
)

var ErrUnavailable = errors.New("connected_active_resources_unavailable")

const root = connectedprofile.StateDirectory + "/uploader-root"

type fixedResource struct {
	path  string
	limit int64
	mode  os.FileMode
}

var fixed = [5]fixedResource{
	{root + "/etc/ssl/certs/ca-certificates.crt", connectedtransition.MaxCABytes, 0644},
	{root + "/etc/hosts", 4096, 0644},
	{"/etc/systemd/system/" + connectedunits.CollectorUnit, connectedunits.MaxRenderedBytes, 0644},
	{"/etc/systemd/system/" + connectedunits.UploaderUnit, connectedunits.MaxRenderedBytes, 0644},
	{connectedprofile.ConfigPath, connectedprofile.MaxConfigBytes, 0644},
}

// Read uses the profile's anchored no-follow, root-owned, non-writable reader.
// A caller must hold the installation lease and separately prove that both
// owned workers are stopped. Ext4 and ledger evidence remain mandatory before
// any replacement or staging cleanup.
func Read() (connectedresources.Set, error) {
	return readWith(connectedprofile.ReadRootFileMode)
}

// Private seam verifies path selection and bounds without touching host paths.
func readWith(read func(string, int64, os.FileMode) ([]byte, error)) (connectedresources.Set, error) {
	if read == nil {
		return connectedresources.Set{}, ErrUnavailable
	}
	var data [5][]byte
	for i, item := range fixed {
		b, err := read(item.path, item.limit, item.mode)
		if err != nil || len(b) == 0 || int64(len(b)) > item.limit {
			return connectedresources.Set{}, ErrUnavailable
		}
		data[i] = append([]byte(nil), b...)
	}
	return connectedresources.Set{
		CA: data[0], Hosts: data[1], CollectorUnit: data[2],
		UploaderUnit: data[3], InstalledConfig: data[4],
	}, nil
}

// ClassifyBytes requires every active byte sequence to equal either the exact
// previous or next code-derived sequence. It returns hashes for the pure
// staged classifier, not permission to write, clean up, or start workers.
func ClassifyBytes(expected connectedresources.Pair, actual connectedresources.Set) (connectedtransition.Resources, error) {
	for _, triple := range [][3][]byte{
		{actual.CA, expected.Previous.CA, expected.Next.CA},
		{actual.Hosts, expected.Previous.Hosts, expected.Next.Hosts},
		{actual.CollectorUnit, expected.Previous.CollectorUnit, expected.Next.CollectorUnit},
		{actual.UploaderUnit, expected.Previous.UploaderUnit, expected.Next.UploaderUnit},
		{actual.InstalledConfig, expected.Previous.InstalledConfig, expected.Next.InstalledConfig},
	} {
		if len(triple[0]) == 0 || len(triple[1]) == 0 || len(triple[2]) == 0 ||
			(!bytes.Equal(triple[0], triple[1]) && !bytes.Equal(triple[0], triple[2])) {
			return connectedtransition.Resources{}, ErrUnavailable
		}
	}
	return connectedresources.Hash(actual), nil
}
