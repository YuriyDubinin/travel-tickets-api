// Command api is the entry point for the travel-tickets HTTP service. It loads
// configuration, wires the layers together, and runs the HTTP server until it
// receives an interrupt signal.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/example/travel-tickets-api/internal/config"
	"github.com/example/travel-tickets-api/internal/integration/travelpayouts"
	"github.com/example/travel-tickets-api/internal/repository/postgres"
	"github.com/example/travel-tickets-api/internal/service"
	transporthttp "github.com/example/travel-tickets-api/internal/transport/http"
	"github.com/example/travel-tickets-api/internal/transport/http/handler"
	"github.com/example/travel-tickets-api/internal/worker"
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

	// Repositories.
	offerRepo := postgres.NewFlightOfferRepository(pool)

	// Background price-collector worker (optional). It starts only when enabled
	// and an Aviasales token is configured; otherwise the service runs without it.
	var wg sync.WaitGroup
	if cfg.Worker.Enabled && cfg.Aviasales.Token != "" {
		tpClient := travelpayouts.NewClient(travelpayouts.Config{
			BaseURL:  cfg.Aviasales.BaseURL,
			Token:    cfg.Aviasales.Token,
			Marker:   cfg.Aviasales.Marker,
			Currency: cfg.Aviasales.Currency,
			Timeout:  cfg.Aviasales.HTTPTimeout,
		})
		collector := service.NewCollector(tpClient, offerRepo, log, service.CollectorConfig{
			Origin:       cfg.Worker.Origin,
			Destinations: cfg.Worker.Destinations,
			MonthsAhead:  cfg.Worker.MonthsAhead,
			OneWay:       cfg.Worker.OneWay,
			RequestDelay: cfg.Worker.RequestDelay,
		})
		w := worker.NewWorker(collector, cfg.Worker.Interval, log)

		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Run(ctx)
		}()
		log.Info("price worker enabled",
			"origin", cfg.Worker.Origin,
			"destinations", cfg.Worker.Destinations,
			"interval", cfg.Worker.Interval.String())
	} else {
		log.Warn("price worker disabled",
			"worker_enabled", cfg.Worker.Enabled,
			"has_token", cfg.Aviasales.Token != "")
	}

	// Handlers.
	healthHandler := handler.NewHealthHandler()
	offersHandler := handler.NewOffersHandler(service.NewOfferService(offerRepo), log)

	// Router with all dependencies injected.
	router := transporthttp.NewRouter(transporthttp.Deps{
		Logger:        log,
		HealthHandler: healthHandler,
		OffersHandler: offersHandler,
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

	// Wait for the worker (if any) to finish its graceful stop.
	wg.Wait()

	log.Info("service stopped")
}
