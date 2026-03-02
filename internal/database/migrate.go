package database

import (
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/alokemajumder/AegisClaw/internal/config"
)

// RunMigrations applies all pending database migrations.
func RunMigrations(cfg config.DatabaseConfig, migrationsPath string, logger *slog.Logger) error {
	m, err := migrate.New(
		fmt.Sprintf("file://%s", migrationsPath),
		cfg.DSN(),
	)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("running migrations: %w", err)
	}

	version, dirty, _ := m.Version()
	logger.Info("database migrations applied",
		"version", version,
		"dirty", dirty,
	)

	return nil
}
