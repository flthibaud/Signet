package main

import (
	"time"

	"github.com/flthibaud/signet/internal/env"
	"github.com/flthibaud/signet/internal/service"
)

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

// loadConfig reads the application's configuration from the environment.
//
// It reads the process environment only — loading a .env file is main()'s job,
// which keeps this function drivable from tests with t.Setenv. Problems are
// collected rather than fatal on sight (see internal/env), so a misconfigured
// deployment sees all of them at once.
func loadConfig() (config, error) {
	l := env.NewLoader()
	var cfg config

	cfg.port = l.Int("PORT", 8000)
	cfg.env = l.String("ENV", "")

	cfg.limiter.rps = l.Float("RATE_LIMITER_RPS", 5)
	l.Check(cfg.limiter.rps > 0, "RATE_LIMITER_RPS", "must be greater than 0")

	cfg.limiter.burst = l.Int("RATE_LIMITER_BURST", 10)
	l.Check(cfg.limiter.burst > 0, "RATE_LIMITER_BURST", "must be greater than 0")

	// Off unless asked for: whether to rate limit is a deployment decision. It
	// only ever applies to the JSON API (see rateLimitPrefix), never to the
	// embedded SPA's static assets.
	cfg.limiter.enabled = l.Bool("RATE_LIMITER_ENABLED", false)

	cfg.scheduler.interval = l.Duration("SCHEDULER_INTERVAL", service.DefaultSyncInterval)
	l.Check(cfg.scheduler.interval > 0, "SCHEDULER_INTERVAL", "must be positive")

	cfg.scheduler.workers = l.Int("SCHEDULER_WORKERS", 5)
	l.Check(cfg.scheduler.workers > 0, "SCHEDULER_WORKERS", "must be greater than 0")

	cfg.scheduler.batchSize = l.Int("SCHEDULER_BATCH_SIZE", 50)
	l.Check(cfg.scheduler.batchSize > 0, "SCHEDULER_BATCH_SIZE", "must be greater than 0")

	// A year, the usual HSTS lifetime. Only ever sent over HTTPS (see
	// secureHeaders), so a plain-HTTP install ignores it; 0 disables it outright.
	cfg.hstsMaxAge = l.Int("HSTS_MAX_AGE", 31536000)
	l.Check(cfg.hstsMaxAge >= 0, "HSTS_MAX_AGE", "must not be negative")

	// How long a session survives without being used. The expiry slides forward
	// while the session is active (see refreshSession), so this is an idle
	// timeout: thirty days suits an app people open a few times a week, and a
	// shared machine wants it shorter.
	cfg.sessionTTL = l.Duration("SESSION_TTL", 30*24*time.Hour)
	l.Check(cfg.sessionTTL > 0, "SESSION_TTL", "must be positive")

	cfg.fetch.TLSImpersonate = l.Bool("TLS_IMPERSONATE_ENABLED", true)
	cfg.fetch.SolverURL = l.String("SOLVER_URL", "")

	// Zero leaves the service layer's own default in place, for both of these.
	cfg.fetch.SolverTimeout = l.Duration("SOLVER_TIMEOUT", 0)
	l.Check(cfg.fetch.SolverTimeout >= 0, "SOLVER_TIMEOUT", "must not be negative")

	cfg.fetch.SolverMaxPerFeed = l.Int("SOLVER_MAX_PER_FEED", 0)
	l.Check(cfg.fetch.SolverMaxPerFeed >= 0, "SOLVER_MAX_PER_FEED", "must not be negative")

	// Off by default: feed and article URLs come from users, so outbound fetches
	// are only allowed to reach public addresses. Turn it on to subscribe to a
	// feed on the LAN or in a neighbouring container. Cloud metadata
	// (169.254.169.254) stays blocked either way.
	cfg.fetch.AllowPrivateNetworks = l.Bool("FETCH_ALLOW_PRIVATE_NETWORKS", false)

	cfg.db.dsn = l.Required("DATABASE_URL")

	// The pool sizes are handed to database/sql as-is, where 0 carries its own
	// meaning (unlimited open connections, no retained idle ones), so they are
	// deliberately left unconstrained.
	cfg.db.maxOpenConns = l.Int("DATABASE_MAX_OPEN_CONNS", 25)
	cfg.db.maxIdleConns = l.Int("DATABASE_MAX_IDLE_CONNS", 25)
	cfg.db.maxIdleTime = l.Duration("DATABASE_MAX_IDLE_TIME", 15*time.Minute)

	cfg.db.autoMigrate = l.Bool("AUTO_MIGRATE", true)

	return cfg, l.Err()
}
