package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/travel-tickets-api/internal/domain"
)

// FlightOfferRepository persists and queries collected flight offers.
type FlightOfferRepository struct {
	pool *pgxpool.Pool
}

// NewFlightOfferRepository constructs a FlightOfferRepository.
func NewFlightOfferRepository(pool *pgxpool.Pool) *FlightOfferRepository {
	return &FlightOfferRepository{pool: pool}
}

const upsertFlightOfferSQL = `
INSERT INTO flight_offers (
	origin, destination, origin_airport, destination_airport,
	departure_at, departure_date, return_at, price, currency,
	airline, flight_number, transfers, duration, link, one_way, source, collected_at,
	published
) VALUES (
	$1, $2, $3, $4, $5, $6::date, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17,
	false
)
-- A new offer starts unpublished. On conflict we deliberately leave published /
-- published_at untouched, so an already-published offer is never re-published.
ON CONFLICT (origin, destination, departure_date, one_way) DO UPDATE SET
	origin_airport      = EXCLUDED.origin_airport,
	destination_airport = EXCLUDED.destination_airport,
	departure_at        = EXCLUDED.departure_at,
	return_at           = EXCLUDED.return_at,
	price               = EXCLUDED.price,
	currency            = EXCLUDED.currency,
	airline             = EXCLUDED.airline,
	flight_number       = EXCLUDED.flight_number,
	transfers           = EXCLUDED.transfers,
	duration            = EXCLUDED.duration,
	link                = EXCLUDED.link,
	source              = EXCLUDED.source,
	collected_at        = EXCLUDED.collected_at
`

// UpsertMany inserts or updates a batch of offers keyed by
// (origin, destination, departure_date, one_way). Returns the number of rows
// written. An empty input is a no-op.
func (r *FlightOfferRepository) UpsertMany(ctx context.Context, offers []domain.FlightOffer) (int64, error) {
	if len(offers) == 0 {
		return 0, nil
	}

	batch := &pgx.Batch{}
	for _, o := range offers {
		batch.Queue(upsertFlightOfferSQL,
			o.Origin, o.Destination, o.OriginAirport, o.DestinationAirport,
			o.DepartureAt, o.DepartureDate, o.ReturnAt, o.Price, o.Currency,
			o.Airline, o.FlightNumber, o.Transfers, o.Duration, o.Link, o.OneWay, o.Source, o.CollectedAt,
		)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()

	var affected int64
	for range offers {
		tag, err := br.Exec()
		if err != nil {
			return affected, fmt.Errorf("postgres: upsert flight offer: %w", err)
		}
		affected += tag.RowsAffected()
	}
	return affected, nil
}

const (
	defaultOfferLimit = 100
	maxOfferLimit     = 500
)

const listFlightOffersSQL = `
SELECT origin, destination,
       COALESCE(origin_airport, ''), COALESCE(destination_airport, ''),
       departure_at, to_char(departure_date, 'YYYY-MM-DD'), return_at,
       price, currency,
       COALESCE(airline, ''), COALESCE(flight_number, ''),
       transfers, COALESCE(duration, 0), COALESCE(link, ''),
       one_way, source, collected_at
FROM flight_offers
WHERE ($1 = '' OR origin = $1)
  AND ($2 = '' OR destination = $2)
ORDER BY departure_date, price
LIMIT $3
`

// List returns offers matching the filter, ordered by departure date then price.
func (r *FlightOfferRepository) List(ctx context.Context, filter domain.OfferFilter) ([]domain.FlightOffer, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultOfferLimit
	}
	if limit > maxOfferLimit {
		limit = maxOfferLimit
	}

	rows, err := r.pool.Query(ctx, listFlightOffersSQL, filter.Origin, filter.Destination, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: list flight offers: %w", err)
	}
	defer rows.Close()

	offers := make([]domain.FlightOffer, 0)
	for rows.Next() {
		var o domain.FlightOffer
		if err := rows.Scan(
			&o.Origin, &o.Destination, &o.OriginAirport, &o.DestinationAirport,
			&o.DepartureAt, &o.DepartureDate, &o.ReturnAt, &o.Price, &o.Currency,
			&o.Airline, &o.FlightNumber, &o.Transfers, &o.Duration, &o.Link,
			&o.OneWay, &o.Source, &o.CollectedAt,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan flight offer: %w", err)
		}
		offers = append(offers, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate flight offers: %w", err)
	}
	return offers, nil
}

const listUnpublishedSQL = `
SELECT id, origin, destination,
       COALESCE(origin_airport, ''), COALESCE(destination_airport, ''),
       departure_at, to_char(departure_date, 'YYYY-MM-DD'), return_at,
       price, currency,
       COALESCE(airline, ''), COALESCE(flight_number, ''),
       transfers, COALESCE(duration, 0), COALESCE(link, ''),
       one_way, source, collected_at
FROM flight_offers
WHERE published IS NOT TRUE
ORDER BY collected_at, price
LIMIT $1
`

// ListUnpublished returns offers not yet published (published is false or NULL),
// oldest first. A non-positive limit falls back to the default.
func (r *FlightOfferRepository) ListUnpublished(ctx context.Context, limit int) ([]domain.FlightOffer, error) {
	if limit <= 0 {
		limit = defaultOfferLimit
	}
	if limit > maxOfferLimit {
		limit = maxOfferLimit
	}

	rows, err := r.pool.Query(ctx, listUnpublishedSQL, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: list unpublished offers: %w", err)
	}
	defer rows.Close()

	offers := make([]domain.FlightOffer, 0)
	for rows.Next() {
		var o domain.FlightOffer
		if err := rows.Scan(
			&o.ID, &o.Origin, &o.Destination, &o.OriginAirport, &o.DestinationAirport,
			&o.DepartureAt, &o.DepartureDate, &o.ReturnAt, &o.Price, &o.Currency,
			&o.Airline, &o.FlightNumber, &o.Transfers, &o.Duration, &o.Link,
			&o.OneWay, &o.Source, &o.CollectedAt,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan unpublished offer: %w", err)
		}
		offers = append(offers, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate unpublished offers: %w", err)
	}
	return offers, nil
}

// MarkPublished marks an offer as published and stamps the current time.
func (r *FlightOfferRepository) MarkPublished(ctx context.Context, id int64) error {
	if _, err := r.pool.Exec(ctx,
		`UPDATE flight_offers SET published = true, published_at = now() WHERE id = $1`, id,
	); err != nil {
		return fmt.Errorf("postgres: mark offer %d published: %w", id, err)
	}
	return nil
}
