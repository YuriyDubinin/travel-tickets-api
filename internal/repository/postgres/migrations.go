package postgres

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	// Register the database driver (postgres://) and source driver (file://).
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// RunMigrations applies all pending "up" migrations found in dir against the
// database at dsn. A lack of pending migrations (migrate.ErrNoChange) is treated
// as success. On success the target schema version is logged.
func RunMigrations(dsn, dir string, log *slog.Logger) error {
	m, err := migrate.New("file://"+dir, dsn)
	if err != nil {
		return fmt.Errorf("migrations: init: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			log.Error("migrations: source close error", "error", srcErr)
		}
		if dbErr != nil {
			log.Error("migrations: database close error", "error", dbErr)
		}
	}()

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			log.Info("no migrations to apply")
			return nil
		}
		return fmt.Errorf("migrations: up: %w", err)
	}

	version, dirty, err := m.Version()
	if err != nil {
		return fmt.Errorf("migrations: read version: %w", err)
	}
	log.Info("migrations applied", "version", version, "dirty", dirty)

	return nil
}
