// Command api is the entry point for the travel-tickets HTTP service. It loads
// configuration, wires the layers together, and runs the HTTP server until it
// receives an interrupt signal.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/travel-tickets-api/internal/config"
	"github.com/example/travel-tickets-api/internal/repository/postgres"
	transporthttp "github.com/example/travel-tickets-api/internal/transport/http"
	"github.com/example/travel-tickets-api/internal/transport/http/handler"
	"github.com/example/travel-tickets-api/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(cfg.Log.Level, cfg.App.Env)
	log.Info("service starting", "env", cfg.App.Env, "http_port", cfg.HTTP.Port)

	// Cancel the root context on SIGINT/SIGTERM to trigger graceful shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Database connection pool.
	pool, err := postgres.NewPool(ctx, cfg.Postgres.DSN(), cfg.Postgres.MaxConns)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	log.Info("database connected")

	// Schema migrations.
	if err := postgres.RunMigrations(cfg.Postgres.DSN(), "migrations", log); err != nil {
		log.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Handlers.
	healthHandler := handler.NewHealthHandler()

	// Router with all dependencies injected.
	router := transporthttp.NewRouter(transporthttp.Deps{
		Logger:        log,
		HealthHandler: healthHandler,
	})

	// HTTP server.
	srv := transporthttp.NewServer(transporthttp.HTTPConfig{
		Port:         cfg.HTTP.Port,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}, router, log)

	log.Info("http server starting on :" + cfg.HTTP.Port)
	log.Info("ready")

	if err := srv.Run(ctx); err != nil {
		log.Error("server error", "error", err)
		os.Exit(1)
	}

	log.Info("service stopped")
}
