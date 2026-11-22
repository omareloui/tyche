package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"slices"

	"github.com/pressly/goose/v3"
)

// tenantGoMigrations are the tenant DB's migrations written in Go (one-off
// backfills), run with the SQL ones of migrationsDir. A file registers its
// own in an init, so removing it is deleting the file: goose leaves alone the
// applied versions it doesn't know anymore.
var tenantGoMigrations []*goose.Migration

// newMigrationProvider is goose for a tenant DB: the SQL migrations of dir
// plus tenantGoMigrations. Its own registry, so the shared DB (migrated with
// goose's global one) never runs them.
func newMigrationProvider(db *sql.DB, dir string) (*goose.Provider, error) {
	return goose.NewProvider(goose.DialectSQLite3, db, os.DirFS(dir),
		goose.WithGoMigrations(tenantGoMigrations...),
		goose.WithDisableGlobalRegistry(true),
	)
}

// migrateTenantDB applies every pending tenant migration of dir.
func migrateTenantDB(ctx context.Context, db *sql.DB, dir string) error {
	p, err := newMigrationProvider(db, dir)
	if err != nil {
		return fmt.Errorf("failed to set up migrations: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}

// latestMigration is the version the tenant migrations bring a DB to.
func latestMigration() (int64, error) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return 0, err
	}
	defer db.Close()
	p, err := newMigrationProvider(db, migrationsDir)
	if err != nil {
		return 0, err
	}
	srcs := p.ListSources()
	if len(srcs) == 0 {
		return 0, goose.ErrNoMigrations
	}
	return slices.MaxFunc(srcs, func(a, b *goose.Source) int { return int(a.Version - b.Version) }).Version, nil
}
