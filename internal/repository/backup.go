package repository

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
)

// InvalidBackupError is returned when an imported file can't be used as the
// tenant's database, Reason says why (shown to the user).
type InvalidBackupError struct {
	Reason string
}

func (e *InvalidBackupError) Error() string {
	return "invalid backup: " + e.Reason
}

func invalidBackup(reason string) error {
	return &InvalidBackupError{Reason: reason}
}

// userIdentityCols are the user columns an import keeps from the current
// database: who signs in to it, not what's in the backup.
var userIdentityCols = []string{
	"id", "email", "verified_email", "first_name", "last_name", "picture",
	"picture_data", "picture_type", "picture_hash", "onboarded_at",
}

// ExportDB writes a plain text copy of the tenant's database (decrypted when
// it's encrypted) to a temp file, which the caller has to remove.
func (r *Repository) ExportDB(ctx context.Context) (string, error) {
	db, _, err := r.dbFromCtx(ctx)
	if err != nil {
		return "", err
	}
	tenantID, err := TenantIDFromCtx(ctx)
	if err != nil {
		return "", err
	}

	f, err := os.CreateTemp(r.tm.baseDir, tenantID+"-*.download")
	if err != nil {
		return "", err
	}
	path := f.Name()
	f.Close()

	if err := exportTo(ctx, db, path, nil); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// ImportDB replaces the tenant's database with the backup read from src, an
// exported (plain text) database. It's checked, migrated and copied into a
// fresh database with the current schema, keeping the current user's
// identity and dropping the devices, then encrypted with the tenant's key
// when it has one. The replaced database is kept (RestoreBackup).
func (r *Repository) ImportDB(ctx context.Context, src io.Reader) error {
	// Its recurrent transactions changed: written again on the next request.
	defer r.forgetTopUpOf(ctx)
	provider, oauthID, err := tenantFromCtx(ctx)
	if err != nil {
		return err
	}
	key, err := r.tenantKey(ctx, provider, oauthID)
	if err != nil {
		return err
	}
	db, _, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	identity, err := currentIdentity(ctx, db, oauthID)
	if err != nil {
		return err
	}

	tenantID := models.TenantID(provider, oauthID)
	upload := r.tm.dbPath(tenantID) + ".upload"
	imported := r.tm.dbPath(tenantID) + ".import"
	defer os.Remove(upload)
	defer os.Remove(imported)

	if err := writeUpload(upload, src); err != nil {
		return err
	}
	if err := r.tm.checkBackup(upload); err != nil {
		return err
	}
	if err := r.tm.buildImport(ctx, upload, imported, provider, oauthID, identity); err != nil {
		return err
	}
	return r.tm.replace(tenantID, imported, key)
}

// HasBackup reports whether there's a database replaced by an import to
// restore.
func (r *Repository) HasBackup(ctx context.Context) bool {
	tenantID, err := TenantIDFromCtx(ctx)
	if err != nil {
		return false
	}
	_, err = os.Stat(r.tm.backupPath(tenantID))
	return err == nil
}

// RestoreBackup swaps the tenant's database with the one replaced by the
// last import, so restoring again undoes it. Its devices are dropped too.
func (r *Repository) RestoreBackup(ctx context.Context) error {
	// Its recurrent transactions changed: written again on the next request.
	defer r.forgetTopUpOf(ctx)
	provider, oauthID, err := tenantFromCtx(ctx)
	if err != nil {
		return err
	}
	key, err := r.tenantKey(ctx, provider, oauthID)
	if err != nil {
		return err
	}
	if err := r.tm.restoreBackup(models.TenantID(provider, oauthID), key); err != nil {
		return err
	}

	db, _, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "DELETE FROM devices")
	return err
}

// currentIdentity is the values of userIdentityCols of the current user.
func currentIdentity(ctx context.Context, db *sql.DB, oauthID string) ([]any, error) {
	vals := make([]any, len(userIdentityCols))
	ptrs := make([]any, len(vals))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	q := "SELECT " + strings.Join(userIdentityCols, ", ") + " FROM user WHERE oauth_id = ?"
	if err := db.QueryRowContext(ctx, q, oauthID).Scan(ptrs...); err != nil {
		return nil, fmt.Errorf("getting the current user: %w", err)
	}
	return vals, nil
}

// writeUpload saves src to path, checking it starts like a plain text sqlite
// database.
func writeUpload(path string, src io.Reader) error {
	header := make([]byte, 16)
	n, err := io.ReadFull(src, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return err
	}
	if !bytes.Equal(header[:n], []byte("SQLite format 3\x00")) {
		return invalidBackup("This file isn't a Tyche backup (or it's encrypted).")
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(header); err != nil {
		return err
	}
	if _, err := io.Copy(f, src); err != nil {
		return err
	}
	return f.Close()
}

// checkBackup checks the uploaded database is sound and not from a newer
// version of the app, then migrates it to the current schema.
func (tm *TenantManager) checkBackup(path string) error {
	db, err := openDB(path, nil)
	if err != nil {
		return invalidBackup("This file isn't a Tyche backup (or it's corrupted).")
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA trusted_schema = OFF"); err != nil {
		return err
	}

	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check(1)").Scan(&integrity); err != nil || integrity != "ok" {
		return invalidBackup("This backup is corrupted.")
	}

	var triggers int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'trigger'").Scan(&triggers); err != nil {
		return err
	}
	if triggers > 0 {
		return invalidBackup("This file isn't a Tyche backup.")
	}

	var version sql.NullInt64
	if err := db.QueryRow("SELECT max(version_id) FROM goose_db_version WHERE is_applied").Scan(&version); err != nil || !version.Valid {
		return invalidBackup("This file isn't a Tyche backup.")
	}
	latest, err := latestMigration()
	if err != nil {
		return err
	}
	if version.Int64 > latest {
		return invalidBackup("This backup is from a newer version of Tyche.")
	}

	if err := tm.runMigrations(db); err != nil {
		logger.Get().Warn("migrating an imported backup", zap.Error(err))
		return invalidBackup("This backup can't be upgraded to the current version.")
	}
	return nil
}

func latestMigration() (int64, error) {
	ms, err := goose.CollectMigrations(migrationsDir, 0, goose.MaxVersion)
	if err != nil {
		return 0, err
	}
	last, err := ms.Last()
	if err != nil {
		return 0, err
	}
	return last.Version, nil
}

// buildImport creates a fresh database at dst with the current schema and
// copies the tables of the (checked) backup at src into it, so nothing but
// rows comes from the file. The user row gets the current identity and the
// devices are dropped (their tokens belong to the replaced database).
func (tm *TenantManager) buildImport(ctx context.Context, src, dst string, provider models.OAuthProvider, oauthID string, identity []any) error {
	_ = os.Remove(dst)
	db, err := openDB(dst, nil)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := tm.runMigrations(db); err != nil {
		return err
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	for _, q := range []string{"PRAGMA foreign_keys = OFF", "PRAGMA trusted_schema = OFF"} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	if _, err := conn.ExecContext(ctx, "ATTACH DATABASE ? AS up KEY ''", src); err != nil {
		return fmt.Errorf("attaching the backup: %w", err)
	}
	defer conn.ExecContext(ctx, "DETACH DATABASE up") //nolint:errcheck

	if err := copyTables(ctx, conn); err != nil {
		return err
	}

	var users int
	if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM main.user").Scan(&users); err != nil {
		return err
	}
	if users != 1 {
		return invalidBackup("This backup doesn't have exactly one user.")
	}
	set := make([]string, 0, len(userIdentityCols)+2)
	for _, c := range userIdentityCols {
		set = append(set, c+" = ?")
	}
	set = append(set, "oauth_id = ?", "oauth_provider = ?")
	args := append(append([]any{}, identity...), oauthID, provider)
	if _, err := conn.ExecContext(ctx, "UPDATE main.user SET "+strings.Join(set, ", "), args...); err != nil {
		return fmt.Errorf("setting the user: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return err
	}
	// With the foreign keys on, so the sms of the devices lose them.
	if _, err := conn.ExecContext(ctx, "DELETE FROM main.devices"); err != nil {
		return err
	}
	rows, err := conn.QueryContext(ctx, "PRAGMA main.foreign_key_check")
	if err != nil {
		return err
	}
	broken := rows.Next()
	rows.Close()
	if broken {
		return invalidBackup("This backup has broken references between its records.")
	}
	return nil
}

// copyTables copies every table of the current schema (main) from the backup
// (up), the columns they share, in one transaction.
func copyTables(ctx context.Context, conn *sql.Conn) error {
	tables, err := queryStrings(ctx, conn, "SELECT name FROM main.sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name != 'goose_db_version'")
	if err != nil {
		return err
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	for _, t := range tables {
		cols, err := queryStrings(ctx, tx, "SELECT name FROM pragma_table_info(?, 'main')", t)
		if err != nil {
			return err
		}
		upCols, err := queryStrings(ctx, tx, "SELECT name FROM pragma_table_info(?, 'up')", t)
		if err != nil {
			return err
		}
		if len(upCols) == 0 {
			return invalidBackup("This backup is missing its " + t + ".")
		}
		has := make(map[string]bool, len(upCols))
		for _, c := range upCols {
			has[c] = true
		}
		var shared []string
		for _, c := range cols {
			if has[c] {
				shared = append(shared, `"`+c+`"`)
			}
		}

		list := strings.Join(shared, ", ")
		q := fmt.Sprintf(`INSERT INTO main."%s" (%s) SELECT %s FROM up."%s"`, t, list, list, t)
		if _, err := tx.ExecContext(ctx, q); err != nil {
			logger.Get().Warn("copying a table of an imported backup", zap.String("table", t), zap.Error(err))
			return invalidBackup("This backup's " + t + " can't be read.")
		}
	}
	return tx.Commit()
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func queryStrings(ctx context.Context, q querier, query string, args ...any) ([]string, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var strs []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		strs = append(strs, s)
	}
	return strs, rows.Err()
}

func (tm *TenantManager) backupPath(tenantID string) string {
	return tm.dbPath(tenantID) + ".bak"
}

// replace makes the plain text database at src the tenant's, encrypted with
// key (nil for plain text), keeping the current one as its backup.
func (tm *TenantManager) replace(tenantID, src string, key []byte) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.closeTenant(tenantID)

	path, bak := tm.dbPath(tenantID), tm.backupPath(tenantID)
	if err := os.Rename(path, bak); err != nil {
		return fmt.Errorf("keeping the current database: %w", err)
	}
	_ = os.Remove(path + "-journal")
	if err := exportFile(src, path, nil, key); err != nil {
		if rerr := os.Rename(bak, path); rerr != nil {
			logger.Get().Error("restoring the database after a failed import", zap.String("tenantID", tenantID), zap.Error(rerr))
		}
		return err
	}
	return nil
}

// restoreBackup swaps the tenant's database with its backup. Both have the
// same key, as changing it deletes the backup.
func (tm *TenantManager) restoreBackup(tenantID string, key []byte) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	path, bak := tm.dbPath(tenantID), tm.backupPath(tenantID)
	if _, err := os.Stat(bak); err != nil {
		return err
	}
	db, err := openDB(bak, key)
	if err != nil {
		return fmt.Errorf("opening the backup: %w", err)
	}
	db.Close()

	tm.closeTenant(tenantID)
	swap := path + ".swap"
	if err := os.Rename(path, swap); err != nil {
		return err
	}
	if err := os.Rename(bak, path); err != nil {
		_ = os.Rename(swap, path)
		return err
	}
	if err := os.Rename(swap, bak); err != nil {
		return err
	}
	return syncFile(tm.baseDir)
}

// closeTenant closes the tenant's open connection, if any; tm.mu must be held.
func (tm *TenantManager) closeTenant(tenantID string) {
	if tenant, ok := tm.connections[tenantID]; ok {
		tenant.db.Close()
		delete(tm.connections, tenantID)
	}
}
