ALTER TABLE flight_offers
    DROP COLUMN IF EXISTS published,
    DROP COLUMN IF EXISTS published_at;
