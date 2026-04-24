package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/flthibaud/origami/internal/data"
	"github.com/flthibaud/origami/internal/jsonlog"
	"github.com/flthibaud/origami/internal/service"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

const version = "1.0.0"

type config struct {
	port int
	env  string
	db   struct {
		dsn          string
		maxOpenConns int
		maxIdleConns int
		maxIdleTime  string
	}
	limiter struct {
		rps     float64
		burst   int
		enabled bool
	}
	scheduler struct {
		interval  time.Duration
		workers   int
		batchSize int
	}
}

type application struct {
	config    config
	logger    *jsonlog.Logger
	models    data.Models
	services  service.Services
	scheduler *service.Scheduler
}

func main() {
	var cfg config

	// Load .env file if present
	_ = godotenv.Load()

	cfg.env = os.Getenv("ENV")
	cfg.port, _ = strconv.Atoi(os.Getenv("PORT"))

	cfg.limiter.rps, _ = strconv.ParseFloat(os.Getenv("RATE_LIMITER_RPS"), 64)
	cfg.limiter.burst, _ = strconv.Atoi(os.Getenv("RATE_LIMITER_BURST"))
	cfg.limiter.enabled, _ = strconv.ParseBool(os.Getenv("RATE_LIMITER_ENABLED"))

	schedulerInterval := os.Getenv("SCHEDULER_INTERVAL")
	if schedulerInterval == "" {
		schedulerInterval = "15m"
	}
	var err error
	cfg.scheduler.interval, err = time.ParseDuration(schedulerInterval)
	if err != nil {
		log.Fatalf("invalid SCHEDULER_INTERVAL: %v", err)
	}
	cfg.scheduler.workers, _ = strconv.Atoi(os.Getenv("SCHEDULER_WORKERS"))
	if cfg.scheduler.workers == 0 {
		cfg.scheduler.workers = 5
	}
	cfg.scheduler.batchSize, _ = strconv.Atoi(os.Getenv("SCHEDULER_BATCH_SIZE"))
	if cfg.scheduler.batchSize == 0 {
		cfg.scheduler.batchSize = 50
	}

	databaseURL := os.Getenv("DATABASE_URI")
	if databaseURL == "" {
		log.Fatal("DATABASE_URI is not set")
	}

	cfg.db.dsn = databaseURL

	logger := jsonlog.New(os.Stdout, jsonlog.LevelInfo)

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
	services := service.NewServices(models)

	// 4. Init du scheduler
	scheduler := service.NewScheduler(&services, logger, cfg.scheduler.interval, cfg.scheduler.workers, cfg.scheduler.batchSize)

	app := &application{
		config:    cfg,
		logger:    logger,
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
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		return nil, err
	}
	return db, nil
}
