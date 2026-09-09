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
	"strconv"
	"syscall"
	"time"

	"github.com/braidenm/home-lab-observer/internal/collector"
	"github.com/braidenm/home-lab-observer/internal/history"
	"github.com/braidenm/home-lab-observer/internal/localapi"
	"github.com/braidenm/home-lab-observer/internal/localauth"
	"github.com/braidenm/home-lab-observer/internal/scheduler"
)

func runServe(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	listen := flags.String("listen", "127.0.0.1:9847", "local IPv4 address and port")
	stateDir := flags.String("state-dir", "", "dedicated observer state directory on a local disk")
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
	logger := slog.New(slog.NewJSONHandler(stderr, nil))
	if err := serve(ctx, *listen, resolved, stdout, logger); err != nil {
		// Errors may contain a private file path or OS data. Keep ordinary logs code-owned.
		logger.Error("observer_stopped", "code", "SERVICE_FAILED")
		return 1
	}
	return 0
}

func serve(ctx context.Context, address, stateDir string, output io.Writer, logger *slog.Logger) error {
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
	cfg := collector.DefaultConfig()
	cfg.CollectorVersion, cfg.MaxProcesses = version, 200
	runtime, err := scheduler.New(collector.New(collector.RealClock{}, collector.GopsutilProvider{}, cfg).Collect, store, scheduler.DefaultConfig())
	if err != nil {
		_ = store.Close()
		return err
	}
	_, portText, _ := net.SplitHostPort(address)
	port, _ := strconv.Atoi(portText)
	handler, err := localapi.NewHandler(localapi.Config{Port: port, Token: token, Version: version, Source: runtime, History: store, Now: time.Now})
	if err != nil {
		_ = store.Close()
		return err
	}
	// Drain HTTP requests before closing the store in the deferred scheduler stop.
	if err := runtime.Start(context.WithoutCancel(ctx)); err != nil {
		_ = store.Close()
		return err
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := runtime.Stop(stopCtx); err != nil {
			logger.Error("collection_shutdown_failed", "code", "SHUTDOWN_TIMEOUT")
		}
	}()
	server := &http.Server{Handler: handler, ErrorLog: log.New(safeHTTPLog{logger}, "", 0), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()
	fmt.Fprintf(output, "Home Lab Observer: http://%s\nLocal access token file: %s\nOpen the file locally and paste its token into the dashboard. Press Ctrl+C to stop.\n", address, tokenPath)
	logger.Info("observer_started", "version", version)
	select {
	case err := <-stopped:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
		logger.Info("observer_stopped", "code", "SHUTDOWN_COMPLETE")
		return nil
	}
}

type safeHTTPLog struct{ logger *slog.Logger }

func (writer safeHTTPLog) Write(message []byte) (int, error) {
	writer.logger.Error("http_server_error", "code", "HTTP_INTERNAL_ERROR")
	return len(message), nil
}
