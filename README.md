# travel-tickets-api

A clean-architecture HTTP API in Go (chi + pgx/PostgreSQL): configuration,
logging, a PostgreSQL pool, migrations, HTTP middleware, a `GET /api/ping`
health-check, and a background worker that periodically collects flight prices
from the Travelpayouts (Aviasales) Data API into PostgreSQL.

Note on routes: there are currently no flights Novosibirsk (OVB) → Simferopol
(SIP) — the only Crimean airport is closed. The worker therefore monitors the
real "feeder" airports (Sochi/AER, Krasnodar/KRR, Anapa/AAQ, Mineralnye Vody/MRV);
the final leg to Crimea is by ground transport.

## Configuration

All settings live in `.env` (ignored by git and Docker, so credentials never
leak into the repo or the image — copy `.env` to the server manually). The
PostgreSQL container and the API run on the same host, and the API reaches the DB
through the host's published port `5432`, so `POSTGRES_HOST` is:

- `host.docker.internal` — **default**. Requires
  `--add-host=host.docker.internal:host-gateway` in `docker run`
  (works on Linux and Docker Desktop).
- `172.17.0.1` — docker0 gateway; works on a default Linux bridge with **no extra
  run flag** (use this if you keep the run command exactly as-is).
- `localhost` — only when running the API directly via `go run`.

## Background price worker

When `WORKER_ENABLED=true` and `AVIASALES_TOKEN` is set, a worker runs every
`WORKER_INTERVAL` (default `5m`): it queries `aviasales/v3/prices_for_dates` for
`WORKER_ORIGIN` → each of `WORKER_DESTINATIONS`, across the next
`WORKER_MONTHS_AHEAD` months, and upserts the results into the `flight_offers`
table. Empty routes (e.g. `OVB→SIP`) are logged and skipped, not treated as
errors. Without a token the worker stays disabled and the rest of the service
runs normally.

## Data model: `flight_offers`

The worker upserts collected offers into the `flight_offers` table. One row =
the cheapest known offer for a route on a given departure date.

| Column                | Meaning                                                               |
|-----------------------|-----------------------------------------------------------------------|
| `id`                  | Auto-increment primary key (internal row id).                         |
| `origin`              | Departure city / IATA code (e.g. `OVB`).                              |
| `destination`         | Destination city / IATA code (e.g. `AER`).                            |
| `origin_airport`      | Specific departure airport (IATA).                                    |
| `destination_airport` | Specific arrival airport (IATA).                                      |
| `departure_at`        | Full departure date & time (with timezone).                           |
| `departure_date`      | Departure date only — part of the dedup key.                          |
| `return_at`           | Return date & time; empty for one-way tickets.                        |
| `price`               | Ticket price (integer).                                               |
| `currency`            | Price currency (e.g. `rub`).                                          |
| `airline`             | Airline IATA code (e.g. `S7`).                                        |
| `flight_number`       | Flight number.                                                        |
| `transfers`           | Number of stops (`0` = direct).                                       |
| `duration`            | Trip duration, in minutes.                                            |
| `link`                | Full clickable Aviasales URL (absolute, e.g. `https://www.aviasales.ru/…`). |
| `one_way`             | `true` = one-way, `false` = round-trip.                               |
| `source`              | Data source (`travelpayouts`).                                        |
| `collected_at`        | When this row was last written/updated (last successful parse).       |

Deduplication: the unique key **`(origin, destination, departure_date, one_way)`**
identifies "the same ticket". On each parse an existing row is updated in place
(price, airline, flight_number, link, collected_at refreshed) rather than
duplicated.

## Telegram bot

The service can post messages to a Telegram channel. Configure it in `.env`:
`TELEGRAM_BOT_TOKEN` (from @BotFather), `TELEGRAM_CHANNEL_ID` (`@channel` or a
numeric `-100...` id), and optional `TELEGRAM_PARSE_MODE` (default `HTML`). The
notifier is disabled — and the rest of the service runs normally — when
`TELEGRAM_ENABLED=false` or the token / channel is empty. At startup the bot is
validated with `getMe` (nothing is posted).

Post a message to the channel via the scaffold endpoint:

```sh
curl -X POST http://localhost:18080/api/notify \
  -H 'Content-Type: application/json' \
  -d '{"text":"<b>Hello</b> from travel-tickets-api"}'
# {"status":"sent"}
```

> `/api/notify` has no authentication yet — add auth before exposing it publicly,
> since anyone who can reach it can post to the channel.

## Deploy: build → push → pull → run

Image: `yuriydubinin100/travel-tickets-api:1.0.0`

**1. Build on your Mac (cross-compiled to linux/amd64) and push to Docker Hub:**

```sh
docker login
docker build --platform linux/amd64 -t yuriydubinin100/travel-tickets-api:1.0.0 .
docker push yuriydubinin100/travel-tickets-api:1.0.0
```

**2. On the server — pull and run.** Make sure the `postgres` container is up
(`docker compose up -d`) and `.env` is present in the current directory:

```sh
docker pull yuriydubinin100/travel-tickets-api:1.0.0

docker run -d \
  --name travel-tickets-api \
  --env-file .env \
  --add-host=host.docker.internal:host-gateway \
  -p 18080:8080 \
  --user root \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /run/systemd:/run/systemd:ro \
  -v /run/dbus/system_bus_socket:/run/dbus/system_bus_socket:ro \
  -v travel-tickets-ssh:/data/ssh \
  -v /usr/libexec/docker/cli-plugins/docker-compose:/usr/libexec/docker/cli-plugins/docker-compose:ro \
  yuriydubinin100/travel-tickets-api:1.0.0
```

> `--add-host=host.docker.internal:host-gateway` is the one line added to your
> command — without it the container cannot reach PostgreSQL and exits at startup.
> Alternative: drop it and set `POSTGRES_HOST=172.17.0.1` in `.env`.
>
> The `docker.sock` / `systemd` / `dbus` / `ssh` / `compose-plugin` mounts and
> `--user root` are NOT used by this API — they are kept from the shared deploy
> template. The minimal run needs only `--env-file`, `--add-host`, and `-p`.

**3. Verify:**

```sh
curl http://localhost:18080/api/ping
# {"status":"ok","time":"..."}
docker logs --tail 20 travel-tickets-api
```

## Endpoints

Base URL (local run): `http://localhost:18080`

| Method | Path          | Description                                                                          |
|--------|---------------|--------------------------------------------------------------------------------------|
| GET    | `/api/ping`   | Health-check of the service. Returns 200 OK if the process is alive.                  |
| GET    | `/api/offers` | Collected flight offers. Optional query: `origin`, `destination`, `limit`. Example: `/api/offers?origin=OVB&destination=AER`. |
| POST   | `/api/notify` | Post a message to the Telegram channel. Body: `{"text":"..."}`. Returns 503 if the bot is not configured. |

## Run locally (go run)

```sh
# set POSTGRES_HOST=localhost in .env, then:
go run ./cmd/api
curl http://localhost:8080/api/ping
```
