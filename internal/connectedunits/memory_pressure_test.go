package connectedunits

import (
	"strings"
	"testing"
)

func TestEveryOwnedProfileSkipsMemoryPressureEnvironment(t *testing.T) {
	c := fixture()
	u, err := RenderUnits(c)
	if err != nil {
		t.Fatal(err)
	}
	profiles := []string{string(u.Collector), string(u.Uploader)}
	input := EnrollmentInput{c.UploaderUID, c.UploaderGID, c.SharedGID, c.ArtifactSHA256, c.Addresses}
	for _, mode := range []EnrollmentMode{Enroll, ValidateEnrollment, ValidateLedger} {
		command, err := RenderEnrollmentProperties(input, mode)
		if err != nil {
			t.Fatal(err)
		}
		profiles = append(profiles, strings.Join(command.Properties, "\n")+"\n")
	}
	for _, text := range profiles {
		if strings.Count(text, "MemoryPressureWatch=") != 1 || !strings.Contains(text, "\nMemoryPressureWatch=skip\n") || !strings.Contains(text, "\nMemoryMax=128M\n") {
			t.Fatal("owned profile must suppress generated pressure environment without changing memory limit")
		}
	}
}
