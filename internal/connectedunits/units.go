// Package connectedunits renders only the reviewed installed canary profiles.
// Templates are not proof that a host enforces their effective policy.
package connectedunits

import (
	"bytes"
	"embed"
	"errors"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"text/template"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

const CollectorUnit = "home-lab-observer-connected-collector.service"
const UploaderUnit = "home-lab-observer-connected-uploader.service"
const MaxRenderedBytes = 16384

var ErrInvalid = errors.New("connected_units_invalid")
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

//go:embed templates/*
var resources embed.FS

type Units struct{ Collector, Uploader []byte }
type EnrollmentInput struct {
	UploaderUID, UploaderGID, SharedGID uint32
	ArtifactSHA256                      string
	Addresses                           []string
}
type EnrollmentMode string

const Enroll EnrollmentMode = "enroll"
const ValidateEnrollment EnrollmentMode = "validate-enrollment"
const ValidateLedger EnrollmentMode = "validate-ledger"

type EnrollmentCommand struct {
	Properties []string
	Executable string
	Args       []string
}
type renderData struct {
	CollectorUID, UploaderUID, UploaderGID, SharedGID                                    uint32
	ArtifactSHA256, AddressRules, AddressFamilies, PrivateNetwork, StateBind, StateWrite string
}

// Resources returns detached reviewed bytes, keyed by exact bundle filename.
func Resources() map[string][]byte {
	result := make(map[string][]byte, 4)
	for _, name := range []string{CollectorUnit + ".tmpl", UploaderUnit + ".tmpl", "enrollment.properties.tmpl", "installed-config.schema.json"} {
		data, err := resources.ReadFile("templates/" + name)
		if err != nil {
			panic("connected template missing")
		}
		result[name] = data
	}
	return result
}

func render(name string, data renderData) ([]byte, error) {
	t, err := template.New(name).Option("missingkey=error").ParseFS(resources, "templates/"+name)
	if err != nil {
		return nil, ErrInvalid
	}
	var b bytes.Buffer
	if t.Execute(&b, data) != nil || b.Len() > MaxRenderedBytes {
		return nil, ErrInvalid
	}
	return b.Bytes(), nil
}

func rules(addresses []string) (string, error) {
	if len(addresses) == 0 || len(addresses) > 8 || !slices.IsSorted(addresses) {
		return "", ErrInvalid
	}
	var b strings.Builder
	for i, raw := range addresses {
		a, err := netip.ParseAddr(raw)
		if err != nil || a.String() != raw || !connectedprofile.PublicAddress(a) || (i > 0 && raw == addresses[i-1]) {
			return "", ErrInvalid
		}
		bits := "/128"
		if a.Is4() {
			bits = "/32"
		}
		b.WriteString("IPAddressAllow=" + raw + bits + "\n")
	}
	return b.String(), nil
}

func validID(id uint32) bool { return id != 0 && id != ^uint32(0) }

func RenderUnits(c connectedprofile.Config) (Units, error) {
	if c.Validate() != nil || !validID(c.CollectorUID) || !validID(c.UploaderUID) || !validID(c.UploaderGID) || !validID(c.SharedGID) {
		return Units{}, ErrInvalid
	}
	addresses, err := rules(c.Addresses)
	if err != nil {
		return Units{}, ErrInvalid
	}
	d := renderData{CollectorUID: c.CollectorUID, UploaderUID: c.UploaderUID, UploaderGID: c.UploaderGID, SharedGID: c.SharedGID, ArtifactSHA256: c.ArtifactSHA256, AddressRules: addresses}
	collector, err := render(CollectorUnit+".tmpl", d)
	if err != nil {
		return Units{}, err
	}
	uploader, err := render(UploaderUnit+".tmpl", d)
	if err != nil {
		return Units{}, err
	}
	return Units{collector, uploader}, nil
}

// RenderEnrollmentProperties is for a fixed systemd-run --pipe --wait invocation.
// StandardInput/Output must remain those private inherited pipes; stderr must be
// discarded by the root parent. No secret enters properties or arguments.
func RenderEnrollmentProperties(c EnrollmentInput, mode EnrollmentMode) (EnrollmentCommand, error) {
	if (mode != Enroll && mode != ValidateEnrollment && mode != ValidateLedger) || !validID(c.UploaderUID) || !validID(c.UploaderGID) || !validID(c.SharedGID) || c.UploaderGID == c.SharedGID || !digestPattern.MatchString(c.ArtifactSHA256) {
		return EnrollmentCommand{}, ErrInvalid
	}
	addresses, err := rules(c.Addresses)
	if err != nil {
		return EnrollmentCommand{}, ErrInvalid
	}
	d := renderData{UploaderUID: c.UploaderUID, UploaderGID: c.UploaderGID, SharedGID: c.SharedGID, ArtifactSHA256: c.ArtifactSHA256, AddressRules: addresses, AddressFamilies: "AF_INET AF_INET6", PrivateNetwork: "no", StateBind: "/var/lib/home-lab-observer-connected/enrollment:/state/enrollment", StateWrite: "/state/enrollment"}
	if mode != Enroll {
		d.AddressRules = ""
		d.AddressFamilies = "none"
		d.PrivateNetwork = "yes"
	}
	if mode == ValidateLedger {
		d.StateBind = "/var/lib/home-lab-observer-connected/ledger:/state/ledger"
		d.StateWrite = "/state/ledger"
	}
	data, err := render("enrollment.properties.tmpl", d)
	if err != nil {
		return EnrollmentCommand{}, err
	}
	return EnrollmentCommand{Properties: strings.Split(strings.TrimSuffix(string(data), "\n"), "\n"), Executable: "/bin/observer-connected-uploader", Args: []string{string(mode)}}, nil
}
