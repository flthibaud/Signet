package main

import (
	"strings"
	"testing"
	"time"

	"github.com/flthibaud/signet/internal/service"
)

// configKeys is every variable loadConfig reads. Tests clear the lot so a
// developer's own environment (or the devcontainer's DATABASE_URL) can't decide
// what the defaults look like.
var configKeys = []string{
	"PORT",
	"ENV",
	"RATE_LIMITER_RPS",
	"RATE_LIMITER_BURST",
	"RATE_LIMITER_ENABLED",
	"TRUSTED_PROXY_COUNT",
	"SCHEDULER_INTERVAL",
	"SCHEDULER_WORKERS",
	"SCHEDULER_BATCH_SIZE",
	"HSTS_MAX_AGE",
	"SESSION_TTL",
	"TLS_IMPERSONATE_ENABLED",
	"SOLVER_URL",
	"SOLVER_TIMEOUT",
	"SOLVER_MAX_PER_FEED",
	"FETCH_ALLOW_PRIVATE_NETWORKS",
	"DATABASE_URL",
	"DATABASE_MAX_OPEN_CONNS",
	"DATABASE_MAX_IDLE_CONNS",
	"DATABASE_MAX_IDLE_TIME",
	"AUTO_MIGRATE",
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range configKeys {
		t.Setenv(key, "")
	}
}

// The defaults are the contract with every existing deployment: an install that
// sets only DATABASE_URL must keep behaving exactly as it did before the config
// parsing moved out of main().
func TestLoadConfigDefaults(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/signet")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"port", cfg.port, 8000},
		{"env", cfg.env, ""},
		{"limiter.rps", cfg.limiter.rps, float64(5)},
		{"limiter.burst", cfg.limiter.burst, 10},
		{"limiter.enabled", cfg.limiter.enabled, false},
		{"trustedProxyCount", cfg.trustedProxyCount, 0},
		{"scheduler.interval", cfg.scheduler.interval, service.DefaultSyncInterval},
		{"scheduler.workers", cfg.scheduler.workers, 5},
		{"scheduler.batchSize", cfg.scheduler.batchSize, 50},
		{"hstsMaxAge", cfg.hstsMaxAge, 31536000},
		{"sessionTTL", cfg.sessionTTL, 30 * 24 * time.Hour},
		{"fetch.TLSImpersonate", cfg.fetch.TLSImpersonate, true},
		{"fetch.SolverURL", cfg.fetch.SolverURL, ""},
		{"fetch.SolverTimeout", cfg.fetch.SolverTimeout, time.Duration(0)},
		{"fetch.SolverMaxPerFeed", cfg.fetch.SolverMaxPerFeed, 0},
		{"fetch.AllowPrivateNetworks", cfg.fetch.AllowPrivateNetworks, false},
		{"db.dsn", cfg.db.dsn, "postgres://localhost/signet"},
		{"db.maxOpenConns", cfg.db.maxOpenConns, 25},
		{"db.maxIdleConns", cfg.db.maxIdleConns, 25},
		{"db.maxIdleTime", cfg.db.maxIdleTime, 15 * time.Minute},
		{"db.autoMigrate", cfg.db.autoMigrate, true},
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestLoadConfigReadsEnvironment(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/signet")
	t.Setenv("PORT", "9090")
	t.Setenv("ENV", "production")
	t.Setenv("RATE_LIMITER_ENABLED", "true")
	t.Setenv("RATE_LIMITER_RPS", "2.5")
	t.Setenv("TRUSTED_PROXY_COUNT", "1")
	t.Setenv("SESSION_TTL", "168h")
	t.Setenv("HSTS_MAX_AGE", "0")
	t.Setenv("FETCH_ALLOW_PRIVATE_NETWORKS", "true")
	t.Setenv("AUTO_MIGRATE", "false")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"port", cfg.port, 9090},
		{"env", cfg.env, "production"},
		{"limiter.enabled", cfg.limiter.enabled, true},
		{"limiter.rps", cfg.limiter.rps, 2.5},
		{"trustedProxyCount", cfg.trustedProxyCount, 1},
		{"sessionTTL", cfg.sessionTTL, 168 * time.Hour},
		// 0 disables HSTS and must survive as 0, not fall back to the default.
		{"hstsMaxAge", cfg.hstsMaxAge, 0},
		{"fetch.AllowPrivateNetworks", cfg.fetch.AllowPrivateNetworks, true},
		{"db.autoMigrate", cfg.db.autoMigrate, false},
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestLoadConfigRequiresDatabaseURL(t *testing.T) {
	clearConfigEnv(t)

	_, err := loadConfig()
	if err == nil {
		t.Fatal("expected an error when DATABASE_URL is unset")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error should name DATABASE_URL, got: %v", err)
	}
}

// Reporting every bad variable in one go is the behaviour this refactor added;
// it is worth pinning down.
func TestLoadConfigReportsEveryProblemAtOnce(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("DATABASE_URL", "postgres://localhost/signet")
	t.Setenv("RATE_LIMITER_RPS", "-1")
	t.Setenv("SESSION_TTL", "30d")
	t.Setenv("HSTS_MAX_AGE", "-5")

	_, err := loadConfig()
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, key := range []string{"RATE_LIMITER_RPS", "SESSION_TTL", "HSTS_MAX_AGE"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error should mention %s, got: %v", key, err)
		}
	}
}

// These three used to be parsed with the error discarded, so a typo silently
// became the default and a negative value went straight through to the worker
// pool. They are now reported like everything else.
func TestLoadConfigRejectsUnusableSchedulerValues(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{"SCHEDULER_WORKERS", "abc"},
		{"SCHEDULER_WORKERS", "0"},
		{"SCHEDULER_WORKERS", "-3"},
		{"SCHEDULER_BATCH_SIZE", "abc"},
		{"SCHEDULER_BATCH_SIZE", "0"},
		{"SOLVER_MAX_PER_FEED", "-1"},
		{"TRUSTED_PROXY_COUNT", "-1"},
		{"TRUSTED_PROXY_COUNT", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv("DATABASE_URL", "postgres://localhost/signet")
			t.Setenv(tt.key, tt.value)

			_, err := loadConfig()
			if err == nil {
				t.Fatalf("expected an error for %s=%s", tt.key, tt.value)
			}
			if !strings.Contains(err.Error(), tt.key) {
				t.Errorf("error should name %s, got: %v", tt.key, err)
			}
		})
	}
}
