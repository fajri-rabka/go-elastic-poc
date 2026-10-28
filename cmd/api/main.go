// Command api runs the Motorcycle Catalog REST API server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fajri-rabka/go-elasticsearch-poc/internal/config"
	"github.com/fajri-rabka/go-elasticsearch-poc/internal/handler"
	"github.com/fajri-rabka/go-elasticsearch-poc/internal/repository/elastic"
	"github.com/fajri-rabka/go-elasticsearch-poc/internal/service"
)

const (
	startupTimeout  = 10 * time.Second
	shutdownTimeout = 5 * time.Second
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("application startup failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	client, err := elastic.NewClient(cfg.ElasticsearchURL)
	if err != nil {
		return fmt.Errorf("create elasticsearch client: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := client.Close(closeCtx); err != nil {
			logger.Warn("close elasticsearch client", "error", err)
		}
	}()

	// Verify datastore connectivity and ensure the index is ready on startup.
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), startupTimeout)
	defer cancelStartup()

	info, err := client.Info().Do(startupCtx)
	if err != nil {
		return fmt.Errorf("check elasticsearch connection to %s: %w", cfg.ElasticsearchURL, err)
	}
	logger.Info("connected to elasticsearch",
		"cluster", info.ClusterName,
		"version", info.Version.Int,
	)

	repo := elastic.NewMotorcycleRepository(client, cfg.ElasticsearchIndex)
	created, err := repo.EnsureIndex(startupCtx)
	if err != nil {
		return fmt.Errorf("ensure index %q: %w", cfg.ElasticsearchIndex, err)
	}
	if created {
		logger.Info("index created", "index", cfg.ElasticsearchIndex)
	} else {
		logger.Info("index ready (existing mapping verified)", "index", cfg.ElasticsearchIndex)
	}

	svc := service.NewMotorcycleService(repo)
	h := handler.NewMotorcycleHandler(svc, logger)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("REST API server listening", "addr", fmt.Sprintf("http://%s", cfg.HTTPAddr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	shutdownSig := make(chan os.Signal, 1)
	signal.Notify(shutdownSig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("http server failure: %w", err)
	case sig := <-shutdownSig:
		logger.Info("received termination signal, shutting down gracefully", "signal", sig.String())
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	logger.Info("server stopped cleanly")
	return nil
}
