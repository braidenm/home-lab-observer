package logobs

import "errors"

// ParseSources validates explicit configuration and returns a private canonical
// copy. It never discovers sources or opens an operating-system log facility.
func ParseSources(goos string, values []string) ([]Source, error) {
	invalid := errors.New("invalid native log source configuration")
	if len(values) > MaxSources {
		return nil, invalid
	}
	var system, application bool
	for _, value := range values {
		switch value {
		case "system":
			if system {
				return nil, invalid
			}
			system = true
		case "application":
			if application || goos == "linux" {
				return nil, invalid
			}
			application = true
		default:
			return nil, invalid
		}
	}
	var result []Source
	if system {
		result = append(result, SourceSystem)
	}
	if application {
		result = append(result, SourceApplication)
	}
	return result, nil
}
