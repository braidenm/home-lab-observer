package releasepack

import (
	"debug/buildinfo"
	"errors"
	"os"
	"runtime/debug"

	"github.com/braidenm/home-lab-observer/internal/buildidentity"
)

func journalInputName(arch string) string { return "observer-journal-helper_linux_" + arch }

func validateReleaseBinaries(config Config, binaries map[string]sourceFile) error {
	for _, target := range targets {
		digest := ""
		if target.os == "linux" {
			helper := binaries[journalInputName(target.arch)]
			var err error
			digest, _, err = hashFileBounded(helper.path, maxBinarySize)
			if err != nil || verifyBuildFile(helper, config, target, "", true) != nil {
				return errors.New("HELPER_IDENTITY_INVALID")
			}
		}
		if verifyBuildFile(binaries[target.binaryName], config, target, digest, false) != nil {
			return errors.New("BINARY_IDENTITY_INVALID")
		}
	}
	return nil
}

func verifyBuildFile(source sourceFile, config Config, target target, digest string, helper bool) error {
	file, err := os.Open(source.path)
	if err != nil {
		return errors.New("BUILD_IDENTITY_INVALID")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !os.SameFile(info, source.info) || info.Size() != source.info.Size() || !singleLink(source.path, info) {
		return errors.New("BUILD_IDENTITY_INVALID")
	}
	build, err := buildinfo.Read(file)
	if err != nil {
		return errors.New("BUILD_IDENTITY_INVALID")
	}
	if err := validateBuildInfo(build, target, helper); err != nil {
		return err
	}
	if _, err := file.Seek(0, 0); err != nil {
		return buildidentity.ErrInvalid
	}
	identity, err := buildidentity.Scan(file, info.Size())
	role := "observer"
	if helper {
		role = "journal-helper"
	}
	if err != nil || identity.Role != role || identity.Version != config.Version || identity.Commit != config.Commit || identity.OS != target.os || identity.Arch != target.arch || identity.HelperSHA256 != digest {
		return buildidentity.ErrInvalid
	}
	return nil
}

func validateBuildInfo(build *debug.BuildInfo, target target, helper bool) error {
	invalid := errors.New("BUILD_IDENTITY_INVALID")
	if build == nil {
		return invalid
	}
	settings := map[string]string{}
	for _, setting := range build.Settings {
		if _, exists := settings[setting.Key]; exists {
			return invalid
		}
		settings[setting.Key] = setting.Value
	}
	if settings["GOOS"] != target.os || settings["GOARCH"] != target.arch {
		return invalid
	}
	if !helper && settings["CGO_ENABLED"] != "0" {
		return invalid
	}
	// Trimpath omits linker flags. The authoritative record supplies identity;
	// build info independently validates target and static-core mode.
	if settings["-trimpath"] != "true" {
		return invalid
	}
	return nil
}
