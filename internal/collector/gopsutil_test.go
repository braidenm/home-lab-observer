package collector

import "testing"

func TestNormalizedProcessStatus(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		statuses []string
		want     string
	}{
		{name: "missing", want: "unknown"},
		{name: "empty", statuses: []string{""}, want: "unknown"},
		{name: "reported", statuses: []string{"running", "sleeping"}, want: "running"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := normalizedProcessStatus(testCase.statuses); got != testCase.want {
				t.Fatalf("status=%q want=%q", got, testCase.want)
			}
		})
	}
}
