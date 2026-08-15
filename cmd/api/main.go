// Command api is Signet's single binary: it serves the JSON API under /v1 and
// the embedded React Router SPA on everything else, from one process and one
// port.
//
// Startup order matters. Pending migrations are applied first, on a throwaway
// connection (migrate.go), so the pool the rest of the wiring opens is against
// the schema that wiring assumes. Configuration is read from the environment
// through internal/env, which reports every bad value at once rather than one
// per restart. Passing --migrate-only migrates and exits, for a PaaS release
// command or a job run ahead of a multi-replica rollout.
//
// routes.go is the single source of truth for what this binary exposes,
// middleware.go for the chain every request passes through.
package main

import (
	"context"
	"database/sql"
	"flag"
	"os"
	"time"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/jsonlog"
	"github.com/flthibaud/signet/internal/service"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

const version = "1.0.0"

type application struct {
	config    config
	logger    *jsonlog.Logger
	db        *sql.DB
	models    data.Models
	services  service.Services
	scheduler *service.Scheduler
}

func main() {
	var migrateOnly bool
	flag.BoolVar(&migrateOnly, "migrate-only", false, "apply pending database migrations, then exit")
	flag.Parse()

	// Load .env file if present
	_ = godotenv.Load()

	// Built before the config is read, so a configuration error is reported
	// through the same structured logger as everything else.
	logger := jsonlog.New(os.Stdout, jsonlog.LevelInfo)

	cfg, err := loadConfig()
	if err != nil {
		logger.PrintFatal(err, nil)
	}

	if migrateOnly || cfg.db.autoMigrate {
		err = migrateUp(cfg.db.dsn, logger)
		if err != nil {
			logger.PrintFatal(err, nil)
		}
	}
	if migrateOnly {
		return
	}

	db, err := openDB(cfg)
	if err != nil {
		logger.PrintFatal(err, nil)
	}
	// Defer a call to db.Close() so that the connection pool is closed before the
	// main() function exits.
	defer db.Close()
	// Also log a message to say that the connection pool has been successfully
	// established.
	logger.PrintInfo("database connection pool established", nil)

	// 2. Init de la couche DATA
	models := data.NewModels(db)

	// 3. Init de la couche SERVICE (avec injection de data)
	services := service.NewServices(models, logger, cfg.fetch)

	// 4. Init du scheduler
	scheduler := service.NewScheduler(&services, logger, cfg.scheduler.interval, cfg.scheduler.workers, cfg.scheduler.batchSize)

	services.OPMLService.SetSyncTrigger(scheduler)

	app := &application{
		config:    cfg,
		logger:    logger,
		db:        db,
		models:    models,
		services:  services,
		scheduler: scheduler,
	}

	// Start the HTTP server.
	err = app.serve()
	if err != nil {
		logger.PrintFatal(err, nil)
	}
}

// The openDB() function returns a sql.DB connection pool.
func openDB(cfg config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.db.dsn)
	if err != nil {
		return nil, err
	}

	// Configurer le pool de connexion
	db.SetMaxOpenConns(cfg.db.maxOpenConns)
	db.SetMaxIdleConns(cfg.db.maxIdleConns)
	db.SetConnMaxIdleTime(cfg.db.maxIdleTime)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		return nil, err
	}
	return db, nil
}
