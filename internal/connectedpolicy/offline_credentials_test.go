package connectedpolicy

import "testing"

func TestEmptyCredentialOutput(t *testing.T) {
	for chunk := 1; chunk <= len(emptyCredentials); chunk++ {
		var output emptyCredentialsOutput
		for offset := 0; offset < len(emptyCredentials); offset += chunk {
			end := min(offset+chunk, len(emptyCredentials))
			p := []byte(emptyCredentials[offset:end])
			if n, err := output.Write(p); err != nil || n != len(p) {
				t.Fatal("empty rejected")
			}
		}
		if !output.complete() {
			t.Fatal("incomplete")
		}
	}
}

func TestCredentialOutputRejectsDrift(t *testing.T) {
	for _, value := range []string{"", "a(ss) 1 secret", emptyCredentials + "extra", emptyCredentials[:len(emptyCredentials)-1], "[unprintable]\n", "a(ss) 0\na(ss) 0\na(say) 1 synthetic-secret"} {
		var output emptyCredentialsOutput
		p := []byte(value)
		_, _ = output.Write(p)
		if output.complete() {
			t.Fatal("unsafe accepted")
		}
	}
}
