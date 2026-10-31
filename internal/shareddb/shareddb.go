// Package shareddb opens the database shared by all users (e.g. fx rates), as
// opposed to the per-user tenant databases.
package shareddb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/omareloui/tyche/internal/sharedqueries"
	"github.com/pressly/goose/v3"
)

const migrationsDir = "./sql/shared/schema"

func Open(path string) (*sql.DB, *sharedqueries.Queries, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, fmt.Errorf("failed to create the shared database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open the shared database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := goose.SetDialect("sqlite3"); err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("failed to set dialect: %w", err)
	}
	if err := goose.Up(db, migrationsDir); err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("failed to run the shared database migrations: %w", err)
	}

	return db, sharedqueries.New(db), nil
}
