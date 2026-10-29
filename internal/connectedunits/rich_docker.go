package connectedunits

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

const RichProcessesDockerCollectorUnit = "home-lab-observer-connected-rich-docker-collector.service"
const RichProcessesDockerReaderUnit = "home-lab-observer-connected-rich-docker-reader.service"
const RichProcessesDockerUploaderUnit = "home-lab-observer-connected-rich-docker-uploader.service"

type RichProcessesDockerUnits struct{ Collector, Reader, Uploader []byte }
type RichProcessesDockerEnrollmentInput struct {
	UploaderUID, UploaderGID, SharedGID uint32
	ArtifactSHA256                      string
	Addresses                           []string
}
type dockerRenderData struct {
	CollectorUID, UploaderUID, UploaderGID, SharedGID, ReaderUID, FrameGID, SocketGID                uint32
	ArtifactSHA256, AddressRules, AddressFamilies, PrivateNetwork, StateBind, StateWrite, SocketDeny string
}

func RichProcessesDockerResources() map[string][]byte {
	names := []string{RichProcessesDockerCollectorUnit + ".tmpl", RichProcessesDockerReaderUnit + ".tmpl", RichProcessesDockerUploaderUnit + ".tmpl", "rich-docker-enrollment.properties.tmpl", "rich-docker-installed-config.schema.json", "rich-docker-collection-policy.schema.json"}
	result := make(map[string][]byte, len(names))
	for _, name := range names {
		b, err := resources.ReadFile("templates/" + name)
		if err != nil {
			panic("connected Docker template missing")
		}
		result[name] = b
	}
	return result
}
func renderDocker(name string, data dockerRenderData) ([]byte, error) {
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
func RenderRichProcessesDockerUnits(c connectedprofile.RichProcessesDockerConfig) (RichProcessesDockerUnits, error) {
	if c.Validate() != nil {
		return RichProcessesDockerUnits{}, ErrInvalid
	}
	addresses, err := rules(c.Addresses)
	if err != nil {
		return RichProcessesDockerUnits{}, err
	}
	d := dockerRenderData{CollectorUID: c.CollectorUID, UploaderUID: c.UploaderUID, UploaderGID: c.UploaderGID, SharedGID: c.SharedGID, ReaderUID: c.ReaderUID, FrameGID: c.FrameGID, SocketGID: c.SocketGID, ArtifactSHA256: c.ArtifactSHA256, AddressRules: addresses}
	collector, err := renderDocker(RichProcessesDockerCollectorUnit+".tmpl", d)
	if err != nil {
		return RichProcessesDockerUnits{}, err
	}
	reader, err := renderDocker(RichProcessesDockerReaderUnit+".tmpl", d)
	if err != nil {
		return RichProcessesDockerUnits{}, err
	}
	uploader, err := renderDocker(RichProcessesDockerUploaderUnit+".tmpl", d)
	if err != nil {
		return RichProcessesDockerUnits{}, err
	}
	return RichProcessesDockerUnits{collector, reader, uploader}, nil
}
func RenderRichProcessesDockerEnrollmentProperties(c RichProcessesDockerEnrollmentInput, mode EnrollmentMode) (EnrollmentCommand, error) {
	if (mode != Enroll && mode != ValidateEnrollment && mode != ValidateLedger) || !validID(c.UploaderUID) || !validID(c.UploaderGID) || !validID(c.SharedGID) || c.UploaderGID == c.SharedGID || !digestPattern.MatchString(c.ArtifactSHA256) {
		return EnrollmentCommand{}, ErrInvalid
	}
	addresses, err := rules(c.Addresses)
	if err != nil {
		return EnrollmentCommand{}, err
	}
	d := dockerRenderData{UploaderUID: c.UploaderUID, UploaderGID: c.UploaderGID, SharedGID: c.SharedGID, ArtifactSHA256: c.ArtifactSHA256, AddressRules: addresses, AddressFamilies: "AF_INET AF_INET6", PrivateNetwork: "no", StateBind: connectedprofile.RichProcessesDockerStateDirectory + "/enrollment:/state/enrollment", StateWrite: "/state/enrollment"}
	if mode != Enroll {
		d.AddressRules = ""
		d.AddressFamilies = ""
		d.SocketDeny = " socket"
		d.PrivateNetwork = "yes"
	}
	if mode == ValidateLedger {
		d.StateBind = connectedprofile.RichProcessesDockerStateDirectory + "/ledger:/state/ledger"
		d.StateWrite = "/state/ledger"
	}
	b, err := renderDocker("rich-docker-enrollment.properties.tmpl", d)
	if err != nil {
		return EnrollmentCommand{}, err
	}
	return EnrollmentCommand{Properties: strings.Split(strings.TrimSuffix(string(b), "\n"), "\n"), Executable: "/bin/observer-connected-rich-docker-uploader", Args: []string{string(mode)}}, nil
}
