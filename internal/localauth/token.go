package localauth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const tokenBytes = 32

var ErrInvalidTokenFile = errors.New("invalid local API token file")

func DefaultTokenPath() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(directory, "home-lab-observer", "local-api.token"), nil
}

// Ensure returns a stable, cryptographically random local bearer token. The
// token is never accepted from process arguments or environment variables.
func Ensure(path string) (token string, created bool, err error) {
	if path == "" {
		return "", false, fmt.Errorf("%w: empty path", ErrInvalidTokenFile)
	}
	if token, err = read(path); err == nil {
		return token, false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", false, fmt.Errorf("create token directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return "", false, fmt.Errorf("restrict token directory: %w", err)
	}

	raw := make([]byte, tokenBytes)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", false, fmt.Errorf("generate local API token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		token, err = read(path)
		return token, false, err
	}
	if err != nil {
		return "", false, fmt.Errorf("create token file: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = file.Close()
			_ = os.Remove(path)
		}
	}()
	if _, err := io.WriteString(file, token+"\n"); err != nil {
		return "", false, fmt.Errorf("write token file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return "", false, fmt.Errorf("sync token file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", false, fmt.Errorf("close token file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", false, fmt.Errorf("restrict token file: %w", err)
	}
	complete = true
	return token, true, nil
}

func MatchesAuthorization(header, expectedToken string) bool {
	scheme, candidate, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || candidate == "" || strings.ContainsAny(candidate, " \t\r\n") {
		return false
	}
	return len(candidate) == len(expectedToken) && subtle.ConstantTimeCompare([]byte(candidate), []byte(expectedToken)) == 1
}

func read(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 256 {
		return "", ErrInvalidTokenFile
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read token file: %w", err)
	}
	token := strings.TrimSuffix(string(contents), "\n")
	if strings.ContainsAny(token, "\r\n \t") {
		return "", ErrInvalidTokenFile
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != tokenBytes {
		return "", ErrInvalidTokenFile
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", fmt.Errorf("restrict token file: %w", err)
	}
	return token, nil
}
