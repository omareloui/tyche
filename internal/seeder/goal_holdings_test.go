package seeder

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/omareloui/tyche/internal/queries"
)

// TestGoalHoldings checks splitting the goals made before goals held money
// per currency: a goal saved for gets its deposits from other currencies (and
// the withdraws its holding covers) moved to a holding, the balances
// following; a bought one is only tied to its account.
func TestGoalHoldings(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "split.sqlite"))
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
	goose.SetLogger(goose.NopLogger())
	if err := goose.Up(db, "../../sql/schema"); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO currencies (id, string, symbol, kind) VALUES ('e', 'egp', 'E£', 'Fiat'), ('u', 'usd', 'U$', 'Fiat')`,
		`INSERT INTO account_kinds (id, enum) VALUES ('kb', 2), ('kg', 6)`,
		`INSERT INTO accounts (id, name, kind_id, amount, currency_id) VALUES ('b', 'Bank', 'kb', 0, 'e'), ('ub', 'Dollars', 'kb', 0, 'u')`,
		`INSERT INTO accounts (id, name, kind_id, amount, currency_id, exclude_from_balance) VALUES
			('ga', 'Goal: Car', 'kg', 280, 'e', TRUE), ('ga2', 'Goal: Phone', 'kg', 0, 'e', TRUE)`,
		`INSERT INTO goals (id, name, price, currency_id, account_id) VALUES ('g', 'Car', 5000, 'e', 'ga')`,
		`INSERT INTO goals (id, name, price, currency_id, account_id, fulfilled_at) VALUES ('g2', 'Phone', 480, 'e', 'ga2', '2026-03-01')`,
		`INSERT INTO transactions (id, account_id, kind, amount, to_account_id, to_amount, scheduled_to, fulfilled_at) VALUES
			('t1', 'b', 3, 1000, 'ga', 1000, '2026-01-01', '2026-01-01'),
			('t2', 'ub', 3, 10, 'ga', 480, '2026-01-02', '2026-01-02'),
			('t3', 'ga', 3, 240, 'ub', 5, '2026-01-03', '2026-01-03'),
			('t4', 'ga', 3, 960, 'ub', 20, '2026-01-04', '2026-01-04'),
			('t5', 'ub', 3, 2, 'ga', 96, '2099-01-01', NULL),
			('p1', 'ub', 3, 10, 'ga2', 480, '2026-02-01', '2026-02-01'),
			('p2', 'ga2', 1, 480, NULL, NULL, '2026-03-01', '2026-03-01')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	split := func() {
		t.Helper()
		ctx := context.Background()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		NewSeeder().goalHoldings(ctx, tx, queries.New(tx))
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	split()
	split() // a second run changes nothing

	var holding string
	var n int
	if err := db.QueryRow(`SELECT id, COUNT(*) OVER () FROM accounts WHERE goal_id = 'g' AND id != 'ga'`).Scan(&holding, &n); err != nil || n != 1 {
		t.Fatalf("holdings of g = %d (%v)", n, err)
	}
	amounts := map[string]float64{}
	rows, err := db.Query(`SELECT id, amount FROM accounts`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		var a float64
		if err := rows.Scan(&id, &a); err != nil {
			t.Fatal(err)
		}
		amounts[id] = a
	}
	rows.Close()
	if amounts["ga"] != 40 || amounts[holding] != 5 || amounts["ga2"] != 0 {
		t.Errorf("balances: own %v, holding %v, bought %v; want 40, 5, 0", amounts["ga"], amounts[holding], amounts["ga2"])
	}

	type leg struct {
		from, to     string
		amount, toAm float64
	}
	want := map[string]leg{
		"t1": {"b", "ga", 1000, 1000},
		"t2": {"ub", holding, 10, 10},
		"t3": {holding, "ub", 5, 5},
		"t4": {"ga", "ub", 960, 20}, // more than the holding had: a conversion
		"t5": {"ub", holding, 2, 2},
		"p1": {"ub", "ga2", 10, 480},
	}
	for id, w := range want {
		var got leg
		if err := db.QueryRow(`SELECT account_id, to_account_id, amount, to_amount FROM transactions WHERE id = ?`, id).
			Scan(&got.from, &got.to, &got.amount, &got.toAm); err != nil {
			t.Fatal(err)
		}
		if got != w {
			t.Errorf("%s = %+v, want %+v", id, got, w)
		}
	}
	for _, id := range []string{"ga", "ga2"} {
		var goal sql.NullString
		if err := db.QueryRow(`SELECT goal_id FROM accounts WHERE id = ?`, id).Scan(&goal); err != nil || !goal.Valid {
			t.Errorf("%s not tied to its goal (%v)", id, err)
		}
	}
}
