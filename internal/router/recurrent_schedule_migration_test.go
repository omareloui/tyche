package router_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
)

// TestRecurrentScheduleMigration checks the recurrent txs keep their
// scheduled transactions, their anchor taken from their last scheduled date
// (next_on NULL: continuing from today when it's past), and going down puts
// that date back.
func TestRecurrentScheduleMigration(t *testing.T) {
	setupTenant(t)

	db, err := sql.Open("sqlite3", "data/migration.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpTo(db, "sql/schema", 20261109000000); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO currencies (id, string, symbol, kind) VALUES ('c', 'EGP', 'E£', 'Fiat')`,
		`INSERT INTO account_kinds (id, enum) VALUES ('k', 2)`,
		`INSERT INTO accounts (id, name, kind_id, amount, currency_id) VALUES ('a', 'Bank', 'k', 10, 'c')`,
		`INSERT INTO recurrent_txs (id, every, unit, account_id, kind, starts_at, last_transaction_scheduled_to)
			VALUES ('r', 1, 2, 'a', 1, '2026-01-15', '2026-06-15')`,
		`INSERT INTO transactions (id, account_id, kind, scheduled_to, fulfilled_at, recurrent_id)
			VALUES ('t1', 'a', 1, '2026-05-15', '2026-05-15', 'r'), ('t2', 'a', 1, '2026-06-15', NULL, 'r')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	if err := goose.Up(db, "sql/schema"); err != nil {
		t.Fatal(err)
	}

	var anchor time.Time
	var nextIndex int
	var nextOn sql.NullTime
	if err := db.QueryRow("SELECT anchor, next_index, next_on FROM recurrent_txs WHERE id = 'r'").Scan(&anchor, &nextIndex, &nextOn); err != nil {
		t.Fatal(err)
	}
	if anchor.Format(time.DateOnly) != "2026-06-15" || nextIndex != 1 || nextOn.Valid {
		t.Errorf("anchor %s, next %d, next_on %v", anchor, nextIndex, nextOn)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM transactions WHERE recurrent_id = 'r' AND NOT recurrent_overridden").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("transactions of it = %d, want 2", n)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM recurrent_txs_details WHERE recurrent_tx_id = 'r'").Scan(&n); err != nil || n != 1 {
		t.Errorf("details view = %d, %v", n, err)
	}

	if err := goose.DownTo(db, "sql/schema", 20261109000000); err != nil {
		t.Fatalf("down: %v", err)
	}
	var last time.Time
	if err := db.QueryRow("SELECT last_transaction_scheduled_to FROM recurrent_txs WHERE id = 'r'").Scan(&last); err != nil {
		t.Fatal(err)
	}
	if last.Format(time.DateOnly) != "2026-06-15" {
		t.Errorf("down: last scheduled = %s", last)
	}
}
