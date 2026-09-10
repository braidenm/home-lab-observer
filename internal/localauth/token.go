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

	"github.com/braidenm/home-lab-observer/internal/ownerfs"
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
	parent, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return "", false, err
	}
	userHome, _ := os.UserHomeDir()
	if parent == filepath.Dir(parent) || strings.EqualFold(parent, userHome) {
		return "", false, fmt.Errorf("%w: use a dedicated observer directory", ErrInvalidTokenFile)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", false, fmt.Errorf("create token directory: %w", err)
	}
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", false, fmt.Errorf("%w: state directory must not be a link", ErrInvalidTokenFile)
	}
	if err := restrictDirectory(filepath.Dir(path)); err != nil {
		return "", false, fmt.Errorf("restrict token directory: %w", err)
	}
	if token, err = read(path); err == nil {
		return token, false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", false, err
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
	// Protect a Windows file before any secret bytes reach it.
	if err := restrictFile(path); err != nil {
		return "", false, fmt.Errorf("restrict token file: %w", err)
	}
	if _, err := io.WriteString(file, token+"\n"); err != nil {
		return "", false, fmt.Errorf("write token file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return "", false, fmt.Errorf("sync token file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", false, fmt.Errorf("close token file: %w", err)
	}
	if err := restrictFile(path); err != nil {
		return "", false, fmt.Errorf("restrict token file: %w", err)
	}
	complete = true
	return token, true, nil
}

// Load reads and validates an existing private local token without creating or
// replacing it. Status probes use it instead of the create-if-missing path.
func Load(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("%w: empty path", ErrInvalidTokenFile)
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := ownerfs.ValidateDedicatedDirectory(filepath.Dir(resolved)); err != nil {
		return "", fmt.Errorf("%w: unsafe state directory", ErrInvalidTokenFile)
	}
	info, err := ownerfs.ValidateRegular(resolved, 256)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		return "", fmt.Errorf("%w: unsafe token file", ErrInvalidTokenFile)
	}
	if !validPrivateTokenMode(resolved, info) {
		return "", fmt.Errorf("%w: token permissions are not private", ErrInvalidTokenFile)
	}
	file, err := os.Open(resolved)
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return "", ErrInvalidTokenFile
	}
	contents, err := io.ReadAll(io.LimitReader(file, 257))
	if err != nil || len(contents) > 256 {
		return "", ErrInvalidTokenFile
	}
	return parseToken(contents)
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
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return "", ErrInvalidTokenFile
	}
	contents, err := io.ReadAll(io.LimitReader(file, 257))
	if err != nil {
		return "", fmt.Errorf("read token file: %w", err)
	}
	if len(contents) > 256 {
		return "", ErrInvalidTokenFile
	}
	token, err := parseToken(contents)
	if err != nil {
		return "", err
	}
	if err := restrictFile(path); err != nil {
		return "", fmt.Errorf("restrict token file: %w", err)
	}
	return token, nil
}

func parseToken(contents []byte) (string, error) {
	token := strings.TrimSuffix(string(contents), "\n")
	if strings.ContainsAny(token, "\r\n \t") {
		return "", ErrInvalidTokenFile
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != tokenBytes {
		return "", ErrInvalidTokenFile
	}
	return token, nil
}
