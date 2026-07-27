package main

import (
	"os"
	"strings"
	"testing"

	"github.com/flthibaud/signet"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func TestEmbeddedMigrationsMatchDirectory(t *testing.T) {
	src, err := iofs.New(signet.Migrations, "migrations")
	if err != nil {
		t.Fatalf("iofs.New over the embedded migrations: %v", err)
	}
	defer src.Close()

	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatalf("reading the migrations directory: %v", err)
	}

	var onDisk []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			onDisk = append(onDisk, e.Name())
		}
	}
	if len(onDisk) == 0 {
		t.Fatal("no .up.sql files on disk, the test is not checking anything")
	}

	// Walk the source driver the way migrate does, counting the versions it can
	// actually see.
	seen := 0
	version, err := src.First()
	for err == nil {
		seen++
		version, err = src.Next(version)
	}
	if !os.IsNotExist(err) {
		t.Fatalf("walking the embedded migrations: %v", err)
	}

	if seen != len(onDisk) {
		t.Errorf("embedded migrations = %d, on disk = %d; the go:embed glob in migrations.go is out of step with the directory", seen, len(onDisk))
	}
}

// Every up needs its down: `make migrate-down` and the down leg of reset-db
// both assume the pair exists.
func TestEveryMigrationHasBothDirections(t *testing.T) {
	src, err := iofs.New(signet.Migrations, "migrations")
	if err != nil {
		t.Fatalf("iofs.New over the embedded migrations: %v", err)
	}
	defer src.Close()

	version, err := src.First()
	for err == nil {
		if _, _, upErr := src.ReadUp(version); upErr != nil {
			t.Errorf("migration %d has no readable up: %v", version, upErr)
		}
		if _, _, downErr := src.ReadDown(version); downErr != nil {
			t.Errorf("migration %d has no readable down: %v", version, downErr)
		}
		version, err = src.Next(version)
	}
	if !os.IsNotExist(err) {
		t.Fatalf("walking the embedded migrations: %v", err)
	}
}
