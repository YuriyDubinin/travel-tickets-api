// Package config loads and validates application configuration from the
// environment (optionally seeded from a .env file).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config is the root configuration object, grouped by concern.
type Config struct {
	App       App
	HTTP      HTTP
	Postgres  Postgres
	Log       Log
	Aviasales Aviasales
	Worker    Worker
	Telegram  Telegram
}

// App holds general application settings.
type App struct {
	Env string
}

// HTTP holds HTTP server settings.
type HTTP struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// Postgres holds database connection settings.
type Postgres struct {
	Host     string
	Port     string
	User     string
	Password string
	DB       string
	SSLMode  string
	MaxConns int32
}

// DSN assembles a PostgreSQL connection string of the form
// postgres://user:pass@host:port/db?sslmode=...
func (p Postgres) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		p.User, p.Password, p.Host, p.Port, p.DB, p.SSLMode,
	)
}

// Log holds logging settings.
type Log struct {
	Level string
}

// Aviasales holds Travelpayouts (Aviasales Data API) integration settings.
type Aviasales struct {
	Token       string
	Marker      string
	BaseURL     string
	Currency    string
	HTTPTimeout time.Duration
}

// Worker holds background price-collector settings. When Enabled is false (or the
// Aviasales token is empty), the worker is not started and the rest of the
// service runs normally.
type Worker struct {
	Enabled      bool
	Interval     time.Duration
	Origins      []string
	Destinations []string
	MonthsAhead  int
	OneWay       bool
	Direct       bool
	RequestDelay time.Duration

	// Publishing of collected offers to Telegram, run after each collection pass.
	PublishBatchSize int           // max offers published per cycle
	PublishDelay     time.Duration // pause between individual messages (anti-flood)
}

// Telegram holds settings for the Telegram bot that posts to a channel. When
// Enabled is false (or the bot token / channel ID is empty), the notifier is
// inert and the rest of the service runs normally.
type Telegram struct {
	Enabled               bool
	BotToken              string
	ChannelID             string
	ParseMode             string
	DisableWebPagePreview bool
	BaseURL               string
	HTTPTimeout           time.Duration
}

// Load reads configuration from environment variables, applying defaults where
// appropriate. A .env file is loaded if present (its absence is not an error).
// POSTGRES_USER and POSTGRES_PASSWORD are required and have no defaults.
func Load() (*Config, error) {
	// .env is optional: ignore the error if the file does not exist.
	_ = godotenv.Load()

	cfg := &Config{
		App: App{
			Env: getEnv("APP_ENV", "development"),
		},
		HTTP: HTTP{
			Port: getEnv("HTTP_PORT", "8080"),
		},
		Postgres: Postgres{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnv("POSTGRES_PORT", "5432"),
			User:     os.Getenv("POSTGRES_USER"),
			Password: os.Getenv("POSTGRES_PASSWORD"),
			DB:       getEnv("POSTGRES_DB", "travel_tickets_db"),
			SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
		},
		Log: Log{
			Level: getEnv("LOG_LEVEL", "info"),
		},
		Aviasales: Aviasales{
			Token:    os.Getenv("AVIASALES_TOKEN"),
			Marker:   os.Getenv("AVIASALES_MARKER"),
			BaseURL:  getEnv("AVIASALES_BASE_URL", "https://api.travelpayouts.com"),
			Currency: getEnv("AVIASALES_CURRENCY", "rub"),
		},
		Worker: Worker{
			Origins:      parseCSV("WORKER_ORIGINS", "OVB"),
			Destinations: parseCSV("WORKER_DESTINATIONS", "CXR,PQC"),
		},
		Telegram: Telegram{
			BotToken:  os.Getenv("TELEGRAM_BOT_TOKEN"),
			ChannelID: os.Getenv("TELEGRAM_CHANNEL_ID"),
			ParseMode: getEnv("TELEGRAM_PARSE_MODE", "HTML"),
			BaseURL:   getEnv("TELEGRAM_BASE_URL", "https://api.telegram.org"),
		},
	}

	var err error
	if cfg.HTTP.ReadTimeout, err = parseDuration("HTTP_READ_TIMEOUT", "15s"); err != nil {
		return nil, err
	}
	if cfg.HTTP.WriteTimeout, err = parseDuration("HTTP_WRITE_TIMEOUT", "30s"); err != nil {
		return nil, err
	}
	if cfg.HTTP.IdleTimeout, err = parseDuration("HTTP_IDLE_TIMEOUT", "60s"); err != nil {
		return nil, err
	}

	maxConns, err := parseInt("POSTGRES_MAX_CONNS", 10)
	if err != nil {
		return nil, err
	}
	cfg.Postgres.MaxConns = int32(maxConns)

	if cfg.Aviasales.HTTPTimeout, err = parseDuration("AVIASALES_HTTP_TIMEOUT", "30s"); err != nil {
		return nil, err
	}
	if cfg.Worker.Interval, err = parseDuration("WORKER_INTERVAL", "5m"); err != nil {
		return nil, err
	}
	if cfg.Worker.RequestDelay, err = parseDuration("WORKER_REQUEST_DELAY", "200ms"); err != nil {
		return nil, err
	}
	if cfg.Worker.Enabled, err = parseBool("WORKER_ENABLED", true); err != nil {
		return nil, err
	}
	if cfg.Worker.OneWay, err = parseBool("WORKER_ONE_WAY", true); err != nil {
		return nil, err
	}
	if cfg.Worker.Direct, err = parseBool("WORKER_DIRECT", true); err != nil {
		return nil, err
	}

	monthsAhead, err := parseInt("WORKER_MONTHS_AHEAD", 3)
	if err != nil {
		return nil, err
	}
	cfg.Worker.MonthsAhead = monthsAhead

	publishBatch, err := parseInt("WORKER_PUBLISH_BATCH", 10)
	if err != nil {
		return nil, err
	}
	cfg.Worker.PublishBatchSize = publishBatch

	if cfg.Worker.PublishDelay, err = parseDuration("WORKER_PUBLISH_DELAY", "3s"); err != nil {
		return nil, err
	}

	if cfg.Telegram.HTTPTimeout, err = parseDuration("TELEGRAM_HTTP_TIMEOUT", "10s"); err != nil {
		return nil, err
	}
	if cfg.Telegram.Enabled, err = parseBool("TELEGRAM_ENABLED", true); err != nil {
		return nil, err
	}
	if cfg.Telegram.DisableWebPagePreview, err = parseBool("TELEGRAM_DISABLE_WEB_PAGE_PREVIEW", true); err != nil {
		return nil, err
	}

	if cfg.Postgres.User == "" || cfg.Postgres.Password == "" {
		return nil, fmt.Errorf("config: POSTGRES_USER and POSTGRES_PASSWORD are required")
	}

	return cfg, nil
}

// getEnv returns the value of key, or fallback if it is unset or empty.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseDuration reads a time.Duration env var, falling back to a default string.
func parseDuration(key, fallback string) (time.Duration, error) {
	raw := getEnv(key, fallback)
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("config: invalid duration for %s=%q: %w", key, raw, err)
	}
	return d, nil
}

// parseInt reads an integer env var, falling back to a default when unset.
func parseInt(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("config: invalid integer for %s=%q: %w", key, raw, err)
	}
	return n, nil
}

// parseBool reads a boolean env var, falling back to a default when unset.
func parseBool(key string, fallback bool) (bool, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("config: invalid bool for %s=%q: %w", key, raw, err)
	}
	return b, nil
}

// parseCSV reads a comma-separated env var into a normalized (trimmed, uppercased,
// non-empty) slice, falling back to a default raw string when unset.
func parseCSV(key, fallback string) []string {
	raw := getEnv(key, fallback)
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.ToUpper(strings.TrimSpace(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}
