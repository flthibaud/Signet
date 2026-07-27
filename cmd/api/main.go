package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/jsonlog"
	"github.com/flthibaud/signet/internal/service"
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
		maxIdleTime  time.Duration
		autoMigrate  bool
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
	hstsMaxAge int
	sessionTTL time.Duration
	fetch      service.FetchConfig
}

type application struct {
	config    config
	logger    *jsonlog.Logger
	db        *sql.DB
	models    data.Models
	services  service.Services
	scheduler *service.Scheduler
}

func main() {
	var cfg config
	var err error

	var migrateOnly bool
	flag.BoolVar(&migrateOnly, "migrate-only", false, "apply pending database migrations, then exit")
	flag.Parse()

	// Load .env file if present
	_ = godotenv.Load()

	cfg.env = os.Getenv("ENV")

	cfg.port = 8000
	if v := os.Getenv("PORT"); v != "" {
		cfg.port, err = strconv.Atoi(v)
		if err != nil {
			log.Fatalf("invalid PORT: %v", err)
		}
	}

	cfg.limiter.rps = 5
	if v := os.Getenv("RATE_LIMITER_RPS"); v != "" {
		cfg.limiter.rps, err = strconv.ParseFloat(v, 64)
		if err != nil {
			log.Fatalf("invalid RATE_LIMITER_RPS: %v", err)
		}
		if cfg.limiter.rps <= 0 {
			log.Fatalf("invalid RATE_LIMITER_RPS: must be greater than 0, got %v", cfg.limiter.rps)
		}
	}

	cfg.limiter.burst = 10
	if v := os.Getenv("RATE_LIMITER_BURST"); v != "" {
		cfg.limiter.burst, err = strconv.Atoi(v)
		if err != nil {
			log.Fatalf("invalid RATE_LIMITER_BURST: %v", err)
		}
		if cfg.limiter.burst <= 0 {
			log.Fatalf("invalid RATE_LIMITER_BURST: must be greater than 0, got %d", cfg.limiter.burst)
		}
	}

	// Off unless asked for: whether to rate limit is a deployment decision. It
	// only ever applies to the JSON API (see rateLimitPrefix), never to the
	// embedded SPA's static assets.
	if v := os.Getenv("RATE_LIMITER_ENABLED"); v != "" {
		cfg.limiter.enabled, err = strconv.ParseBool(v)
		if err != nil {
			log.Fatalf("invalid RATE_LIMITER_ENABLED: %v", err)
		}
	}

	cfg.scheduler.interval = service.DefaultSyncInterval
	if v := os.Getenv("SCHEDULER_INTERVAL"); v != "" {
		cfg.scheduler.interval, err = time.ParseDuration(v)
		if err != nil {
			log.Fatalf("invalid SCHEDULER_INTERVAL: %v", err)
		}
		if cfg.scheduler.interval <= 0 {
			log.Fatalf("invalid SCHEDULER_INTERVAL: must be positive, got %s", v)
		}
	}
	cfg.scheduler.workers, _ = strconv.Atoi(os.Getenv("SCHEDULER_WORKERS"))
	if cfg.scheduler.workers == 0 {
		cfg.scheduler.workers = 5
	}
	cfg.scheduler.batchSize, _ = strconv.Atoi(os.Getenv("SCHEDULER_BATCH_SIZE"))
	if cfg.scheduler.batchSize == 0 {
		cfg.scheduler.batchSize = 50
	}

	// A year, the usual HSTS lifetime. Only ever sent over HTTPS (see
	// secureHeaders), so a plain-HTTP install ignores it; 0 disables it outright.
	cfg.hstsMaxAge = 31536000
	if v := os.Getenv("HSTS_MAX_AGE"); v != "" {
		cfg.hstsMaxAge, err = strconv.Atoi(v)
		if err != nil {
			log.Fatalf("invalid HSTS_MAX_AGE: %v", err)
		}
		if cfg.hstsMaxAge < 0 {
			log.Fatalf("invalid HSTS_MAX_AGE: must not be negative, got %d", cfg.hstsMaxAge)
		}
	}

	// How long a session survives without being used. The expiry slides forward
	// while the session is active (see refreshSession), so this is an idle
	// timeout: thirty days suits an app people open a few times a week, and a
	// shared machine wants it shorter.
	cfg.sessionTTL = 30 * 24 * time.Hour
	if v := os.Getenv("SESSION_TTL"); v != "" {
		cfg.sessionTTL, err = time.ParseDuration(v)
		if err != nil {
			log.Fatalf("invalid SESSION_TTL: %v", err)
		}
		if cfg.sessionTTL <= 0 {
			log.Fatalf("invalid SESSION_TTL: must be positive, got %s", v)
		}
	}

	cfg.fetch.TLSImpersonate = true
	if v := os.Getenv("TLS_IMPERSONATE_ENABLED"); v != "" {
		cfg.fetch.TLSImpersonate, err = strconv.ParseBool(v)
		if err != nil {
			log.Fatalf("invalid TLS_IMPERSONATE_ENABLED: %v", err)
		}
	}
	cfg.fetch.SolverURL = os.Getenv("SOLVER_URL")
	if v := os.Getenv("SOLVER_TIMEOUT"); v != "" {
		cfg.fetch.SolverTimeout, err = time.ParseDuration(v)
		if err != nil {
			log.Fatalf("invalid SOLVER_TIMEOUT: %v", err)
		}
	}
	cfg.fetch.SolverMaxPerFeed, _ = strconv.Atoi(os.Getenv("SOLVER_MAX_PER_FEED"))

	// Off by default: feed and article URLs come from users, so outbound fetches
	// are only allowed to reach public addresses. Turn it on to subscribe to a
	// feed on the LAN or in a neighbouring container. Cloud metadata
	// (169.254.169.254) stays blocked either way.
	if v := os.Getenv("FETCH_ALLOW_PRIVATE_NETWORKS"); v != "" {
		cfg.fetch.AllowPrivateNetworks, err = strconv.ParseBool(v)
		if err != nil {
			log.Fatalf("invalid FETCH_ALLOW_PRIVATE_NETWORKS: %v", err)
		}
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	cfg.db.dsn = databaseURL

	cfg.db.maxOpenConns = 25
	if v := os.Getenv("DATABASE_MAX_OPEN_CONNS"); v != "" {
		cfg.db.maxOpenConns, err = strconv.Atoi(v)
		if err != nil {
			log.Fatalf("invalid DATABASE_MAX_OPEN_CONNS: %v", err)
		}
	}

	cfg.db.maxIdleConns = 25
	if v := os.Getenv("DATABASE_MAX_IDLE_CONNS"); v != "" {
		cfg.db.maxIdleConns, err = strconv.Atoi(v)
		if err != nil {
			log.Fatalf("invalid DATABASE_MAX_IDLE_CONNS: %v", err)
		}
	}

	cfg.db.maxIdleTime = 15 * time.Minute
	if v := os.Getenv("DATABASE_MAX_IDLE_TIME"); v != "" {
		cfg.db.maxIdleTime, err = time.ParseDuration(v)
		if err != nil {
			log.Fatalf("invalid DATABASE_MAX_IDLE_TIME: %v", err)
		}
	}

	cfg.db.autoMigrate = true
	if v := os.Getenv("AUTO_MIGRATE"); v != "" {
		cfg.db.autoMigrate, err = strconv.ParseBool(v)
		if err != nil {
			log.Fatalf("invalid AUTO_MIGRATE: %v", err)
		}
	}

	logger := jsonlog.New(os.Stdout, jsonlog.LevelInfo)

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
