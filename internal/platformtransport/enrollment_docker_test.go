package platformtransport

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
	"github.com/braidenm/home-lab-observer/internal/enrollmentcoord"
)

func TestDockerEnrollmentRequiresIndependentExactApproval(t *testing.T) {
	valid := fmt.Sprintf(`{"server_id":%q,"connector_secret":%q,"collection_profile":%q,"collection_definition_sha256":%q}`, transportBinding.ServerID, transportCredential, connectedprofile.RichProcessesDockerProfile, connectedprofile.RichProcessesDockerDefinitionSHA256)
	for name, body := range map[string]string{
		"approved":       valid,
		"old-server":     fmt.Sprintf(`{"server_id":%q,"connector_secret":%q}`, transportBinding.ServerID, transportCredential),
		"process-only":   strings.Replace(valid, connectedprofile.RichProcessesDockerProfile, connectedprofile.RichProcessesProfile, 1),
		"old-definition": strings.Replace(valid, connectedprofile.RichProcessesDockerDefinitionSHA256, connectedprofile.RichProcessesDefinitionSHA256, 1),
		"duplicate":      strings.TrimSuffix(valid, "}") + `,"collection_profile":"RICH_PROCESSES_DOCKER_V1"}`,
		"trailing":       valid + "{}",
	} {
		t.Run(name, func(t *testing.T) {
			tr, err := NewRichProcessesDockerEnrollment("https://platform.example.com")
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			tr.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				b, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatal(err)
				}
				expected := fmt.Sprintf(`{"enrollment_secret":%q,"connector_instance_id":%q,"collection_profile":%q,"collection_definition_sha256":%q}`, enrollmentGrant, transportBinding.ConnectorID, connectedprofile.RichProcessesDockerProfile, connectedprofile.RichProcessesDockerDefinitionSHA256)
				if string(b) != expected || req.GetBody != nil || req.Header.Get("Authorization") != "" {
					t.Fatal("Docker enrollment request changed")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			result, err := tr.Exchange(context.Background(), validEnrollmentRequestFixture())
			if err != nil || calls != 1 {
				t.Fatal("exchange failed or retried")
			}
			defer clear(result.ConnectorSecret)
			if (result.Outcome == enrollmentcoord.ExchangeAccepted) != (name == "approved") {
				t.Fatal("unapproved Docker credential accepted")
			}
		})
	}
}
