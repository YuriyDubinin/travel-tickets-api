-- Cached flight offers collected from the Aviasales Data API.
CREATE TABLE IF NOT EXISTS flight_offers (
    id                  bigserial PRIMARY KEY,
    origin              text        NOT NULL,
    destination         text        NOT NULL,
    origin_airport      text,
    destination_airport text,
    departure_at        timestamptz NOT NULL,
    departure_date      date        NOT NULL,
    return_at           timestamptz,
    price               bigint      NOT NULL,
    currency            text        NOT NULL,
    airline             text,
    flight_number       text,
    transfers           integer     NOT NULL DEFAULT 0,
    duration            integer,
    link                text,
    one_way             boolean     NOT NULL DEFAULT true,
    source              text        NOT NULL DEFAULT 'travelpayouts',
    collected_at        timestamptz NOT NULL DEFAULT now()
);

-- One current row per route + departure date + trip type (the upsert target).
CREATE UNIQUE INDEX IF NOT EXISTS uq_flight_offers_route_date
    ON flight_offers (origin, destination, departure_date, one_way);

-- Lookup helpers for the read API.
CREATE INDEX IF NOT EXISTS idx_flight_offers_route
    ON flight_offers (origin, destination);
CREATE INDEX IF NOT EXISTS idx_flight_offers_departure_date
    ON flight_offers (departure_date);
