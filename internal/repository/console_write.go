package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-sqlite3"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"go.uber.org/zap"
)

// The console's write mode (POST /console) runs the user's SQL in its own
// transaction with consoleWriteAuthorizer: rows of the app's tables are
// inserted, updated and deleted, nothing else (no schema change, ATTACH,
// transaction, key pragma). The app's own rules aren't checked. Before a run
// the database is exported to a snapshot, kept when the run changed rows, so
// RestoreConsoleSnapshot puts it back as it was before the last write.

// consoleWriteDenied are the tables the console never writes: the internal
// ones aside (consoleInternal), the devices (their key slots live in the
// shared DB).
var consoleWriteDenied = map[string]bool{
	"devices": true,
}

// consoleUserLocked are the user row's columns the console never writes: its
// identity (signing in finds the database and the row by them) and seeded_at
// (cleared, the defaults are seeded again).
var consoleUserLocked = map[string]bool{
	"id":             true,
	"oauth_id":       true,
	"oauth_provider": true,
	"email":          true,
	"verified_email": true,
	"seeded_at":      true,
}

func consoleInternal(table string) bool {
	return strings.HasPrefix(table, "sqlite_") || table == "goose_db_version"
}

// consoleWriteAuthorizer allows what consoleAuthorizer does, plus writing the
// rows of the main database's tables but the denied ones. The user row can
// only be updated, its locked columns aside.
func consoleWriteAuthorizer(op int, arg1, arg2, db string) int {
	table := strings.ToLower(arg1)
	writable := db == "main" && !consoleInternal(table) && !consoleWriteDenied[table]
	switch op {
	case sqlite3.SQLITE_INSERT, sqlite3.SQLITE_DELETE:
		if writable && table != "user" {
			return sqlite3.SQLITE_OK
		}
		return sqlite3.SQLITE_DENY
	case sqlite3.SQLITE_UPDATE:
		if writable && (table != "user" || !consoleUserLocked[strings.ToLower(arg2)]) {
			return sqlite3.SQLITE_OK
		}
	}
	return consoleAuthorizer(op, arg1, arg2, db)
}

// consoleWrite runs query on db in a transaction with consoleWriteAuthorizer,
// rolled back on any error. The database is exported to snapshot (encrypted
// with key) first; it's kept only when the run changed rows (Changed).
func consoleWrite(ctx context.Context, db *sql.DB, maxRows int, snapshot string, key []byte, query string) (models.ConsoleResult, error) {
	// One connection for all: no write of the app gets between the snapshot
	// and the run.
	conn, err := db.Conn(ctx)
	if err != nil {
		return models.ConsoleResult{}, err
	}
	defer conn.Close()

	// Before the run's timeout: copying a database with receipts takes a while.
	tmp := snapshot + ".tmp"
	_ = os.Remove(tmp)
	defer os.Remove(tmp)
	if err := exportConn(ctx, conn, tmp, key); err != nil {
		return models.ConsoleResult{}, fmt.Errorf("keeping a copy to undo: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, consoleTimeout)
	defer cancel()

	var before int64
	if err := conn.QueryRowContext(ctx, "SELECT total_changes()").Scan(&before); err != nil {
		return models.ConsoleResult{}, consoleErr(ctx, err)
	}
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return models.ConsoleResult{}, consoleErr(ctx, err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		// The run's context may be over: roll back anyway.
		if _, err := conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK"); err != nil {
			logger.FromCtx(ctx).Error("rolling back a console write", zap.Error(err))
		}
	}()

	res, err := withConsoleAuthorizer(ctx, conn, consoleWriteAuthorizer, func() (models.ConsoleResult, error) {
		// The driver's Query runs only the last statement: the others are run
		// first, the last one's rows read.
		stmts := consoleStatements(query)
		if len(stmts) == 0 {
			return models.ConsoleResult{}, nil
		}
		for _, stmt := range stmts[:len(stmts)-1] {
			if _, err := conn.ExecContext(ctx, stmt); err != nil {
				return models.ConsoleResult{}, consoleErr(ctx, err)
			}
		}
		return consoleRows(ctx, conn, maxRows, stmts[len(stmts)-1])
	})
	if err != nil {
		return models.ConsoleResult{}, err
	}

	var after int64
	if err := conn.QueryRowContext(ctx, "SELECT total_changes()").Scan(&after); err != nil {
		return models.ConsoleResult{}, consoleErr(ctx, err)
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return models.ConsoleResult{}, consoleErr(ctx, err)
	}
	committed = true
	res.Write, res.Changed = true, after-before

	if res.Changed > 0 {
		if err := syncFile(tmp); err != nil {
			return res, err
		}
		if err := os.Rename(tmp, snapshot); err != nil {
			return res, fmt.Errorf("keeping the copy to undo: %w", err)
		}
		if err := syncFile(filepath.Dir(snapshot)); err != nil {
			return res, err
		}
	}
	return res, nil
}

// consoleStatements splits query at the semicolons ending its statements
// (not the ones in strings, quoted names or comments), leaving out the empty
// ones. A wrong split only fails, as every part is authorized on its own.
func consoleStatements(query string) []string {
	var stmts []string
	start, content := 0, false
	flush := func(end int) {
		if content {
			stmts = append(stmts, strings.TrimSpace(query[start:end]))
		}
		start, content = end+1, false
	}
	for i := 0; i < len(query); i++ {
		switch c := query[i]; {
		case c == '\'' || c == '"' || c == '`' || c == '[':
			end := c
			if c == '[' {
				end = ']'
			}
			// A doubled quote is an escaped one: the scan goes on after it.
			j := strings.IndexByte(query[i+1:], end)
			if j < 0 {
				i = len(query)
			} else {
				i += j + 1
			}
			content = true
		case c == '-' && strings.HasPrefix(query[i:], "--"):
			if j := strings.IndexByte(query[i:], '\n'); j < 0 {
				i = len(query)
			} else {
				i += j
			}
		case c == '/' && strings.HasPrefix(query[i:], "/*"):
			if j := strings.Index(query[i+2:], "*/"); j < 0 {
				i = len(query)
			} else {
				i += j + 3
			}
		case c == ';':
			flush(i)
		case c != ' ' && c != '\t' && c != '\n' && c != '\r':
			content = true
		}
	}
	flush(len(query))
	return stmts
}

// withConsoleAuthorizer runs fn with auth as conn's authorizer and the
// console's length limit, both taken back after.
func withConsoleAuthorizer(ctx context.Context, conn *sql.Conn, auth func(int, string, string, string) int, fn func() (models.ConsoleResult, error)) (models.ConsoleResult, error) {
	var oldLength int
	if err := conn.Raw(func(dc any) error {
		c, ok := dc.(*sqlite3.SQLiteConn)
		if !ok {
			return fmt.Errorf("unexpected driver connection %T", dc)
		}
		c.RegisterAuthorizer(auth)
		oldLength = c.SetLimit(sqlite3.SQLITE_LIMIT_LENGTH, consoleMaxLength)
		return nil
	}); err != nil {
		return models.ConsoleResult{}, err
	}
	defer func() {
		_ = conn.Raw(func(dc any) error {
			c := dc.(*sqlite3.SQLiteConn)
			c.RegisterAuthorizer(nil)
			c.SetLimit(sqlite3.SQLITE_LIMIT_LENGTH, oldLength)
			return nil
		})
	}()
	return fn()
}

// ConsoleWrite runs the user's SQL in the console's write mode.
func (r *Repository) ConsoleWrite(ctx context.Context, query string) (models.ConsoleResult, error) {
	provider, oauthID, err := tenantFromCtx(ctx)
	if err != nil {
		return models.ConsoleResult{}, err
	}
	key, err := r.tenantKey(ctx, provider, oauthID)
	if err != nil {
		return models.ConsoleResult{}, err
	}
	db, _, err := r.dbFromCtx(ctx)
	if err != nil {
		return models.ConsoleResult{}, err
	}
	snapshot := r.tm.consoleSnapshotPath(models.TenantID(provider, oauthID))
	res, err := consoleWrite(ctx, db, models.ConsoleMaxRows, snapshot, key, query)

	l := logger.FromCtx(ctx)
	if err != nil {
		l.Info("console write failed", zap.Bool("denied", errors.Is(err, ErrConsoleDenied)))
		return res, err
	}
	l.Info("console write", zap.Int64("changed", res.Changed), zap.Int("rows", len(res.Rows)), zap.Duration("took", res.Took))
	if res.Changed > 0 {
		// Its recurrent transactions may have changed: written again on the
		// next request.
		r.forgetDailyUpkeepOf(ctx)
	}
	return res, nil
}

// HasConsoleSnapshot reports whether there's a console write to undo.
func (r *Repository) HasConsoleSnapshot(ctx context.Context) bool {
	tenantID, err := TenantIDFromCtx(ctx)
	if err != nil {
		return false
	}
	_, err = os.Stat(r.tm.consoleSnapshotPath(tenantID))
	return err == nil
}

// RestoreConsoleSnapshot swaps the tenant's database with the one before the
// last console write, so restoring again redoes it.
func (r *Repository) RestoreConsoleSnapshot(ctx context.Context) error {
	defer r.forgetDailyUpkeepOf(ctx)
	provider, oauthID, err := tenantFromCtx(ctx)
	if err != nil {
		return err
	}
	key, err := r.tenantKey(ctx, provider, oauthID)
	if err != nil {
		return err
	}
	tenantID := models.TenantID(provider, oauthID)
	return r.tm.swapWith(tenantID, key, r.tm.consoleSnapshotPath(tenantID))
}

func (tm *TenantManager) consoleSnapshotPath(tenantID string) string {
	return tm.dbPath(tenantID) + ".console"
}
