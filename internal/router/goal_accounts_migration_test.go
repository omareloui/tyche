package router_test

import (
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"
)

// TestGoalAccountsMigration checks rebuilding transactions, recurrent_txs and
// accounts (dropping the goal links, soft deleted accounts) keeps every row
// and the links between them.
func TestGoalAccountsMigration(t *testing.T) {
	setupTenant(t)

	db, err := sql.Open("sqlite3", "data/migration.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// As the app: one connection, foreign keys on.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpTo(db, "sql/schema", 20261102000000); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO currencies (id, string, symbol, kind) VALUES ('c', 'EGP', 'E£', 'Fiat')`,
		`INSERT INTO account_kinds (id, enum) VALUES ('k', 2)`,
		`INSERT INTO accounts (id, name, kind_id, amount, currency_id) VALUES ('a', 'Bank', 'k', 10, 'c'), ('b', 'Cash', 'k', 5, 'c')`,
		`INSERT INTO user (id, oauth_id, oauth_provider, email, first_name, last_name, default_currency_id, default_account_id)
			VALUES ('u', 'o', 1, 'e@example.com', 'Fi', 'La', 'c', 'a')`,
		`INSERT INTO goals (id, name, price, currency_id) VALUES ('g', 'Car', 100, 'c')`,
		`INSERT INTO recurrent_txs (id, every, unit, account_id, kind, starts_at, goal_id) VALUES ('r', 1, 1, 'a', 1, '2026-01-01', 'g')`,
		`INSERT INTO transactions (id, account_id, kind, scheduled_to, recurrent_id, goal_id) VALUES ('t1', 'a', 1, '2026-01-01', 'r', 'g')`,
		`INSERT INTO transactions (id, account_id, kind, scheduled_to, reimburses_id, transfer_pair_id) VALUES ('t2', 'b', 2, '2026-01-01', 't1', 't1')`,
		`UPDATE transactions SET transfer_pair_id = 't2' WHERE id = 't1'`,
		`INSERT INTO sms_messages (id, sender, body, hash, received_at, status, transaction_id) VALUES ('s', 'Bank', 'b', 'h', '2026-01-01', 'parsed', 't1')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	if err := goose.Up(db, "sql/schema"); err != nil {
		t.Fatal(err)
	}

	var recurrentID, reimburses, pair sql.NullString
	if err := db.QueryRow("SELECT recurrent_id, transfer_pair_id FROM transactions WHERE id = 't1'").Scan(&recurrentID, &pair); err != nil {
		t.Fatal(err)
	}
	if recurrentID.String != "r" || pair.String != "t2" {
		t.Errorf("t1 links = %v, %v", recurrentID, pair)
	}
	if err := db.QueryRow("SELECT reimburses_id, transfer_pair_id FROM transactions WHERE id = 't2'").Scan(&reimburses, &pair); err != nil {
		t.Fatal(err)
	}
	if reimburses.String != "t1" || pair.String != "t1" {
		t.Errorf("t2 links = %v, %v", reimburses, pair)
	}
	var smsTx, dflt sql.NullString
	if err := db.QueryRow("SELECT transaction_id FROM sms_messages").Scan(&smsTx); err != nil || smsTx.String != "t1" {
		t.Errorf("sms transaction = %v (%v)", smsTx, err)
	}
	if err := db.QueryRow("SELECT default_account_id FROM user").Scan(&dflt); err != nil || dflt.String != "a" {
		t.Errorf("default account = %v (%v)", dflt, err)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM recurrent_txs_details").Scan(&n); err != nil || n != 1 {
		t.Errorf("recurrent_txs_details rows = %d (%v)", n, err)
	}
	for _, q := range []string{"SELECT goal_id FROM transactions", "SELECT goal_id FROM recurrent_txs"} {
		if _, err := db.Exec(q); err == nil {
			t.Errorf("%q still works", q)
		}
	}
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		t.Error("broken foreign keys after the migration")
	}
	rows.Close()

	// Names are unique among live accounts only.
	if _, err := db.Exec(`UPDATE accounts SET deleted_at = CURRENT_TIMESTAMP WHERE id = 'b'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO accounts (id, name, kind_id, amount, currency_id) VALUES ('c2', 'Cash', 'k', 0, 'c')`); err != nil {
		t.Errorf("reusing a deleted account's name: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO accounts (id, name, kind_id, amount, currency_id) VALUES ('c3', 'Bank', 'k', 0, 'c')`); err == nil {
		t.Error("two live accounts with the same name")
	}

	if err := goose.Down(db, "sql/schema"); err != nil {
		t.Fatalf("down: %v", err)
	}
}
