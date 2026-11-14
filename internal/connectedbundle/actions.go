package connectedbundle

import (
	"io"

	"github.com/braidenm/home-lab-observer/internal/connectedidentity"
	"github.com/braidenm/home-lab-observer/packaging/actions"
)

func commandPath(role string) string {
	const prefix = "github.com/braidenm/home-lab-observer/cmd/observer-"
	switch role {
	case "action-worker", "action-broker", "action-setup":
		return prefix + role
	default:
		return prefix + "connected-" + role
	}
}

var actionBundleSpec = bundleSpec{
	profile:     "ubuntu24.04-systemd255-amd64-actions-v1-canary",
	contract:    connectedidentity.ActionContractDigest(),
	installRole: "action-setup",
	binaries: map[string]string{
		"observer-action-worker": "action-worker",
		"observer-action-broker": "action-broker",
		"observer-action-setup":  "action-setup",
	},
	resources: actions.Resources,
}

func ActionNames() []string { return names(actionBundleSpec) }

func EncodeActions(m Manifest) ([]byte, error) {
	return encode(m, func() error { return validateManifest(m, actionBundleSpec, false) })
}

func DecodeActions(raw []byte) (Manifest, error) {
	return decode(raw, EncodeActions)
}

func VerifyActionDirectory(directory, digest string) (Manifest, error) {
	return verifyDirectory(directory, digest, actionBundleSpec, DecodeActions)
}

func CreateActionManifest(directory, version, commit string) ([]byte, error) {
	return createManifestForSpec(directory, version, commit, VersionV2, actionBundleSpec.contract, actionBundleSpec, EncodeActions)
}

func WriteActionArchive(w io.Writer, directory, digest string) error {
	return writeArchive(w, directory, digest, VerifyActionDirectory, EncodeActions)
}
