package containerobs

import "testing"

func TestValidateEndpointMatchesCollectorWithoutConnecting(t *testing.T) {
	for _, endpoint := range []string{
		"", "unix:///nonexistent-observer-test/docker.sock", "npipe:////./pipe/nonexistent-observer-test",
		"npipe:////./pipe/", "unix:///", "unix://remote.invalid/path", "tcp://127.0.0.1:2375",
		" unix:///tmp/docker.sock", "unix:///tmp/a%2Fb", "unix:///tmp/docker.sock?secret=redacted",
		"npipe:////./pipe/one/two", "npipe://remote.invalid/pipe/docker", "https://example.invalid",
	} {
		t.Run(endpoint, func(t *testing.T) {
			validationErr := ValidateEndpoint(endpoint)
			collector, constructionErr := New(Config{Endpoint: endpoint})
			if collector != nil {
				defer collector.Close()
			}
			if (validationErr == nil) != (constructionErr == nil) {
				t.Fatal("lifecycle endpoint validation drifted from collector policy")
			}
		})
	}
}
