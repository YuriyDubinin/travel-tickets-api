// Package config loads and validates application configuration from the
// environment (optionally seeded from a .env file).
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config is the root configuration object, grouped by concern.
type Config struct {
	App      App
	HTTP     HTTP
	Postgres Postgres
	Log      Log
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
