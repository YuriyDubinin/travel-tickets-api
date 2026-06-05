// Package service contains application logic that orchestrates domain entities,
// repositories, and outbound integrations.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/example/travel-tickets-api/internal/domain"
	"github.com/example/travel-tickets-api/internal/integration/travelpayouts"
)

// PricesProvider fetches cached flight offers for a single route. Implemented by
// the travelpayouts client.
type PricesProvider interface {
	PricesForDates(ctx context.Context, params travelpayouts.Params) ([]domain.FlightOffer, error)
}

// OfferStore persists collected offers. Implemented by the postgres
// FlightOfferRepository.
type OfferStore interface {
	UpsertMany(ctx context.Context, offers []domain.FlightOffer) (int64, error)
}

// CollectorConfig configures a Collector (built from config.Worker in main).
type CollectorConfig struct {
	Origin       string
	Destinations []string
	MonthsAhead  int
	OneWay       bool
	RequestDelay time.Duration
}

// Collector fetches prices for the configured routes and upserts them.
type Collector struct {
	provider     PricesProvider
	store        OfferStore
	log          *slog.Logger
	origin       string
	destinations []string
	monthsAhead  int
	oneWay       bool
	reqDelay     time.Duration
}

// NewCollector constructs a Collector.
func NewCollector(provider PricesProvider, store OfferStore, log *slog.Logger, cfg CollectorConfig) *Collector {
	return &Collector{
		provider:     provider,
		store:        store,
		log:          log,
		origin:       cfg.Origin,
		destinations: cfg.Destinations,
		monthsAhead:  cfg.MonthsAhead,
		oneWay:       cfg.OneWay,
		reqDelay:     cfg.RequestDelay,
	}
}

// CollectResult summarizes a single collection cycle.
type CollectResult struct {
	RoutesOK     int
	RoutesEmpty  int
	RoutesFailed int
	Fetched      int
	Upserted     int64
}

// CollectOnce runs one full collection pass over destinations × upcoming months.
// A failure on one route does not abort the cycle: errors are accumulated and
// returned joined. An empty route result is expected (e.g. OVB→SIP) and is not
// an error.
func (c *Collector) CollectOnce(ctx context.Context) (CollectResult, error) {
	var (
		res  CollectResult
		errs []error
	)

	months := upcomingMonths(time.Now(), c.monthsAhead)

	for _, dest := range c.destinations {
		for _, month := range months {
			// Politeness delay between requests (ctx-aware).
			if err := sleep(ctx, c.reqDelay); err != nil {
				return res, err
			}

			offers, err := c.provider.PricesForDates(ctx, travelpayouts.Params{
				Origin:      c.origin,
				Destination: dest,
				DepartureAt: month,
				OneWay:      c.oneWay,
				Limit:       100,
			})
			if err != nil {
				res.RoutesFailed++
				errs = append(errs, fmt.Errorf("%s-%s %s: %w", c.origin, dest, month, err))
				c.log.Error("collect route failed",
					"origin", c.origin, "destination", dest, "month", month, "error", err)
				continue
			}

			if len(offers) == 0 {
				res.RoutesEmpty++
				c.log.Info("collect route empty (likely no flights)",
					"origin", c.origin, "destination", dest, "month", month)
				continue
			}

			written, err := c.store.UpsertMany(ctx, offers)
			if err != nil {
				res.RoutesFailed++
				errs = append(errs, fmt.Errorf("%s-%s %s upsert: %w", c.origin, dest, month, err))
				c.log.Error("collect route upsert failed",
					"origin", c.origin, "destination", dest, "month", month, "error", err)
				continue
			}

			res.RoutesOK++
			res.Fetched += len(offers)
			res.Upserted += written
			c.log.Info("collect route ok",
				"origin", c.origin, "destination", dest, "month", month,
				"fetched", len(offers), "written", written)
		}
	}

	return res, errors.Join(errs...)
}

// upcomingMonths returns "YYYY-MM" for the current month and the next n-1 months.
func upcomingMonths(now time.Time, n int) []string {
	if n < 1 {
		n = 1
	}
	y, m, _ := now.Date()
	base := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	months := make([]string, 0, n)
	for i := 0; i < n; i++ {
		months = append(months, base.AddDate(0, i, 0).Format("2006-01"))
	}
	return months
}

// sleep waits for d, returning early with ctx.Err() if ctx is cancelled.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
