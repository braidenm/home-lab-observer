package connectedprofile

import "encoding/json"

// EncodeRichProcessesDockerReaderStartup domain-separates reader activation
// from the uploader's unchanged raw-config binding. Neither role nor envelope
// version is caller-selectable. This does not replace local policy validation.
func EncodeRichProcessesDockerReaderStartup(c RichProcessesDockerConfig) ([]byte, error) {
	configuration, err := EncodeRichProcessesDockerConfig(c)
	if err != nil {
		return nil, ErrUnsafe
	}
	return json.Marshal(struct {
		Version       string          `json:"version"`
		Role          string          `json:"role"`
		Configuration json.RawMessage `json:"configuration"`
	}{Version: "observer-connected-docker-reader-startup/v1", Role: "rich-docker-reader", Configuration: configuration})
}
