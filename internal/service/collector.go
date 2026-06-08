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

// collectionWindowDays is the size of the departure-date window the collector
// scans on each pass: from today through today+collectionWindowDays (inclusive).
const collectionWindowDays = 14

// CollectOnce runs one full collection pass over destinations × the upcoming
// departure dates (today through today+collectionWindowDays). A failure on one
// route does not abort the cycle: errors are accumulated and returned joined. An
// empty result for a date is expected (e.g. OVB→SIP) and is not an error.
func (c *Collector) CollectOnce(ctx context.Context) (CollectResult, error) {
	var (
		res  CollectResult
		errs []error
	)

	dates := upcomingDates(time.Now(), collectionWindowDays)

	for _, dest := range c.destinations {
		for _, date := range dates {
			// Politeness delay between requests (ctx-aware).
			if err := sleep(ctx, c.reqDelay); err != nil {
				return res, err
			}

			offers, err := c.provider.PricesForDates(ctx, travelpayouts.Params{
				Origin:      c.origin,
				Destination: dest,
				DepartureAt: date,
				OneWay:      c.oneWay,
				Limit:       100,
			})
			if err != nil {
				res.RoutesFailed++
				errs = append(errs, fmt.Errorf("%s-%s %s: %w", c.origin, dest, date, err))
				c.log.Error("collect route failed",
					"origin", c.origin, "destination", dest, "date", date, "error", err)
				continue
			}

			if len(offers) == 0 {
				res.RoutesEmpty++
				c.log.Info("collect route empty (likely no flights)",
					"origin", c.origin, "destination", dest, "date", date)
				continue
			}

			written, err := c.store.UpsertMany(ctx, offers)
			if err != nil {
				res.RoutesFailed++
				errs = append(errs, fmt.Errorf("%s-%s %s upsert: %w", c.origin, dest, date, err))
				c.log.Error("collect route upsert failed",
					"origin", c.origin, "destination", dest, "date", date, "error", err)
				continue
			}

			res.RoutesOK++
			res.Fetched += len(offers)
			res.Upserted += written
			c.log.Info("collect route ok",
				"origin", c.origin, "destination", dest, "date", date,
				"fetched", len(offers), "written", written)
		}
	}

	return res, errors.Join(errs...)
}

// upcomingDates returns "YYYY-MM-DD" for each day from today through today+days
// (inclusive) — the departure-date window the collector queries.
func upcomingDates(now time.Time, days int) []string {
	if days < 0 {
		days = 0
	}
	y, m, d := now.Date()
	base := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	dates := make([]string, 0, days+1)
	for i := 0; i <= days; i++ {
		dates = append(dates, base.AddDate(0, 0, i).Format("2006-01-02"))
	}
	return dates
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
