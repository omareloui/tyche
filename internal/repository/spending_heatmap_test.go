package repository

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/queries"
	"github.com/pressly/goose/v3"
)

// TestGetSpendingTimes checks the heatmap's rows follow the analysis rule:
// expenses net of their reimbursements, refunds negative, reimbursements,
// transfer pair legs, goals' txs and unfulfilled ones left out, times kept.
func TestGetSpendingTimes(t *testing.T) {
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
		"INSERT INTO accounts (id, name, kind_id, amount, currency_id, goal_id) VALUES ('gacc', 'Goal: Car (EGP)', 'k', 0, 'egp', 'g')",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	eet := time.FixedZone("", 3*3600)
	at := time.Date(2026, 10, 22, 21, 30, 0, 0, eet)
	type tx struct {
		id, acc, reimburses, pair string
		kind                      int
		amount                    float64
		fulfilled                 bool
	}
	for _, x := range []tx{
		{id: "exp", acc: "acc", kind: 1, amount: 300, fulfilled: true},
		{id: "back", acc: "acc", kind: 2, amount: 120, fulfilled: true, reimburses: "exp"},
		{id: "refund", acc: "acc", kind: 5, amount: 20, fulfilled: true},
		{id: "income", acc: "acc", kind: 2, amount: 1000, fulfilled: true},
		{id: "leg", acc: "acc", kind: 1, amount: 9000, fulfilled: true, pair: "legin"},
		{id: "goal", acc: "gacc", kind: 1, amount: 5000, fulfilled: true},
		{id: "later", acc: "acc", kind: 1, amount: 80},
	} {
		var fulfilled any
		if x.fulfilled {
			fulfilled = at
		}
		_, err := db.Exec(`INSERT INTO transactions (id, account_id, amount, kind, scheduled_to, fulfilled_at, reimburses_id, transfer_pair_id)
			VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''))`,
			x.id, x.acc, x.amount, x.kind, at, fulfilled, x.reimburses, x.pair)
		if err != nil {
			t.Fatalf("%s: %v", x.id, err)
		}
	}

	rows, err := queries.New(db).GetSpendingTimes(ctx, queries.GetSpendingTimesParams{
		FromDate: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		ToDate:   time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	var amounts []float64
	for _, row := range rows {
		amounts = append(amounts, row.Amount)
		if h := row.ScheduledTo.Hour(); h != 21 {
			t.Errorf("hour = %d, want the stored 21:30 kept", h)
		}
	}
	slices.Sort(amounts)
	if !slices.Equal(amounts, []float64{-20, 180}) {
		t.Errorf("amounts = %v, want the refund (-20) and the expense net of its reimbursement (180)", amounts)
	}
}
