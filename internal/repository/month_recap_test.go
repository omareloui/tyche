package repository

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"github.com/pressly/goose/v3"
)

// TestRecapQueries checks the month recap's new merchants (first fulfilled
// transaction in the month, net of reimbursements) and expenses (transfer
// pair legs out, reimbursed amounts read).
func TestRecapQueries(t *testing.T) {
	ctx := context.Background()
	db, err := openDB(filepath.Join(t.TempDir(), "t.sqlite"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(db, filepath.Join("..", "..", migrationsDir)); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"PRAGMA foreign_keys = OFF",
		"INSERT INTO currencies (id, string, symbol, kind) VALUES ('egp', 'EGP-T', 'E£T', 'fiat')",
		"INSERT INTO accounts (id, name, kind_id, amount, currency_id) VALUES ('acc', 'Bank', 'k', 0, 'egp')",
		`INSERT INTO counterparties (id, name, is_merchant) VALUES
			('old', 'Old shop', TRUE), ('new', 'New shop', TRUE), ('later', 'Later shop', TRUE),
			('person', 'Ali', FALSE)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	d := func(s string) time.Time { v, _ := time.Parse(time.DateOnly, s); return v }
	type tx struct {
		id, cp, reimburses, pair string
		kind                     int
		amount                   float64
		on                       string
		fulfilled                bool
	}
	for _, x := range []tx{
		{id: "o1", cp: "old", kind: 1, amount: 50, on: "2026-09-10", fulfilled: true},
		{id: "o2", cp: "old", kind: 1, amount: 70, on: "2026-10-10", fulfilled: true},
		{id: "n1", cp: "new", kind: 1, amount: 300, on: "2026-10-05", fulfilled: true},
		{id: "n2", cp: "new", kind: 1, amount: 100, on: "2026-10-20", fulfilled: true},
		{id: "back", kind: 2, amount: 120, on: "2026-10-21", fulfilled: true, reimburses: "n1"},
		{id: "l1", cp: "later", kind: 1, amount: 80, on: "2026-10-25", fulfilled: false},
		{id: "p1", cp: "person", kind: 1, amount: 40, on: "2026-10-03", fulfilled: true},
		{id: "leg", kind: 1, amount: 9000, on: "2026-10-04", fulfilled: true, pair: "legin"},
	} {
		var fulfilled any
		if x.fulfilled {
			fulfilled = d(x.on)
		}
		_, err := db.Exec(`INSERT INTO transactions (id, account_id, amount, kind, scheduled_to, fulfilled_at, counterparty_id, reimburses_id, transfer_pair_id)
			VALUES (?, 'acc', ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''))`,
			x.id, x.amount, x.kind, d(x.on), fulfilled, x.cp, x.reimburses, x.pair)
		if err != nil {
			t.Fatalf("%s: %v", x.id, err)
		}
	}
	q := queries.New(db)
	from, to := d("2026-10-01"), d("2026-11-01")

	cps, err := q.GetFirstSeenCounterparties(ctx, queries.GetFirstSeenCounterpartiesParams{FromDate: from, ToDate: to})
	if err != nil {
		t.Fatal(err)
	}
	if len(cps) != 1 || cps[0].CounterpartyID != "new" || cps[0].Total != 280 {
		t.Errorf("new merchants = %+v, want New shop at 280 (400 less 120 paid back)", cps)
	}

	rows, err := q.GetRecapExpenses(ctx, queries.GetRecapExpensesParams{FromDate: from, ToDate: to})
	if err != nil {
		t.Fatal(err)
	}
	txs := make([]models.Transaction, len(rows))
	var ids []string
	for i, row := range rows {
		txs[i] = recapExpense(row)
		ids = append(ids, row.ID)
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"n1", "n2", "o2", "p1"}) {
		t.Errorf("expenses = %v, want the fulfilled ones of the month but the pair leg", ids)
	}
	def := models.Currency{String: "EGP-T"}
	largest, _ := models.LargestExpenses(txs, def, 2)
	if len(largest) != 2 || largest[0].Tx.ID != "n1" || largest[0].Amount != 180 || largest[1].Tx.ID != "n2" {
		t.Errorf("largest = %+v, want n1 (net 180) then n2", largest)
	}
}
