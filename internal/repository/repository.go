package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"github.com/omareloui/tyche/internal/seeder"
	"github.com/omareloui/tyche/internal/sharedqueries"
	"github.com/omareloui/tyche/internal/vault"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
)

type Repository struct {
	tm     *TenantManager
	vaults *vault.Store
	shared *sharedqueries.Queries

	deletedMu sync.RWMutex
	// deleted are the tenants whose account an admin deleted (shared DB's
	// deleted_tenants), whose database mustn't be created again from their
	// old cookies or device tokens.
	deleted map[string]bool

	upkeepMu sync.Mutex
	// keptUp is the day (in the user's time zone) each tenant's daily upkeep
	// last ran on (RunDailyUpkeep).
	keptUp map[string]string
}

// ErrTenantLocked is returned when the tenant's database is encrypted and the
// context has no key for it (middleware.VaultKeyCtxKey), the user has to
// unlock it first.
var ErrTenantLocked = errors.New("the tenant's database is locked")

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

func New(vaults *vault.Store, shared *sharedqueries.Queries) *Repository {
	return &Repository{tm: newTenantManager(), vaults: vaults, shared: shared, deleted: make(map[string]bool), keptUp: make(map[string]string)}
}

// TenantExists reports whether the tenant's database was already created.
func (r *Repository) TenantExists(tenantID string) bool {
	return r.tm.Exists(tenantID)
}

// Vaults holds the keys of the encrypted tenant databases.
func (r *Repository) Vaults() *vault.Store {
	return r.vaults
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

// GetTenant gets the tenant's database, opening it when it's not already.
// key is its encryption key, nil when it's not encrypted.
func (tm *TenantManager) GetTenant(provider models.OAuthProvider, oauthID string, key []byte) (*sql.DB, *queries.Queries, error) {
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

	db, err := tm.open(tm.dbPath(tenantID), key)
	if err != nil {
		return nil, nil, err
	}

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
		tenant.seeder.Seed(ctx, db, oauthID)
		l.Info("ensured user database seeded", zap.String("oauthID", oauthID), zap.String("tenantID", tenantID))
	})

	tm.connections[tenantID] = tenant
	return tenant.db, tenant.queries, nil
}

// Exists reports whether the tenant's database was already created, false on
// their first sign in.
func (tm *TenantManager) Exists(tenantID string) bool {
	tm.mu.RLock()
	_, open := tm.connections[tenantID]
	tm.mu.RUnlock()
	if open {
		return true
	}
	_, err := os.Stat(tm.dbPath(tenantID))
	return err == nil
}

func (tm *TenantManager) dbPath(tenantID string) string {
	return filepath.Join(tm.baseDir, fmt.Sprintf("%s.sqlite", tenantID))
}

// open opens the database, with its key when it's encrypted. An encrypted
// tenant's database still in plain text (the server stopped while encrypting
// it, after its vault was created) is encrypted now.
func (tm *TenantManager) open(path string, key []byte) (*sql.DB, error) {
	db, err := openDB(path, key)
	if err == nil || key == nil || !isNotADatabase(err) {
		return db, err
	}

	plain, plainErr := openDB(path, nil)
	if plainErr != nil {
		return nil, err
	}
	plain.Close()

	logger.Get().Warn("encrypting a database left in plain text", zap.String("path", path))
	if err := exportDB(path, nil, key); err != nil {
		return nil, fmt.Errorf("failed to encrypt the database: %w", err)
	}
	return openDB(path, key)
}

// openDB opens the database and checks it can be read (with the right key).
func openDB(path string, key []byte) (*sql.DB, error) {
	dsn := path
	if key != nil {
		dsn += "?_key=" + vault.SQLCipherKey(key)
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	_, _ = db.Exec("PRAGMA foreign_keys = ON;")

	if _, err := db.Exec("SELECT count(*) FROM sqlite_master"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to read the database: %w", err)
	}
	return db, nil
}

func isNotADatabase(err error) bool {
	return strings.Contains(err.Error(), "file is not a database")
}

// Encrypt encrypts the tenant's database with the key, in place.
func (tm *TenantManager) Encrypt(provider models.OAuthProvider, oauthID string, key []byte) error {
	return tm.export(models.TenantID(provider, oauthID), nil, key, nil)
}

// Decrypt writes the tenant's encrypted database back in plain text, then
// runs done (deleting its vault) before the database can be opened again, as
// it would be encrypted again if opened with the key meanwhile.
func (tm *TenantManager) Decrypt(provider models.OAuthProvider, oauthID string, key []byte, done func() error) error {
	return tm.export(models.TenantID(provider, oauthID), key, nil, done)
}

// export closes the tenant's connection, as its key changes, and rewrites
// its database from the from key to the to key, then runs done (if any),
// all without the database being opened meanwhile.
func (tm *TenantManager) export(tenantID string, from, to []byte, done func() error) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.closeTenant(tenantID)
	if err := exportDB(tm.dbPath(tenantID), from, to); err != nil {
		return err
	}
	// The previous database kept by an import has the old key.
	_ = os.Remove(tm.backupPath(tenantID))
	if done != nil {
		return done()
	}
	return nil
}

// exportDB rewrites the database at path, read with the from key, with the to
// key (nil for plain text) through sqlcipher_export, then replaces it.
func exportDB(path string, from, to []byte) error {
	return exportFile(path, path, from, to)
}

// exportFile writes the database at src, read with the from key, to dst with
// the to key (nil for plain text) through sqlcipher_export, replacing dst.
func exportFile(src, dst string, from, to []byte) error {
	tmp := dst + ".export"
	_ = os.Remove(tmp)

	db, err := openDB(src, from)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := exportTo(context.Background(), db, tmp, to); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	db.Close()

	if err := syncFile(tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("replacing the database: %w", err)
	}
	_ = os.Remove(dst + "-journal")
	return syncFile(filepath.Dir(dst))
}

// exportTo copies the whole database of db to a new database at dst,
// encrypted with the to key (nil for plain text).
func exportTo(ctx context.Context, db *sql.DB, dst string, to []byte) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	toKey := "''"
	if to != nil {
		toKey = `"` + vault.SQLCipherKey(to) + `"`
	}
	if _, err := conn.ExecContext(ctx, "ATTACH DATABASE ? AS export KEY "+toKey, dst); err != nil {
		return fmt.Errorf("attaching the export database: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "SELECT sqlcipher_export('export')"); err != nil {
		_, _ = conn.ExecContext(ctx, "DETACH DATABASE export")
		return fmt.Errorf("exporting the database: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "DETACH DATABASE export"); err != nil {
		return fmt.Errorf("detaching the export database: %w", err)
	}
	return nil
}

func syncFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// Reseed seeds the tenant's database again, e.g. after a reset cleared its
// seeded_at to get the default categories and accounts back.
func (tm *TenantManager) Reseed(ctx context.Context, provider models.OAuthProvider, oauthID string, key []byte) error {
	db, _, err := tm.GetTenant(provider, oauthID, key)
	if err != nil {
		return err
	}

	tm.mu.RLock()
	tenant := tm.connections[models.TenantID(provider, oauthID)]
	tm.mu.RUnlock()
	if tenant == nil {
		return fmt.Errorf("tenant connection not found")
	}

	tenant.seeder.Seed(ctx, db, oauthID)
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
