-- Track whether a flight offer has been published (e.g. to the Telegram channel)
-- and when. Both columns are nullable: NULL means "not published yet".
ALTER TABLE flight_offers
    ADD COLUMN IF NOT EXISTS published    boolean,
    ADD COLUMN IF NOT EXISTS published_at timestamptz;
