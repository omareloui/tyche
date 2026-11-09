package repository

import (
	"context"
	"database/sql"
	"errors"
	"os"
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

func TestConsoleSchema(t *testing.T) {
	db := consoleTestDB(t)
	if _, err := db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, tx_id TEXT REFERENCES txs (id), parent_id TEXT REFERENCES items (id))`); err != nil {
		t.Fatal(err)
	}
	schema, err := consoleSchema(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(schema) != 3 {
		t.Fatalf("got %d tables, want 3 (no views/internal ones)", len(schema))
	}
	ranks := models.ConsoleRanks(schema)
	if ranks["accounts"] != 0 || ranks["txs"] != 1 || ranks["items"] != 2 {
		t.Errorf("ranks %v", ranks)
	}

	// A cycle stays bounded.
	cyc := []models.ConsoleTableInfo{
		{ConsoleTable: models.ConsoleTable{Name: "a"}, FKs: []models.ConsoleFK{{Table: "a", RefTable: "b"}}},
		{ConsoleTable: models.ConsoleTable{Name: "b"}, FKs: []models.ConsoleFK{{Table: "b", RefTable: "a"}}},
	}
	if r := models.ConsoleRanks(cyc); r["a"] >= 2 || r["b"] >= 2 {
		t.Errorf("cycle ranks %v", r)
	}
}

func consoleWriteTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	db := consoleTestDB(t)
	if _, err := db.Exec(`
		CREATE TABLE user (id TEXT PRIMARY KEY, oauth_id TEXT NOT NULL, first_name TEXT, seeded_at TIMESTAMP);
		CREATE TABLE devices (id TEXT PRIMARY KEY);
		CREATE TABLE goose_db_version (id INTEGER PRIMARY KEY, version_id INTEGER);
		INSERT INTO user VALUES ('u1', 'o1', 'Omar', CURRENT_TIMESTAMP);
		INSERT INTO devices VALUES ('d1');
		INSERT INTO goose_db_version VALUES (1, 1);
	`); err != nil {
		t.Fatal(err)
	}
	return db, filepath.Join(t.TempDir(), "console.sqlite.console")
}

func consoleCount(t *testing.T, db *sql.DB, q string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestConsoleWrite checks the write mode writes rows, keeping a copy of the
// database before it to undo.
func TestConsoleWrite(t *testing.T) {
	db, snapshot := consoleWriteTestDB(t)
	ctx := context.Background()

	res, err := consoleWrite(ctx, db, models.ConsoleMaxRows, snapshot, nil, "SELECT COUNT(*) FROM txs")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Write || res.Changed != 0 || res.Rows[0][0].Raw != "700" {
		t.Errorf("select = %+v", res)
	}
	if _, err := os.Stat(snapshot); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("snapshot kept without a change: %v", err)
	}

	res, err = consoleWrite(ctx, db, models.ConsoleMaxRows, snapshot, nil, `
		UPDATE accounts SET name = 'Wallet' WHERE id = 'a1';
		DELETE FROM txs WHERE amount >= 10;
		INSERT INTO accounts VALUES ('a3', 'Card') RETURNING id, name`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed != 1+690+1 {
		t.Errorf("changed = %d, want 692", res.Changed)
	}
	if len(res.Rows) != 1 || res.Rows[0][1].Raw != "Card" {
		t.Errorf("returning rows = %+v", res.Rows)
	}
	if n := consoleCount(t, db, "SELECT COUNT(*) FROM txs"); n != 10 {
		t.Errorf("txs = %d, want 10", n)
	}
	if _, err := db.Exec("UPDATE user SET first_name = 'O'"); err != nil {
		t.Errorf("writing after the console: %v", err)
	}

	// The snapshot is the database before the write.
	snap, err := openDB(snapshot, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer snap.Close()
	if n := consoleCount(t, snap, "SELECT COUNT(*) FROM txs"); n != 700 {
		t.Errorf("snapshot txs = %d, want 700", n)
	}

	// The user row: updated but its identity.
	if _, err := consoleWrite(ctx, db, models.ConsoleMaxRows, snapshot, nil, "UPDATE user SET first_name = 'Om'"); err != nil {
		t.Errorf("updating the user's name: %v", err)
	}
}

func TestConsoleWriteDenies(t *testing.T) {
	db, snapshot := consoleWriteTestDB(t)
	ctx := context.Background()
	other := filepath.Join(t.TempDir(), "other.sqlite")

	for _, q := range []string{
		"UPDATE sqlite_master SET sql = ''",
		"DELETE FROM goose_db_version",
		"INSERT INTO goose_db_version VALUES (2, 2)",
		"DELETE FROM devices",
		"INSERT INTO devices VALUES ('d2')",
		"UPDATE user SET oauth_id = 'x'",
		"UPDATE user SET seeded_at = NULL",
		"DELETE FROM user",
		"INSERT INTO user VALUES ('u2', 'o2', 'x', NULL)",
		"CREATE TABLE x (a)",
		"CREATE TEMP TABLE x (a)",
		"CREATE TRIGGER x AFTER INSERT ON txs BEGIN DELETE FROM txs; END",
		"CREATE VIEW x AS SELECT 1",
		"CREATE INDEX x ON txs (amount)",
		"DROP TABLE txs",
		"DROP VIEW big_txs",
		"ALTER TABLE txs ADD COLUMN y",
		"ATTACH '" + other + "' AS o",
		"DETACH main",
		"PRAGMA user_version = 3",
		"PRAGMA foreign_keys = OFF",
		"PRAGMA key = 'x'",
		"PRAGMA rekey = 'x'",
		"PRAGMA writable_schema = ON",
		"SELECT sqlcipher_export('main')",
		"SELECT load_extension('x')",
		"BEGIN",
		"COMMIT",
		"ROLLBACK",
		"SAVEPOINT s",
		"RELEASE s",
		"VACUUM",
		"VACUUM INTO '" + other + "'",
		"ANALYZE",
		"REINDEX",
		// A foreign key broken: the whole run is rolled back.
		"DELETE FROM txs WHERE id = 't0000'; INSERT INTO txs (id, account_id) VALUES ('x', 'nope')",
		"DELETE FROM txs; DROP TABLE accounts",
	} {
		if _, err := consoleWrite(ctx, db, models.ConsoleMaxRows, snapshot, nil, q); err == nil {
			t.Errorf("%q got through", q)
		}
	}

	if n := consoleCount(t, db, "SELECT COUNT(*) FROM txs"); n != 700 {
		t.Errorf("txs = %d, want 700", n)
	}
	if n := consoleCount(t, db, "SELECT COUNT(*) FROM devices"); n != 1 {
		t.Errorf("devices = %d, want 1", n)
	}
	if n := consoleCount(t, db, "SELECT COUNT(*) FROM user WHERE oauth_id = 'o1' AND seeded_at IS NOT NULL"); n != 1 {
		t.Errorf("the user's identity changed")
	}
	if _, err := os.Stat(other); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("wrote another database: %v", err)
	}
	if _, err := os.Stat(snapshot); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("snapshot kept without a change: %v", err)
	}

	// The connection is back as it was: no transaction, no authorizer.
	if _, err := db.Exec("BEGIN; CREATE TABLE y (a); INSERT INTO devices VALUES ('d3'); COMMIT"); err != nil {
		t.Errorf("writing after the console: %v", err)
	}
	if fk := consoleCount(t, db, "PRAGMA foreign_keys"); fk != 1 {
		t.Errorf("foreign_keys = %d, want on", fk)
	}
}

func TestConsoleWriteTimeout(t *testing.T) {
	db, snapshot := consoleWriteTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	_, err := consoleWrite(ctx, db, models.ConsoleMaxRows, snapshot, nil, `
		DELETE FROM txs;
		WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM n) SELECT COUNT(*) FROM n`)
	if err == nil {
		t.Fatal("an endless query wasn't stopped")
	}
	if n := consoleCount(t, db, "SELECT COUNT(*) FROM txs"); n != 700 {
		t.Errorf("txs = %d, want 700 (rolled back)", n)
	}
	if _, err := db.Exec("INSERT INTO devices VALUES ('d2')"); err != nil {
		t.Errorf("writing after the console: %v", err)
	}
}

// TestConsoleSnapshotSwap checks undoing a console write puts the database
// back, and undoing again redoes it.
func TestConsoleSnapshotSwap(t *testing.T) {
	tm := &TenantManager{baseDir: t.TempDir(), connections: map[string]*TenantDB{}}
	const id = "GL_1"
	db, err := openDB(tm.dbPath(id), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE accounts (id TEXT PRIMARY KEY); INSERT INTO accounts VALUES ('a1')"); err != nil {
		t.Fatal(err)
	}
	if _, err := consoleWrite(context.Background(), db, models.ConsoleMaxRows, tm.consoleSnapshotPath(id), nil, "DELETE FROM accounts"); err != nil {
		t.Fatal(err)
	}
	db.Close()

	count := func() int {
		t.Helper()
		db, err := openDB(tm.dbPath(id), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		return consoleCount(t, db, "SELECT COUNT(*) FROM accounts")
	}
	for i, want := range []int{1, 0, 1} {
		if err := tm.swapWith(id, nil, tm.consoleSnapshotPath(id)); err != nil {
			t.Fatal(err)
		}
		if n := count(); n != want {
			t.Errorf("swap %d: accounts = %d, want %d", i+1, n, want)
		}
	}
}

func TestConsoleStatements(t *testing.T) {
	for q, want := range map[string][]string{
		"":                                    nil,
		" ; -- only a comment\n; /* ; */ ;":   nil,
		"SELECT 1":                            {"SELECT 1"},
		"SELECT 1;\nSELECT 2;":                {"SELECT 1", "SELECT 2"},
		"SELECT 'a;b', \"c;\", `d;`, [e;];":   {"SELECT 'a;b', \"c;\", `d;`, [e;]"},
		"SELECT 'it''s; ok'; SELECT 2":        {"SELECT 'it''s; ok'", "SELECT 2"},
		"-- a; b\nSELECT 1 /* ; */; SELECT 2": {"-- a; b\nSELECT 1 /* ; */", "SELECT 2"},
	} {
		got := consoleStatements(q)
		if len(got) != len(want) {
			t.Errorf("%q = %q, want %q", q, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%q = %q, want %q", q, got, want)
				break
			}
		}
	}
}
