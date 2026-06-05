// Package worker runs background jobs on a schedule.
package worker

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/example/travel-tickets-api/internal/service"
)

// Worker periodically runs the price collector.
type Worker struct {
	collector *service.Collector
	interval  time.Duration
	log       *slog.Logger
}

// NewWorker constructs a Worker.
func NewWorker(collector *service.Collector, interval time.Duration, log *slog.Logger) *Worker {
	return &Worker{collector: collector, interval: interval, log: log}
}

// Run executes one collection cycle immediately, then every interval, until ctx
// is cancelled. It never returns: each cycle is isolated, and a failure or panic
// is logged without stopping the worker.
func (w *Worker) Run(ctx context.Context) {
	w.log.Info("worker started", "interval", w.interval.String())

	// First run immediately, so we don't wait a full interval for initial data.
	w.runCycle(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("worker stopped")
			return
		case <-ticker.C:
			w.runCycle(ctx)
		}
	}
}

// runCycle runs one collection cycle, recovering from panics so a single bad
// cycle cannot bring the worker down.
func (w *Worker) runCycle(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			w.log.Error("worker cycle panic", "error", rec, "stack", string(debug.Stack()))
		}
	}()

	start := time.Now()
	res, err := w.collector.CollectOnce(ctx)
	durMs := time.Since(start).Milliseconds()

	attrs := []any{
		"duration_ms", durMs,
		"routes_ok", res.RoutesOK,
		"routes_empty", res.RoutesEmpty,
		"routes_failed", res.RoutesFailed,
		"fetched", res.Fetched,
		"upserted", res.Upserted,
	}

	switch {
	case err == nil:
		w.log.Info("worker cycle finished", attrs...)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// Shutting down — not a real failure.
	default:
		w.log.Error("worker cycle finished with errors", append(attrs, "error", err)...)
	}
}
