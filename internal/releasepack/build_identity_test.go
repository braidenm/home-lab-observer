package releasepack

import (
	"runtime/debug"
	"testing"
)

func TestBuildInfoTargetIsExact(t *testing.T) {
	for _, mutation := range []string{"valid", "wrong-os", "wrong-arch", "duplicate-setting", "dynamic-core", "untrimmed"} {
		t.Run(mutation, func(t *testing.T) {
			build := &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "GOOS", Value: "linux"}, {Key: "GOARCH", Value: "amd64"}, {Key: "CGO_ENABLED", Value: "0"}, {Key: "-trimpath", Value: "true"}}}
			switch mutation {
			case "wrong-os":
				build.Settings[0].Value = "darwin"
			case "wrong-arch":
				build.Settings[1].Value = "arm64"
			case "duplicate-setting":
				build.Settings = append(build.Settings, build.Settings[0])
			case "dynamic-core":
				build.Settings[2].Value = "1"
			case "untrimmed":
				build.Settings[3].Value = "false"
			}
			if err := validateBuildInfo(build, targets[0], false); (err == nil) != (mutation == "valid") {
				t.Fatal("target acceptance mismatch")
			}
		})
	}
}
