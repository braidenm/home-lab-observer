package connectedbootstrap

import "github.com/braidenm/home-lab-observer/internal/connectedbundle"

var richProcessesDockerRenderSpec = renderSpec{
	decode: connectedbundle.DecodeRichProcessesDocker,
	known:  func(m connectedbundle.Manifest) bool { return m.HasKnownRichProcessesDockerContract() },
	archiveBasename: func(version string) string {
		return "home-lab-observer-connected-rich-processes-docker_" + version + "_linux_amd64.tar.gz"
	},
	installBinary: "observer-connected-rich-docker-install",
	releaseRoot:   "/opt/home-lab-observer-connected-rich/releases",
}

func RenderRichProcessesDocker(manifest []byte, archiveSHA256 string, archiveSize int64) ([]byte, error) {
	return render(manifest, archiveSHA256, archiveSize, richProcessesDockerRenderSpec)
}
