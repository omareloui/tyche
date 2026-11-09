package repository

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

func consoleTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := openDB(filepath.Join(t.TempDir(), "console.sqlite"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`
		CREATE TABLE accounts (id TEXT PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE txs (id TEXT PRIMARY KEY, account_id TEXT REFERENCES accounts (id), amount REAL, note TEXT, img BLOB);
		CREATE INDEX txs_account ON txs (account_id);
		CREATE VIEW big_txs AS SELECT * FROM txs WHERE amount > 100;
		INSERT INTO accounts VALUES ('a1', 'Cash'), ('a2', 'Bank');
	`); err != nil {
		t.Fatal(err)
	}
	// 700 txs t0000…t0699 of amount i, the odd ones in a2.
	if _, err := db.Exec(`
		WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM n WHERE i < 699)
		INSERT INTO txs SELECT printf('t%04d', i), IIF(i % 2 = 1, 'a2', 'a1'), i, NULL, x'0102' FROM n`); err != nil {
		t.Fatal(err)
	}
	return db
}

// TestConsoleReads checks the console reads: selects, CTEs, the schema's
// pragmas, at most the rows asked.
func TestConsoleReads(t *testing.T) {
	db := consoleTestDB(t)
	ctx := context.Background()

	for _, q := range []string{
		"SELECT * FROM txs",
		"WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 10) SELECT i FROM n",
		"SELECT name FROM pragma_table_info('txs')",
		"PRAGMA table_info(txs)",
		"PRAGMA foreign_key_list(txs)",
		"SELECT account_id, COUNT(*), SUM(amount) FROM txs GROUP BY 1",
		"SELECT * FROM big_txs",
		"SELECT * FROM sqlite_schema",
		"SELECT json_object('a', 1), upper('x'), date('now')",
	} {
		if _, err := consoleQuery(ctx, db, models.ConsoleMaxRows, q); err != nil {
			t.Errorf("%q: %v", q, err)
		}
	}

	res, err := consoleQuery(ctx, db, models.ConsoleMaxRows, "SELECT id, account_id, amount, note, img FROM txs")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != models.ConsoleMaxRows || !res.Truncated {
		t.Errorf("got %d rows (truncated %v), want %d truncated", len(res.Rows), res.Truncated, models.ConsoleMaxRows)
	}
	row := res.Rows[1]
	if row[1].Raw != "a2" || !row[2].Number || !row[3].Null || row[4].Blob != 2 {
		t.Errorf("unexpected row %+v", row)
	}
}

// TestConsoleDenies checks nothing but reading gets through, and the
// connection writes as usual afterwards.
func TestConsoleDenies(t *testing.T) {
	db := consoleTestDB(t)
	ctx := context.Background()
	other := filepath.Join(t.TempDir(), "other.sqlite")

	for _, q := range []string{
		"INSERT INTO accounts VALUES ('a3', 'x')",
		"UPDATE accounts SET name = 'x'",
		"UPDATE sqlite_master SET sql = ''",
		"DELETE FROM txs",
		"CREATE TABLE x (a)",
		"CREATE TEMP TABLE x (a)",
		"DROP TABLE txs",
		"ALTER TABLE txs ADD COLUMN y",
		"ATTACH '" + other + "' AS o",
		"ATTACH ':memory:' AS o",
		"DETACH main",
		"PRAGMA user_version = 3",
		"PRAGMA user_version",
		"PRAGMA foreign_keys = OFF",
		"PRAGMA query_only = OFF",
		"PRAGMA key = 'x'",
		"PRAGMA rekey = 'x'",
		"PRAGMA writable_schema = ON",
		"SELECT sqlcipher_export('main')",
		"SELECT load_extension('x')",
		"BEGIN",
		"SAVEPOINT s",
		"VACUUM",
		"VACUUM INTO '" + other + "'",
		"ANALYZE",
		"REINDEX",
		"SELECT 1; DELETE FROM txs",
		"WITH d AS (SELECT 1) DELETE FROM txs",
	} {
		_, err := consoleQuery(ctx, db, models.ConsoleMaxRows, q)
		if err == nil {
			t.Errorf("%q got through", q)
		}
	}

	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM txs").Scan(&n); err != nil || n != 700 {
		t.Fatalf("txs = %d (%v), want 700", n, err)
	}
	if _, err := db.Exec("INSERT INTO accounts VALUES ('a3', 'Card')"); err != nil {
		t.Errorf("writing after the console: %v", err)
	}
	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Errorf("foreign_keys = %d (%v), want on", fk, err)
	}
	var qo int
	if err := db.QueryRow("PRAGMA query_only").Scan(&qo); err != nil || qo != 0 {
		t.Errorf("query_only = %d (%v), want off", qo, err)
	}
	if _, err := db.Exec("SELECT zeroblob(64 << 20)"); err != nil {
		t.Errorf("length limit not restored: %v", err)
	}
}

func TestConsoleLimits(t *testing.T) {
	db := consoleTestDB(t)
	ctx := context.Background()

	if _, err := consoleQuery(ctx, db, 1, "SELECT zeroblob(100 << 20)"); err == nil {
		t.Error("a 100 MB blob got through")
	}

	start := time.Now()
	_, err := consoleQuery(ctx, db, 1, "WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n) SELECT COUNT(*) FROM n")
	if err == nil {
		t.Fatal("an endless query returned")
	}
	if took := time.Since(start); took > consoleTimeout+2*time.Second {
		t.Errorf("stopped after %s", took)
	}
	if _, err := db.Exec("INSERT INTO accounts VALUES ('a4', 'Late')"); err != nil {
		t.Errorf("writing after a timeout: %v", err)
	}
}

func TestConsoleBrowse(t *testing.T) {
	db := consoleTestDB(t)
	ctx := context.Background()

	tables, err := consoleTables(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if tables[0].Name != "accounts" || tables[0].Rows != 2 || tables[len(tables)-1].Name != "sqlite_schema" {
		t.Errorf("unexpected tables %+v", tables)
	}

	info, err := consoleTableInfo(ctx, db, tables, "txs")
	if err != nil {
		t.Fatal(err)
	}
	if fk := info.FK("account_id"); fk == nil || fk.RefTable != "accounts" || fk.To != "id" {
		t.Errorf("account_id's fk = %+v", fk)
	}
	if len(info.Indexes) == 0 {
		t.Error("no indexes")
	}
	acc, err := consoleTableInfo(ctx, db, tables, "accounts")
	if err != nil {
		t.Fatal(err)
	}
	if len(acc.RefBy) != 1 || acc.RefBy[0].Table != "txs" || acc.RefBy[0].From != "account_id" {
		t.Errorf("accounts referenced by %+v", acc.RefBy)
	}

	res, total, err := consoleBrowseTable(ctx, db, info, models.ConsoleBrowse{Sort: "amount", Desc: true, Col: "account_id", Val: "a2", Page: 1})
	if err != nil {
		t.Fatal(err)
	}
	if total != 350 || len(res.Rows) != models.ConsolePageSize || res.Rows[0][2].Raw != "499" {
		t.Errorf("total %d, %d rows, first %+v", total, len(res.Rows), res.Rows[0][2])
	}

	if _, err := consoleTableInfo(ctx, db, tables, "nope"); !errors.Is(err, ErrConsoleNoTable) {
		t.Errorf("unknown table: %v", err)
	}
	if _, _, err := consoleBrowseTable(ctx, db, info, models.ConsoleBrowse{Sort: `amount" ; DROP TABLE txs; --`}); !errors.Is(err, ErrConsoleNoTable) {
		t.Errorf("unknown column: %v", err)
	}
}
