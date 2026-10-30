package connectedbundle

import (
	"io"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/connectedunits"
)

var richProcessesDockerBundleSpec = bundleSpec{
	profile:     "ubuntu24.04-systemd255-amd64-rich-processes-docker-canary",
	contract:    connectedprofile.RichProcessesDockerDefinitionSHA256,
	installRole: "rich-docker-install",
	binaries: map[string]string{
		"observer-connected-rich-docker-collector": "rich-docker-collector",
		"observer-connected-rich-docker-reader":    "rich-docker-reader",
		"observer-connected-rich-docker-uploader":  "rich-docker-uploader",
		"observer-connected-rich-docker-install":   "rich-docker-install",
	},
	resources: connectedunits.RichProcessesDockerResources,
}

func RichProcessesDockerNames() []string { return names(richProcessesDockerBundleSpec) }
func (m Manifest) ValidateRichProcessesDocker() error {
	return validateManifest(m, richProcessesDockerBundleSpec, false)
}
func (m Manifest) HasKnownRichProcessesDockerContract() bool {
	return m.ValidateRichProcessesDocker() == nil && m.Schema == VersionV2 && m.ContractSHA256 == connectedprofile.RichProcessesDockerDefinitionSHA256
}
func EncodeRichProcessesDocker(m Manifest) ([]byte, error) {
	return encode(m, m.ValidateRichProcessesDocker)
}
func DecodeRichProcessesDocker(b []byte) (Manifest, error) {
	return decode(b, EncodeRichProcessesDocker)
}
func VerifyRichProcessesDockerDirectory(directory, digest string) (Manifest, error) {
	return verifyDirectory(directory, digest, richProcessesDockerBundleSpec, DecodeRichProcessesDocker)
}
func CreateRichProcessesDockerManifest(directory, version, commit string) ([]byte, error) {
	return createManifestForSpec(directory, version, commit, VersionV2, richProcessesDockerBundleSpec.contract, richProcessesDockerBundleSpec, EncodeRichProcessesDocker)
}
func WriteRichProcessesDockerArchive(w io.Writer, directory, digest string) error {
	return writeArchive(w, directory, digest, VerifyRichProcessesDockerDirectory, EncodeRichProcessesDocker)
}
