package commands

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
	"time"

	"agentcollab.local/agentcollab/internal/auth"
	"agentcollab.local/agentcollab/internal/config"
	"agentcollab.local/agentcollab/internal/httpapi"
	"agentcollab.local/agentcollab/internal/realtime"
	"agentcollab.local/agentcollab/internal/storage"
	store "agentcollab.local/agentcollab/internal/store/sqlite"
)

const shutdownTimeout = 10 * time.Second

func RunServe(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "config/hub.yaml", "path to Hub YAML configuration")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.LoadHub(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "serve: %v\n", err)
		return 1
	}
	if err := storage.EnsureLayout(cfg.StorageRoot); err != nil {
		fmt.Fprintf(stderr, "serve: storage check failed: %v\n", err)
		return 1
	}
	if cfg.RequireSMB {
		mount, err := storage.DetectMount(cfg.StorageRoot)
		if err != nil {
			fmt.Fprintf(stderr, "serve: mount check failed: %v\n", err)
			return 1
		}
		if !mount.IsSMB() {
			fmt.Fprintf(stderr, "serve: storage root must be on SMB; detected %s at %s\n", mount.Type, mount.MountPoint)
			return 1
		}
	}

	lock, err := storage.AcquireHubLock(cfg.StorageRoot)
	if err != nil {
		fmt.Fprintf(stderr, "serve: lock check failed: %v\n", err)
		return 1
	}
	defer func() {
		if err := lock.Close(); err != nil {
			fmt.Fprintf(stderr, "serve: release lock: %v\n", err)
		}
	}()

	if err := ensureHubIdentity(&cfg); err != nil {
		fmt.Fprintf(stderr, "serve: Hub identity: %v\n", err)
		return 1
	}
	databasePath, err := storage.ResolveWithin(cfg.StorageRoot, cfg.DatabasePath)
	if err != nil {
		fmt.Fprintf(stderr, "serve: database path: %v\n", err)
		return 1
	}
	database, err := store.Open(ctx, databasePath)
	if err != nil {
		fmt.Fprintf(stderr, "serve: open database: %v\n", err)
		return 1
	}
	defer func() {
		if err := database.Close(); err != nil {
			fmt.Fprintf(stderr, "serve: close database: %v\n", err)
		}
	}()

	logPath, err := storage.ResolveWithin(cfg.StorageRoot, cfg.LogPath)
	if err != nil {
		fmt.Fprintf(stderr, "serve: log path: %v\n", err)
		return 1
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintf(stderr, "serve: open log: %v\n", err)
		return 1
	}
	defer logFile.Close()
	logger := slog.New(slog.NewJSONHandler(logFile, nil))

	serveCtx, cancelServe := context.WithCancel(ctx)
	defer cancelServe()
	authService := auth.NewService(store.NewAuthRepository(database))
	repository := store.NewCollabRepository(database)
	broker := realtime.NewBroker(256)
	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           httpapi.NewRouterWithConfig(authService, repository, cfg, broker),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          log.New(logFile, "http: ", log.LstdFlags|log.LUTC),
		BaseContext: func(net.Listener) context.Context {
			return serveCtx
		},
	}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		fmt.Fprintf(stderr, "serve: listen %s: %v\n", server.Addr, err)
		return 1
	}

	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.Serve(listener)
	}()
	logger.Info("hub started", "listen_address", listener.Addr().String(), "storage_root", cfg.StorageRoot)
	fmt.Fprintf(stdout, "hub_listening=%s storage_root=%s\n", listener.Addr().String(), cfg.StorageRoot)

	select {
	case err := <-serveErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("hub stopped unexpectedly", "error", err)
			fmt.Fprintf(stderr, "serve: http server: %v\n", err)
			return 1
		}
	case <-ctx.Done():
		cancelServe()
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
		err := server.Shutdown(shutdownCtx)
		cancelShutdown()
		if err != nil {
			_ = server.Close()
			logger.Error("hub shutdown failed", "error", err)
			fmt.Fprintf(stderr, "serve: shutdown: %v\n", err)
			return 1
		}
		if err := <-serveErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("hub stopped unexpectedly", "error", err)
			fmt.Fprintf(stderr, "serve: http server: %v\n", err)
			return 1
		}
	}

	logger.Info("hub stopped")
	fmt.Fprintln(stdout, "hub_stopped=ok")
	return 0
}
