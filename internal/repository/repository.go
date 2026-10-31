package repository

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"github.com/omareloui/tyche/internal/seeder"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
)

type Repository struct {
	tm *TenantManager
}

const migrationsDir = "./sql/schema"

type TenantDB struct {
	db          *sql.DB
	queries     *queries.Queries
	lastAccess  time.Time
	migrateOnce sync.Once
	seeder      *seeder.Seeder
}

type TenantManager struct {
	baseDir     string
	connections map[string]*TenantDB
	mu          sync.RWMutex
	maxIdle     time.Duration
}

func New() *Repository {
	return &Repository{tm: newTenantManager()}
}

func newTenantManager() *TenantManager {
	tm := &TenantManager{
		baseDir:     "./data",
		connections: make(map[string]*TenantDB),
		maxIdle:     time.Minute * 30,
	}
	go tm.cleanupIdleConnections()
	return tm
}

func (tm *TenantManager) GetTenant(provider models.OAuthProvider, oauthID string) (*sql.DB, *queries.Queries, error) {
	tenantID := models.TenantID(provider, oauthID)

	tm.mu.RLock()
	if tenant, exists := tm.connections[tenantID]; exists {
		tenant.lastAccess = time.Now()
		tm.mu.RUnlock()
		return tenant.db, tenant.queries, nil
	}
	tm.mu.RUnlock()

	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Double-check after acquiring write lock
	if tenant, exists := tm.connections[tenantID]; exists {
		tenant.lastAccess = time.Now()
		return tenant.db, tenant.queries, nil
	}

	dbPath := filepath.Join(tm.baseDir, fmt.Sprintf("%s.sqlite", tenantID))
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	_, _ = db.Exec("PRAGMA foreign_keys = ON;")

	tenant := &TenantDB{
		db:         db,
		queries:    queries.New(db),
		lastAccess: time.Now(),
		seeder:     seeder.NewSeeder(),
	}

	tenant.migrateOnce.Do(func() {
		l := logger.Get()
		if err := tm.runMigrations(db); err != nil {
			l.Error("failed to run migrations", zap.String("tenantID", tenantID), zap.Error(err))
		}

		ctx := logger.WithContext(context.Background(), l)
		tenant.seeder.Seed(ctx, tenant.queries, oauthID)
		l.Info("ensured user database seeded", zap.String("oauthID", oauthID), zap.String("tenantID", tenantID))
	})

	tm.connections[tenantID] = tenant
	return tenant.db, tenant.queries, nil
}

// Reseed seeds the tenant's database again, e.g. after a reset cleared its
// seeded_at to get the default categories and accounts back.
func (tm *TenantManager) Reseed(ctx context.Context, provider models.OAuthProvider, oauthID string) error {
	_, q, err := tm.GetTenant(provider, oauthID)
	if err != nil {
		return err
	}

	tm.mu.RLock()
	tenant := tm.connections[models.TenantID(provider, oauthID)]
	tm.mu.RUnlock()
	if tenant == nil {
		return fmt.Errorf("tenant connection not found")
	}

	tenant.seeder.Seed(ctx, q, oauthID)
	return nil
}

func (tm *TenantManager) runMigrations(db *sql.DB) error {
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("failed to set dialect: %w", err)
	}

	if err := goose.Up(db, migrationsDir); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

func (tm *TenantManager) cleanupIdleConnections() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		tm.mu.Lock()
		now := time.Now()
		for tenantID, tenant := range tm.connections {
			if now.Sub(tenant.lastAccess) > tm.maxIdle {
				tenant.db.Close()
				delete(tm.connections, tenantID)
			}
		}
		tm.mu.Unlock()
	}
}

func (tm *TenantManager) Close() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	var lastErr error
	for tenantID, tenant := range tm.connections {
		if err := tenant.db.Close(); err != nil {
			lastErr = err
		}
		delete(tm.connections, tenantID)
	}
	return lastErr
}
