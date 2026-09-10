// Test-only scratch probe. It never opens host journals or a Docker socket.
package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/braidenm/home-lab-observer/internal/releasepack"
)

var errProof = errors.New("MISSING_RUNTIME_PROOF_FAILED")

func main() {
	var err error
	if len(os.Args) == 4 && os.Args[1] == "--stage" {
		err = stage(os.Args[2], os.Args[3])
	} else if len(os.Args) == 1 {
		err = probe()
	} else {
		err = errProof
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "MISSING_RUNTIME_PROOF_FAILED")
		os.Exit(1)
	}
	fmt.Println("MISSING_RUNTIME_PROOF_PASSED")
}

type archiveIdentity struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// stage runs on the CI host before image creation; it only reads release artifacts.
func stage(source, destination string) error {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return errProof
	}
	return stageLinux(source, destination, runtime.GOARCH)
}

// stageLinux also allows cross-host tests of the exact Linux package layout.
func stageLinux(source, destination, arch string) error {
	if arch != "amd64" && arch != "arm64" {
		return errProof
	}
	if err := releasepack.Verify(source); err != nil {
		return errProof
	}
	manifestBytes, err := os.ReadFile(filepath.Join(source, releasepack.ManifestName))
	if err != nil {
		return errProof
	}
	manifest, err := releasepack.DecodeManifest(manifestBytes)
	if err != nil || manifest.SchemaVersion != releasepack.SchemaVersionV2 {
		return errProof
	}
	var asset *releasepack.Asset
	for i := range manifest.Assets {
		if manifest.Assets[i].OS == "linux" && manifest.Assets[i].Arch == arch {
			asset = &manifest.Assets[i]
		}
	}
	if asset == nil || asset.JournalHelper == nil {
		return errProof
	}
	if err := os.Mkdir(destination, 0700); err != nil {
		return errProof
	}
	if err := os.Mkdir(filepath.Join(destination, "package"), 0755); err != nil {
		return errProof
	}
	f, err := os.Open(filepath.Join(source, asset.Filename))
	if err != nil {
		return errProof
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return errProof
	}
	defer gz.Close()
	if err := extract(tar.NewReader(gz), filepath.Join(destination, "package"), strings.TrimSuffix(asset.Filename, ".tar.gz")); err != nil {
		return errProof
	}
	identity, _ := json.Marshal(archiveIdentity{asset.SHA256, asset.SizeBytes})
	if os.WriteFile(filepath.Join(destination, "archive.json"), identity, 0644) != nil {
		return errProof
	}
	if os.WriteFile(filepath.Join(destination, releasepack.ManifestName), manifestBytes, 0644) != nil {
		return errProof
	}
	return nil
}

func extract(reader *tar.Reader, directory, root string) error {
	if root == "" || strings.ContainsAny(root, "/\\") || root == "." || root == ".." {
		return errProof
	}
	expected := map[string]int64{"observer": 200 << 20, "observer-journal-helper": 200 << 20,
		"LICENSE": 1 << 20, "START-HERE.md": 1 << 20, "run-observer.sh": 1 << 20}
	rootSeen := false
	var total int64
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errProof
		}
		if header.Name == root+"/" {
			if rootSeen || header.Typeflag != tar.TypeDir || header.Size != 0 || header.Mode != 0755 {
				return errProof
			}
			rootSeen = true
			continue
		}
		if !rootSeen || !strings.HasPrefix(header.Name, root+"/") {
			return errProof
		}
		name := strings.TrimPrefix(header.Name, root+"/")
		limit, ok := expected[name]
		if !ok || header.Typeflag != tar.TypeReg || header.Size < 1 || header.Size > limit || header.Mode&07000 != 0 {
			return errProof
		}
		total += header.Size
		if total > 220<<20 {
			return errProof
		}
		delete(expected, name)
		mode := os.FileMode(0644)
		if name == "observer" || name == "observer-journal-helper" || name == "run-observer.sh" {
			mode = 0755
		}
		file, err := os.OpenFile(filepath.Join(directory, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return errProof
		}
		_, copyErr := io.CopyN(file, reader, header.Size)
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			return errProof
		}
	}
	if !rootSeen || len(expected) != 0 {
		return errProof
	}
	return nil
}

func probe() (result error) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	data, err := boundedFile("/archive.json", 1024)
	if err != nil {
		return errProof
	}
	var identity archiveIdentity
	if json.Unmarshal(data, &identity) != nil {
		return errProof
	}
	verify := exec.CommandContext(ctx, "/package/observer", "version", "--release-manifest", "/release-manifest.json",
		"--archive-sha256", identity.SHA256, "--archive-size", strconv.FormatInt(identity.Size, 10))
	verify.Env = []string{"LANG=C", "LC_ALL=C", "TZ=UTC"}
	verify.WaitDelay = time.Second
	if verify.Run() != nil {
		return errProof
	}
	if err := probeEmpty(ctx, verify.Env); err != nil {
		return errProof
	}
	return probeHistory(ctx)
}

// Keep the original null-count missing-runtime proof independent of seeded history.
func probeEmpty(ctx context.Context, environment []string) (result error) {
	if ctx.Err() != nil {
		return errProof
	}
	command := exec.Command("/package/observer", "serve", "--listen", "127.0.0.1:9847", "--state-dir", "/state/observer", "--log-source", "system")
	command.Env = environment
	command.WaitDelay = time.Second
	if command.Start() != nil {
		return errProof
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	defer func() {
		if stop(command, done) != nil {
			result = errProof
		}
	}()
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return errProof }}
	defer client.CloseIdleConnections()
	var token string
	if !until(ctx, func() bool {
		contents, err := boundedFile("/state/observer/local-api.token", 256)
		if err != nil {
			return false
		}
		token = strings.TrimSpace(string(contents))
		return len(token) == 43 && current(ctx, client, token) == nil
	}) {
		return errProof
	}
	if _, status, err := get(ctx, client, "/api/v1/snapshots/current", "", 1<<20); err != nil || status != 401 {
		return errProof
	}
	if !until(ctx, func() bool {
		body, status, err := get(ctx, client, "/api/v1/logs/summary?range=1h", token, 256<<10)
		return err == nil && status == 200 && failedSummary(body) == nil
	}) {
		return errProof
	}
	if current(ctx, client, token) != nil {
		return errProof
	}
	if _, status, err := get(ctx, client, "/health/ready", "", 4096); err != nil || status != 200 {
		return errProof
	}
	return nil
}

func until(ctx context.Context, check func() bool) bool {
	for ctx.Err() == nil {
		if check() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(100 * time.Millisecond):
		}
	}
	return false
}

func boundedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errProof
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errProof
	}
	return data, nil
}

func get(ctx context.Context, client *http.Client, path, token string, limit int64) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:9847"+path, nil)
	if err != nil {
		return nil, 0, errProof
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, errProof
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, 0, errProof
	}
	return body, response.StatusCode, nil
}

func current(ctx context.Context, client *http.Client, token string) error {
	body, status, err := get(ctx, client, "/api/v1/snapshots/current", token, 1<<20)
	if err != nil || status != 200 {
		return errProof
	}
	var snapshot struct {
		Schema     string                     `json:"schema_version"`
		ObservedAt *time.Time                 `json:"observed_at"`
		Sections   map[string]json.RawMessage `json:"sections"`
	}
	if json.Unmarshal(body, &snapshot) != nil || snapshot.Schema != "observer-current-snapshot/v1" || snapshot.ObservedAt == nil || len(snapshot.Sections["overview"]) == 0 {
		return errProof
	}
	return nil
}

func failedSummary(body []byte) error {
	type status struct {
		Support    string     `json:"support_state"`
		Collection string     `json:"collection_state"`
		Reason     *string    `json:"reason_code"`
		Attempted  *time.Time `json:"attempted_at"`
	}
	var summary struct {
		Schema  string          `json:"schema_version"`
		Counts  json.RawMessage `json:"counts"`
		Sources []struct {
			Source   string          `json:"source"`
			Status   status          `json:"status"`
			Counts   json.RawMessage `json:"counts"`
			Covered  uint64          `json:"covered_seconds"`
			Coverage string          `json:"coverage_state"`
			Buckets  []struct {
				Counts   json.RawMessage `json:"counts"`
				Covered  uint64          `json:"covered_seconds"`
				Coverage string          `json:"coverage_state"`
			} `json:"buckets"`
		} `json:"sources"`
	}
	if json.Unmarshal(body, &summary) != nil || summary.Schema != "observer-log-summary/v1" || string(summary.Counts) != "null" {
		return errProof
	}
	found := false
	for _, source := range summary.Sources {
		if source.Source != "system" {
			continue
		}
		if found {
			return errProof
		}
		found = true
		state := source.Status
		if state.Support != "UNAVAILABLE" || state.Attempted == nil || state.Reason == nil ||
			!((state.Collection == "FAILED" && *state.Reason == "READER_FAILED") || (state.Collection == "NOT_RUN" && *state.Reason == "LOG_HELPER_UNAVAILABLE")) ||
			string(source.Counts) != "null" || source.Covered != 0 || source.Coverage == "FULL" || len(source.Buckets) == 0 {
			return errProof
		}
		for _, bucket := range source.Buckets {
			if string(bucket.Counts) != "null" || bucket.Covered != 0 || bucket.Coverage == "FULL" {
				return errProof
			}
		}
	}
	if !found {
		return errProof
	}
	return nil
}
