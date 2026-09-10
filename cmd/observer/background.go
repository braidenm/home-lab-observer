package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"time"

	"github.com/braidenm/home-lab-observer/internal/background"
	"github.com/braidenm/home-lab-observer/internal/diagnostics"
	"github.com/braidenm/home-lab-observer/internal/lifecycle"
	"github.com/braidenm/home-lab-observer/internal/localapi"
	"github.com/braidenm/home-lab-observer/internal/localauth"
)

const (
	backgroundStatusSchema        = "observer-background-status/v1"
	diagnosticsProbeResponseLimit = 32 * 1024
)

type backgroundManagerFactory func(background.Config) (background.Manager, error)
type managedServeCommand func([]string, io.Writer, io.Writer) int

type backgroundStatusOutput struct {
	SchemaVersion     string                             `json:"schema_version"`
	State             background.State                   `json:"state"`
	Code              background.Code                    `json:"code"`
	Scope             string                             `json:"scope"`
	SessionLimitation string                             `json:"session_limitation"`
	Readiness         background.Readiness               `json:"readiness"`
	Diagnostics       background.DiagnosticsAvailability `json:"diagnostics"`
	Registered        bool                               `json:"registered"`
	Running           bool                               `json:"running"`
}

type backgroundOptions struct {
	action         string
	installRoot    string
	stateDir       string
	listenAddress  string
	dockerEndpoint string
	force          bool
	json           bool
}

func runBackground(args []string, stdout, stderr io.Writer) int {
	return runBackgroundWith(args, stdout, stderr, background.NewManager, runServe)
}

func runBackgroundWith(args []string, stdout, stderr io.Writer, newManager backgroundManagerFactory, serveCommand managedServeCommand) int {
	options, code := parseBackgroundOptions(args, stdout, stderr)
	if code != 0 {
		return code
	}
	if options.action == "" {
		return 0
	}
	manager, err := newManager(background.Config{
		InstallRoot: options.installRoot,
		Stopper:     lifecycleStopper{},
		Probe:       localReadinessProbe{},
	})
	if err != nil {
		writeBackgroundError(stderr, err)
		return 1
	}
	if options.action == "run" {
		settings, err := manager.LoadSettings()
		if err != nil {
			writeBackgroundError(stderr, err)
			return 1
		}
		arguments := []string{"--internal-background-runtime", "--listen", settings.ListenAddress, "--state-dir", settings.StateDir}
		if settings.DockerEndpoint != "" {
			arguments = append(arguments, "--docker-endpoint", settings.DockerEndpoint)
		}
		return serveCommand(arguments, io.Discard, io.Discard)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var status background.Status
	switch options.action {
	case "enable":
		status, err = manager.Enable(ctx, background.Settings{StateDir: options.stateDir, ListenAddress: options.listenAddress, DockerEndpoint: options.dockerEndpoint})
	case "start":
		status, err = manager.Start(ctx)
	case "status":
		status, err = manager.Status(ctx)
	case "stop":
		status, err = manager.Stop(ctx, background.StopOptions{Force: options.force})
	case "restart":
		status, err = manager.Restart(ctx, background.StopOptions{Force: options.force})
	case "disable":
		status, err = manager.Disable(ctx, background.StopOptions{Force: options.force})
	}
	if err != nil {
		writeBackgroundError(stderr, err)
		return 1
	}
	if options.action == "enable" && !options.json {
		fmt.Fprintln(stdout, "Background mode is enabled for the current user session. It starts now and after a future login, but not before login.")
	}
	if err := writeBackgroundStatus(stdout, status, options.json); err != nil {
		fmt.Fprintln(stderr, "could not write background status (BACKGROUND_OUTPUT_FAILED)")
		return 1
	}
	return 0
}

func parseBackgroundOptions(args []string, stdout, stderr io.Writer) (backgroundOptions, int) {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printBackgroundUsage(stdout)
		return backgroundOptions{}, 0
	}
	options := backgroundOptions{action: args[0], listenAddress: "127.0.0.1:9847"}
	allowed := map[string]bool{"enable": true, "start": true, "status": true, "stop": true, "restart": true, "disable": true, "run": true}
	if !allowed[options.action] {
		printBackgroundUsage(stderr)
		return backgroundOptions{}, 2
	}
	flags := flag.NewFlagSet("background "+options.action, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&options.installRoot, "install-root", "", "absolute managed installation root")
	if options.action == "enable" {
		flags.StringVar(&options.stateDir, "state-dir", "", "dedicated observer state directory")
		flags.StringVar(&options.listenAddress, "listen", options.listenAddress, "explicit local IPv4 address and port")
		flags.StringVar(&options.dockerEndpoint, "docker-endpoint", "", "opt-in local Docker socket or named pipe")
	}
	if options.action == "stop" || options.action == "restart" || options.action == "disable" {
		flags.BoolVar(&options.force, "force", false, "explicitly authorize manager termination")
	}
	if options.action == "status" {
		flags.BoolVar(&options.json, "json", false, "write observer-background-status/v1 JSON")
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return backgroundOptions{}, 0
		}
		return backgroundOptions{}, 2
	}
	if flags.NArg() != 0 || options.installRoot == "" {
		fmt.Fprintln(stderr, "invalid background options: --install-root is required")
		return backgroundOptions{}, 2
	}
	if options.action == "enable" && options.stateDir == "" {
		tokenPath, err := localauth.DefaultTokenPath()
		if err != nil {
			fmt.Fprintln(stderr, "cannot resolve observer state directory (BACKGROUND_INVALID_SETTINGS)")
			return backgroundOptions{}, 1
		}
		options.stateDir = filepath.Dir(tokenPath)
	}
	return options, 0
}

func printBackgroundUsage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: observer background enable --install-root PATH [--state-dir PATH] [--listen 127.0.0.1:9847] [--docker-endpoint LOCAL_SOCKET]")
	fmt.Fprintln(writer, "       observer background <start|status|stop|restart|disable> --install-root PATH")
	fmt.Fprintln(writer, "Use --force only with stop, restart, or disable; it is never an automatic timeout fallback.")
}

func writeBackgroundStatus(writer io.Writer, status background.Status, asJSON bool) error {
	value := backgroundStatusOutput{
		SchemaVersion: backgroundStatusSchema, State: status.State, Code: status.Code, Scope: status.Scope,
		SessionLimitation: status.SessionLimitation, Readiness: status.Readiness, Diagnostics: status.Diagnostics,
		Registered: status.Registered, Running: status.Running,
	}
	if asJSON {
		return json.NewEncoder(writer).Encode(value)
	}
	_, err := fmt.Fprintf(writer, "Background: %s (%s)\nScope: current user session; %s\nReadiness: %s\nDiagnostics: %s\n", status.State, status.Code, status.SessionLimitation, status.Readiness, status.Diagnostics)
	return err
}

func writeBackgroundError(writer io.Writer, err error) {
	code := background.CodeManagerOperationFailed
	var value *background.Error
	if errors.As(err, &value) {
		code = value.Code
	}
	fmt.Fprintf(writer, "background command failed (%s)\n", code)
}

type lifecycleStopper struct{}

func (lifecycleStopper) RequestStop(ctx context.Context, stateDir string) error {
	return lifecycle.RequestStop(ctx, stateDir)
}

type localReadinessProbe struct{}

func (localReadinessProbe) Probe(ctx context.Context, settings background.Settings) background.ProbeStatus {
	result := background.ProbeStatus{Readiness: background.ReadinessUnreachable, Diagnostics: background.DiagnosticsUnknown}
	if localapi.ValidateBind(settings.ListenAddress) != nil {
		return result
	}
	probeContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	request, err := http.NewRequestWithContext(probeContext, http.MethodGet, "http://"+settings.ListenAddress+"/health/ready", nil)
	if err != nil {
		return result
	}
	response, err := client.Do(request)
	if err != nil {
		return result
	}
	var readiness struct {
		Status string `json:"status"`
	}
	validReadiness := decodeBoundedJSON(response, 1024, &readiness, true)
	if validReadiness && response.StatusCode == http.StatusOK && readiness.Status == "UP" {
		result.Readiness = background.ReadinessReady
	} else if validReadiness && response.StatusCode == http.StatusServiceUnavailable && readiness.Status == "NOT_READY" {
		result.Readiness = background.ReadinessNotReady
	} else {
		return result
	}
	token, err := localauth.Load(filepath.Join(settings.StateDir, "local-api.token"))
	if err != nil {
		return result
	}
	diagnosticsRequest, err := http.NewRequestWithContext(probeContext, http.MethodGet, "http://"+settings.ListenAddress+"/api/v1/diagnostics/health", nil)
	if err != nil {
		return result
	}
	diagnosticsRequest.Header.Set("Authorization", "Bearer "+token)
	diagnosticsResponse, err := client.Do(diagnosticsRequest)
	if err != nil || diagnosticsResponse.StatusCode != http.StatusOK {
		if diagnosticsResponse != nil {
			_ = diagnosticsResponse.Body.Close()
		}
		return result
	}
	var health diagnosticsHealthProbe
	if !decodeBoundedJSON(diagnosticsResponse, diagnosticsProbeResponseLimit, &health, false) || !health.valid() {
		return result
	}
	switch health.State {
	case "AVAILABLE":
		result.Diagnostics = background.DiagnosticsAvailable
	case "UNAVAILABLE":
		result.Diagnostics = background.DiagnosticsUnavailable
	case "DISABLED":
		result.Diagnostics = background.DiagnosticsDisabled
	}
	return result
}

type diagnosticsHealthProbe struct {
	SchemaVersion string  `json:"schema_version"`
	GeneratedAt   string  `json:"generated_at"`
	Enabled       bool    `json:"enabled"`
	Available     bool    `json:"available"`
	State         string  `json:"state"`
	ReasonCode    *string `json:"reason_code"`
	Limits        struct {
		MaxFiles       int   `json:"max_files"`
		MaxFileBytes   int64 `json:"max_file_bytes"`
		MaxTotalBytes  int64 `json:"max_total_bytes"`
		MaxRecordBytes int64 `json:"max_record_bytes"`
		MaxAgeSeconds  int64 `json:"max_age_seconds"`
	} `json:"limits"`
	Usage struct {
		TotalBytes int64 `json:"total_bytes"`
		FileCount  int   `json:"file_count"`
	} `json:"usage"`
	Counters struct {
		DroppedRecords uint64 `json:"dropped_records"`
		WriteFailures  uint64 `json:"write_failures"`
	} `json:"counters"`
	Policy struct {
		DataClassification   string `json:"data_classification"`
		ContainsLogContents  bool   `json:"contains_log_contents"`
		ContainsPaths        bool   `json:"contains_paths"`
		RemoteUploadEligible bool   `json:"remote_upload_eligible"`
	} `json:"policy"`
}

func (health diagnosticsHealthProbe) valid() bool {
	if health.SchemaVersion != "observer-diagnostics-health/v1" || health.GeneratedAt == "" {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, health.GeneratedAt); err != nil {
		return false
	}
	if health.Limits.MaxFiles != diagnostics.MaxFiles || health.Limits.MaxFileBytes != diagnostics.MaxFileBytes || health.Limits.MaxTotalBytes != diagnostics.MaxTotalBytes || health.Limits.MaxRecordBytes != diagnostics.MaxRecordBytes || health.Limits.MaxAgeSeconds != diagnostics.MaxAgeSeconds {
		return false
	}
	if health.Usage.TotalBytes < 0 || health.Usage.TotalBytes > health.Limits.MaxTotalBytes || health.Usage.FileCount < 0 || health.Usage.FileCount > health.Limits.MaxFiles {
		return false
	}
	if health.Policy.DataClassification != "PUBLIC_METADATA" || health.Policy.ContainsLogContents || health.Policy.ContainsPaths || health.Policy.RemoteUploadEligible {
		return false
	}
	switch health.State {
	case "AVAILABLE":
		return health.Enabled && health.Available && health.ReasonCode == nil
	case "UNAVAILABLE":
		return health.Enabled && !health.Available && health.ReasonCode != nil && (*health.ReasonCode == "DIAGNOSTICS_UNAVAILABLE" || *health.ReasonCode == "UNSAFE_DIAGNOSTICS_PATH")
	case "DISABLED":
		return !health.Enabled && !health.Available && health.ReasonCode != nil && *health.ReasonCode == "DIAGNOSTICS_DISABLED" && health.Usage.TotalBytes == 0 && health.Usage.FileCount == 0
	default:
		return false
	}
}

func decodeBoundedJSON(response *http.Response, maximum int64, destination any, closed bool) bool {
	defer response.Body.Close()
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || response.ContentLength > maximum {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximum+1))
	if err != nil || int64(len(body)) > maximum {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if closed {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(destination); err != nil {
		return false
	}
	var trailing any
	return errors.Is(decoder.Decode(&trailing), io.EOF)
}
