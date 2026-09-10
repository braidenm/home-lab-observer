package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/braidenm/home-lab-observer/internal/buildidentity"
	"github.com/braidenm/home-lab-observer/internal/collector"
	"github.com/braidenm/home-lab-observer/internal/containerobs"
	"github.com/braidenm/home-lab-observer/internal/diagnostics"
	"github.com/braidenm/home-lab-observer/internal/history"
	"github.com/braidenm/home-lab-observer/internal/lifecycle"
	"github.com/braidenm/home-lab-observer/internal/localapi"
	"github.com/braidenm/home-lab-observer/internal/localauth"
	"github.com/braidenm/home-lab-observer/internal/logobs"
	"github.com/braidenm/home-lab-observer/internal/logprocess"
	"github.com/braidenm/home-lab-observer/internal/scheduler"
)

func runServe(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	listen := flags.String("listen", "127.0.0.1:9847", "local IPv4 address and port")
	stateDir := flags.String("state-dir", "", "dedicated observer state directory on a local disk")
	dockerEndpoint := flags.String("docker-endpoint", "", "opt-in local Docker Unix socket or Windows named pipe; disabled by default")
	internalBackground := flags.Bool("internal-background-runtime", false, "run the fixed managed background runtime")
	var sourceFlags logSourceFlags
	flags.Var(&sourceFlags, "log-source", "opt-in native metadata source: system or application; repeat per source")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || localapi.ValidateBind(*listen) != nil {
		fmt.Fprintln(stderr, "invalid serve options: use an explicit 127.0.0.1 address and port")
		return 2
	}
	if err := containerobs.ValidateEndpoint(*dockerEndpoint); err != nil {
		fmt.Fprintln(stderr, "invalid Docker endpoint: use an explicit local unix:///absolute/path socket or npipe:////./pipe/name")
		return 2
	}
	sources, sourceErr := logobs.ParseSources(goruntime.GOOS, sourceFlags)
	if sourceErr != nil {
		fmt.Fprintln(stderr, "invalid native log source configuration")
		return 2
	}
	if *stateDir == "" {
		tokenPath, err := localauth.DefaultTokenPath()
		if err != nil {
			fmt.Fprintln(stderr, "cannot resolve observer state directory")
			return 1
		}
		*stateDir = filepath.Dir(tokenPath)
	}
	resolved, err := filepath.Abs(*stateDir)
	if err != nil {
		fmt.Fprintln(stderr, "cannot resolve observer state directory")
		return 1
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *internalBackground {
		stdout, stderr = io.Discard, io.Discard
	}
	logger := slog.New(slog.NewJSONHandler(stderr, nil))
	if err := serveRuntime(ctx, *listen, resolved, stdout, logger, serveRuntimeOptions{managed: *internalBackground, dockerEndpoint: *dockerEndpoint, logSources: sources}); err != nil {
		// Errors may contain a private file path or OS data. Keep ordinary logs code-owned.
		logger.Error("observer_stopped", "code", "SERVICE_FAILED")
		return 1
	}
	return 0
}

func serve(ctx context.Context, address, stateDir string, output io.Writer, logger *slog.Logger, optionalContainers ...*containerobs.Collector) error {
	options := serveRuntimeOptions{}
	if len(optionalContainers) > 0 {
		options.containers = optionalContainers[0]
	}
	return serveRuntime(ctx, address, stateDir, output, logger, options)
}

type serveRuntimeOptions struct {
	managed        bool
	dockerEndpoint string
	containers     *containerobs.Collector
	newLifecycle   func(lifecycle.Config) (managedLifecycle, error)
	logSources     []logobs.Source
	logReader      logobs.Reader
	newLogs        func(logobs.Config) (logRuntime, error)
}

type logRuntime interface {
	Start(context.Context) error
	Stop(context.Context) error
	Current() logobs.Snapshot
	Summary(context.Context, logobs.SummaryQuery) (logobs.Summary, error)
}

// Both collectors borrow history. Only their composition root can close it
// after both have joined; scheduler's ordinary standalone ownership is unchanged.
type borrowedHistory struct{ *history.Store }

func (borrowedHistory) Close() error { return nil }

type collectionStopper interface{ Stop(context.Context) error }

func stopCollectors(logs, host collectionStopper, closeStore func() error) error {
	logCtx, logCancel := context.WithTimeout(context.Background(), 5*time.Second)
	logErr := logs.Stop(logCtx)
	logCancel()
	hostCtx, hostCancel := context.WithTimeout(context.Background(), 15*time.Second)
	hostErr := host.Stop(hostCtx)
	hostCancel()
	if err := errors.Join(logErr, hostErr); err != nil {
		// Do not close storage beneath an unjoined collector. Process exit owns
		// final OS cleanup on this failed-shutdown path.
		return err
	}
	return closeStore()
}

type managedLifecycle interface {
	StopRequested() <-chan struct{}
	Close() error
	Abandon() error
}

func serveRuntime(ctx context.Context, address, stateDir string, output io.Writer, logger *slog.Logger, options serveRuntimeOptions) error {
	// Bind before opening state so a second process cannot mutate an active instance's store.
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		fmt.Fprintln(output, "Could not listen. Check whether another observer is already running on this port.")
		return err
	}
	defer listener.Close()
	tokenPath := filepath.Join(stateDir, "local-api.token")
	token, _, err := localauth.Ensure(tokenPath)
	if err != nil {
		fmt.Fprintln(output, "Could not open the local access token. Check ownership and permissions of the observer state directory.")
		return err
	}
	store, err := history.Open(ctx, history.DefaultConfig(filepath.Join(stateDir, "history.sqlite")), nil)
	if err != nil {
		fmt.Fprintln(output, "Could not open observation history. Use a writable local disk and inspect the observer state directory.")
		return err
	}
	var endpoint managedLifecycle
	lifecycleFinalized := false
	if options.managed {
		newLifecycle := options.newLifecycle
		if newLifecycle == nil {
			newLifecycle = func(config lifecycle.Config) (managedLifecycle, error) {
				return lifecycle.New(config)
			}
		}
		endpoint, err = newLifecycle(lifecycle.Config{StateDir: stateDir})
		if err != nil {
			_ = store.Close()
			return err
		}
		defer func() {
			if !lifecycleFinalized {
				_ = endpoint.Abandon()
			}
		}()
	}
	diagnosticWriter := diagnostics.New(diagnostics.Config{StateDir: stateDir, Enabled: options.managed})
	defer diagnosticWriter.Close()
	diagnosticWriter.Record(diagnostics.Event{Kind: diagnostics.EventRuntimeStarted, Code: diagnostics.CodeOK, Version: version})
	stopCode := diagnostics.CodeFailed
	defer func() {
		diagnosticWriter.Record(diagnostics.Event{Kind: diagnostics.EventRuntimeStopped, Code: stopCode, Version: version})
	}()
	if endpoint != nil {
		baseContext := ctx
		managedContext, managedCancel := context.WithCancel(baseContext)
		defer managedCancel()
		go func() {
			select {
			case <-endpoint.StopRequested():
				diagnosticWriter.Record(diagnostics.Event{Kind: diagnostics.EventStopRequested, Code: diagnostics.CodeOK, Version: version})
				managedCancel()
			case <-managedContext.Done():
			}
		}()
		ctx = managedContext
	}
	cfg := collector.DefaultConfig()
	cfg.CollectorVersion, cfg.MaxProcesses = version, 200
	containers := options.containers
	runtimeResourcesJoined := true
	if containers == nil {
		containers, err = containerobs.New(containerobs.Config{Endpoint: options.dockerEndpoint})
		if err != nil {
			_ = store.Close()
			return err
		}
		defer func() {
			if runtimeResourcesJoined {
				containers.Close()
			}
		}()
	}
	collect := collectWithContainers(collector.New(collector.RealClock{}, collector.GopsutilProvider{}, cfg).Collect, containers)
	runtime, err := scheduler.New(collect, borrowedHistory{store}, scheduler.DefaultConfig())
	if err != nil {
		_ = store.Close()
		return err
	}
	newLogs := options.newLogs
	if newLogs == nil {
		newLogs = func(config logobs.Config) (logRuntime, error) { return logobs.New(config) }
	}
	reader := options.logReader
	if reader == nil && len(options.logSources) > 0 {
		identity, identityErr := buildidentity.Resolve(releaseIdentity, version, commit, "observer", goruntime.GOOS, goruntime.GOARCH)
		if identityErr != nil {
			_ = store.Close()
			return identityErr
		}
		reader = logprocess.NewReader(identity.Version, identity.Commit, identity.HelperSHA256)
	}
	logs, err := newLogs(logobs.Config{Sources: options.logSources, Reader: reader, Store: store})
	if err != nil {
		_ = store.Close()
		return err
	}
	_, portText, _ := net.SplitHostPort(address)
	port, _ := strconv.Atoi(portText)
	handler, err := localapi.NewHandler(localapi.Config{Port: port, Token: token, Version: version, Source: runtime, History: store, ContainerSource: containers, LogSource: logs, LogSummarySource: logs, Diagnostics: diagnosticWriter, Now: time.Now})
	if err != nil {
		_ = store.Close()
		return err
	}
	// Drain HTTP requests before closing scheduler/store resources. The lifecycle
	// nonce is removed only after this explicit sequence completes successfully.
	if err := runtime.Start(context.WithoutCancel(ctx)); err != nil {
		_ = store.Close()
		return err
	}
	runtimeResourcesJoined = false
	stopCollection := func(httpJoined bool) error {
		return stopCollectors(logs, runtime, func() error {
			if !httpJoined {
				// Closing network connections does not join their handlers. Keep
				// borrowed storage/container state alive until process exit.
				return nil
			}
			runtimeResourcesJoined = true
			return store.Close()
		})
	}
	if len(options.logSources) > 0 {
		if err := logs.Start(context.WithoutCancel(ctx)); err != nil {
			return errors.Join(err, stopCollection(true))
		}
	}
	server := &http.Server{Handler: handler, ErrorLog: log.New(safeHTTPLog{logger: logger, diagnostics: diagnosticWriter}, "", 0), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()
	diagnosticWriter.Record(diagnostics.Event{Kind: diagnostics.EventRuntimeReady, Code: diagnostics.CodeOK, Version: version})
	fmt.Fprintf(output, "Home Lab Observer: http://%s\nLocal access token file: %s\nOpen the file locally and paste its token into the dashboard. Press Ctrl+C to stop.\n", address, tokenPath)
	logger.Info("observer_started", "version", version)
	if err := finishHTTP(ctx, server, stopped, 10*time.Second, stopCollection); err != nil {
		var shutdownCode string
		stopCode, shutdownCode = classifyShutdownFailure(err)
		logger.Error("runtime_shutdown_failed", "code", shutdownCode)
		return err
	}
	if endpoint != nil {
		if err := endpoint.Close(); err != nil {
			return err
		}
		lifecycleFinalized = true
	}
	logger.Info("observer_stopped", "code", "SHUTDOWN_COMPLETE")
	stopCode = diagnostics.CodeOK
	return nil
}

// Serve returning does not mean active handlers have stopped borrowing state.
// Both exit paths drain first. The duration argument lets regressions exercise
// timeout policy quickly; production always supplies its fixed ten-second bound.
func finishHTTP(ctx context.Context, server *http.Server, stopped <-chan error, drainTimeout time.Duration, stopCollection func(httpJoined bool) error) error {
	var serveErr error
	unexpected := false
	select {
	case serveErr = <-stopped:
		unexpected = true
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	shutdownErr := server.Shutdown(shutdownCtx)
	cancel()
	if shutdownErr != nil {
		// Close cancels network activity, but cannot prove handler completion.
		_ = server.Close()
	}
	if !unexpected {
		serveErr = <-stopped
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
	} else if serveErr == nil || errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = errors.New("HTTP server stopped without a shutdown request")
	}
	stopErr := stopCollection(shutdownErr == nil)
	return errors.Join(shutdownErr, serveErr, stopErr)
}

func classifyShutdownFailure(err error) (diagnostics.ResultCode, string) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return diagnostics.CodeTimeout, "SHUTDOWN_TIMEOUT"
	}
	return diagnostics.CodeFailed, "SHUTDOWN_FAILED"
}

type safeHTTPLog struct {
	logger      *slog.Logger
	diagnostics *diagnostics.Writer
}

func (writer safeHTTPLog) Write(message []byte) (int, error) {
	writer.logger.Error("http_server_error", "code", "HTTP_INTERNAL_ERROR")
	if writer.diagnostics != nil {
		writer.diagnostics.Record(diagnostics.Event{Kind: diagnostics.EventHTTPServer, Code: diagnostics.CodeFailed, Version: version})
	}
	return len(message), nil
}
