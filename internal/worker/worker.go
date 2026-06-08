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

// Worker periodically collects flight prices and then publishes new offers.
type Worker struct {
	collector *service.Collector
	publisher *service.Publisher // may be nil when Telegram is disabled
	interval  time.Duration
	log       *slog.Logger
}

// NewWorker constructs a Worker. publisher may be nil, in which case the worker
// only collects (no publishing).
func NewWorker(collector *service.Collector, publisher *service.Publisher, interval time.Duration, log *slog.Logger) *Worker {
	return &Worker{collector: collector, publisher: publisher, interval: interval, log: log}
}

// Run executes one cycle immediately, then every interval, until ctx is
// cancelled. It never returns: each cycle is isolated, and a failure or panic is
// logged without stopping the worker.
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

// runCycle collects offers and then publishes pending ones, recovering from
// panics so a single bad cycle cannot bring the worker down.
func (w *Worker) runCycle(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			w.log.Error("worker cycle panic", "error", rec, "stack", string(debug.Stack()))
		}
	}()

	// 1. Collect prices into the database.
	start := time.Now()
	cres, cerr := w.collector.CollectOnce(ctx)
	w.logResult("collect cycle finished", time.Since(start), cerr,
		"routes_ok", cres.RoutesOK,
		"routes_empty", cres.RoutesEmpty,
		"routes_failed", cres.RoutesFailed,
		"fetched", cres.Fetched,
		"upserted", cres.Upserted,
	)

	// 2. Publish not-yet-published offers to Telegram.
	if w.publisher == nil || ctx.Err() != nil {
		return
	}
	start = time.Now()
	pres, perr := w.publisher.PublishPending(ctx)
	w.logResult("publish cycle finished", time.Since(start), perr,
		"fetched", pres.Fetched,
		"published", pres.Published,
		"failed", pres.Failed,
	)
}

// logResult logs a cycle result, treating context cancellation as a normal stop
// rather than an error.
func (w *Worker) logResult(msg string, dur time.Duration, err error, attrs ...any) {
	attrs = append([]any{"duration_ms", dur.Milliseconds()}, attrs...)
	switch {
	case err == nil:
		w.log.Info(msg, attrs...)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// Shutting down — not a real failure.
	default:
		w.log.Error(msg+" with errors", append(attrs, "error", err)...)
	}
}
