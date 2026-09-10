package background

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/braidenm/home-lab-observer/internal/containerobs"
	"github.com/braidenm/home-lab-observer/internal/ownerfs"
)

const (
	managedRootMarker = "home-lab-observer-managed-v1"
	backgroundMarker  = "home-lab-observer-background-v1"
	maxSettingsBytes  = 16 << 10
)

type storedSettings struct {
	SchemaVersion  string `json:"schema_version"`
	StateDir       string `json:"state_dir"`
	ListenAddress  string `json:"listen_address"`
	DockerEndpoint string `json:"docker_endpoint"`
}

func validateSettings(settings Settings) (Settings, error) {
	stateDir, err := secureAbsolutePath(settings.StateDir, false)
	if err != nil {
		return Settings{}, coded(CodeInvalidSettings, err)
	}
	host, portText, err := net.SplitHostPort(settings.ListenAddress)
	port, portErr := strconv.Atoi(portText)
	if err != nil || portErr != nil || host != "127.0.0.1" || port < 1 || port > 65535 {
		return Settings{}, coded(CodeInvalidSettings, errors.New("listen address must be explicit IPv4 loopback"))
	}
	if err := containerobs.ValidateEndpoint(settings.DockerEndpoint); err != nil {
		return Settings{}, coded(CodeInvalidSettings, err)
	}
	settings.StateDir = stateDir
	return settings, nil
}

func secureAbsolutePath(path string, requireExisting bool) (string, error) {
	if path == "" || path != strings.TrimSpace(path) || strings.ContainsAny(path, "\x00\r\n") || !filepath.IsAbs(path) {
		return "", errors.New("path must be a clean absolute path")
	}
	if err := validatePlatformPath(path); err != nil {
		return "", err
	}
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	if clean == volume+string(filepath.Separator) {
		return "", errors.New("filesystem root is not a dedicated directory")
	}
	home, _ := os.UserHomeDir()
	if home != "" && samePath(clean, filepath.Clean(home)) {
		return "", errors.New("user home is not a dedicated directory")
	}
	current := volume + string(filepath.Separator)
	relative := strings.TrimPrefix(clean, current)
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if requireExisting {
				return "", err
			}
			break
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("path contains a link or reparse point")
		}
		if err := rejectReparse(current); err != nil {
			return "", errors.New("path contains a link or reparse point")
		}
	}
	return clean, nil
}

func samePath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func writeSettings(path string, settings Settings) error {
	value := storedSettings{SchemaVersion: SchemaVersion, StateDir: settings.StateDir, ListenAddress: settings.ListenAddress, DockerEndpoint: settings.DockerEndpoint}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writePrivateAtomic(path, append(encoded, '\n'))
}

func readSettings(path string) (Settings, error) {
	file, err := openRegular(path, maxSettingsBytes)
	if err != nil {
		return Settings{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maxSettingsBytes+1))
	decoder.DisallowUnknownFields()
	var value storedSettings
	if err := decoder.Decode(&value); err != nil {
		return Settings{}, err
	}
	if value.SchemaVersion != SchemaVersion {
		return Settings{}, errors.New("unsupported background settings schema")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Settings{}, errors.New("background settings contain trailing data")
	}
	return validateSettings(Settings{StateDir: value.StateDir, ListenAddress: value.ListenAddress, DockerEndpoint: value.DockerEndpoint})
}

func openRegular(path string, max int64) (*os.File, error) {
	info, err := ownerfs.ValidateRegular(path, max)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		file.Close()
		return nil, errors.New("managed file changed while opening")
	}
	return file, nil
}

func readSmallRegular(path string, max int64) ([]byte, error) {
	file, err := openRegular(path, max)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil || int64(len(contents)) > max {
		return nil, errors.New("managed file is unreadable or oversized")
	}
	return contents, nil
}

func writePrivateAtomic(path string, contents []byte) error {
	temporary := path + ".next"
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if err := ownerfs.RestrictFile(temporary); err != nil {
		file.Close()
		_ = os.Remove(temporary)
		return err
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(temporary)
		}
	}()
	if count, err := file.Write(contents); err != nil || count != len(contents) {
		if err == nil {
			err = io.ErrShortWrite
		}
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	ok = true
	return nil
}

func coded(code Code, err error) error { return &Error{Code: code, Err: err} }

func validateInstallRoot(root string) (string, error) {
	resolved, err := secureAbsolutePath(root, true)
	if err != nil {
		return "", coded(CodeUnsafeManagedState, err)
	}
	marker, err := readSmallRegular(filepath.Join(resolved, ".home-lab-observer-managed"), 128)
	if err != nil || strings.TrimSpace(string(marker)) != managedRootMarker {
		return "", coded(CodeUnsafeManagedState, errors.New("install root is not managed"))
	}
	launcher := filepath.Join(resolved, "bin", launcherName())
	file, err := openRegular(launcher, 1<<20)
	if err != nil {
		return "", coded(CodeUnsafeManagedState, fmt.Errorf("stable launcher: %w", err))
	}
	file.Close()
	return resolved, nil
}
