package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/flthibaud/signet"
	"github.com/flthibaud/signet/internal/jsonlog"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"
)

func migrateUp(dsn string, logger *jsonlog.Logger) error {
	src, err := iofs.New(signet.Migrations, "migrations")
	if err != nil {
		return fmt.Errorf("reading embedded migrations: %w", err)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("opening migration connection: %w", err)
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		db.Close()
		return fmt.Errorf("connecting for migrations: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		db.Close()
		return fmt.Errorf("initialising migrations: %w", err)
	}

	defer m.Close()

	err = m.Up()
	switch {
	case errors.Is(err, migrate.ErrNoChange):
		logger.PrintInfo("database schema is up to date", nil)
		return nil

	case err != nil:
		var dirty migrate.ErrDirty
		if errors.As(err, &dirty) {
			return fmt.Errorf("database is marked dirty at version %d: an earlier migration failed part-way "+
				"and the schema may be incomplete. Inspect it, finish or undo migration %d by hand, then clear "+
				"the flag with `make migrate-force version=%d` before starting again", dirty.Version, dirty.Version, dirty.Version)
		}
		return fmt.Errorf("applying migrations: %w", err)
	}

	version, _, err := m.Version()
	if err != nil {
		logger.PrintInfo("migrations applied", nil)
		return nil
	}

	logger.PrintInfo("migrations applied", map[string]string{
		"version": strconv.FormatUint(uint64(version), 10),
	})
	return nil
}
