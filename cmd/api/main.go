package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/flthibaud/omnivore-go/internal/data"
	"github.com/flthibaud/omnivore-go/internal/jsonlog"
	"github.com/flthibaud/omnivore-go/internal/service"
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
}

type application struct {
	config   config
	logger   *jsonlog.Logger
	models   data.Models
	services service.Services
}

func main() {
	var cfg config

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	cfg.env = os.Getenv("ENV")
	cfg.port, _ = strconv.Atoi(os.Getenv("PORT"))

	cfg.limiter.rps, _ = strconv.ParseFloat(os.Getenv("RATE_LIMITER_RPS"), 64)
	cfg.limiter.burst, _ = strconv.Atoi(os.Getenv("RATE_LIMITER_BURST"))
	cfg.limiter.enabled, _ = strconv.ParseBool(os.Getenv("RATE_LIMITER_ENABLED"))

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

	// Declare an instance of the application struct, containing the config struct and
	// the logger.
	app := &application{
		config:   cfg,
		logger:   logger,
		models:   models,
		services: services,
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
