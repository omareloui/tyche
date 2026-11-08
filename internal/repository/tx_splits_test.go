package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"testing"

	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"github.com/pressly/goose/v3"
)

// splitTestDB is a migrated tenant DB with a transaction "tx" and the
// counterparties "ali" and "sara" (parents written with the foreign keys off).
func splitTestDB(t *testing.T) (*sql.DB, *queries.Queries) {
	t.Helper()
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
		"INSERT INTO transactions (id, account_id, amount, kind, scheduled_to) VALUES ('tx', 'acc', 100, 1, CURRENT_TIMESTAMP)",
		"INSERT INTO counterparties (id, name) VALUES ('ali', 'Ali'), ('sara', 'Sara')",
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return db, queries.New(db)
}

func TestItemSharesKeptOnReplace(t *testing.T) {
	ctx := context.Background()
	_, q := splitTestDB(t)
	owner := itemOwner{txID: "tx"}

	items := []models.TxItem{
		{ID: "pizza", Name: "Pizza", Total: 60, SharedWith: []models.ItemShare{{Person: models.SplitMe, Units: 1}, {Person: "ali", Units: 1}}},
		{ID: "fries", Name: "Fries", Total: 30, SharedByAll: true},
		{ID: "soda", Name: "Soda", Total: 10},
		{ID: "tip", Kind: models.TxItemTip, Total: 5, SplitMode: models.ChargeSplitByShare},
	}
	if err := replaceItemsWithTxn(ctx, q, owner, items); err != nil {
		t.Fatal(err)
	}

	// The form sends the lines without their shares; a new one comes too.
	edited := []models.TxItem{
		{ID: "pizza", Name: "Pizza", Total: 65},
		{ID: "fries", Name: "Fries", Total: 30},
		{Name: "Cake", Total: 20},
		{ID: "tip", Kind: models.TxItemTip, Total: 5},
	}
	if err := replaceItemsWithTxn(ctx, q, owner, edited); err != nil {
		t.Fatal(err)
	}

	shares, err := itemSharesOf(ctx, q, "tx")
	if err != nil {
		t.Fatal(err)
	}
	if got := shares["pizza"]; len(got) != 2 || !slices.Contains(got, models.ItemShare{Person: models.SplitMe, Units: 1}) || !slices.Contains(got, models.ItemShare{Person: "ali", Units: 1}) {
		t.Errorf("pizza shared with %v, want me and ali", got)
	}
	if len(shares) != 1 {
		t.Errorf("shares = %v, want only pizza's", shares)
	}
	rows, err := q.GetTransactionItemsOf(ctx, sql.NullString{String: "tx", Valid: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == "tip" && models.ChargeSplitModeOf(row.SplitMode) != models.ChargeSplitByShare {
			t.Errorf("tip split mode = %d, want by share kept", row.SplitMode)
		}
		if want := row.ID == "fries"; row.SharedByAll != want {
			t.Errorf("%s shared by all = %v, want %v", row.Name, row.SharedByAll, want)
		}
	}
}

func TestSplitFollowsCounterparties(t *testing.T) {
	ctx := context.Background()
	db, q := splitTestDB(t)

	if err := replaceItemsWithTxn(ctx, q, itemOwner{txID: "tx"}, []models.TxItem{
		{ID: "a", Name: "A", Quantity: 3, Total: 30, SharedWith: []models.ItemShare{{Person: "ali", Units: 1}, {Person: "sara", Units: 2}}},
		{ID: "b", Name: "B", Total: 10, SharedWith: []models.ItemShare{{Person: "sara", Units: 1}}},
	}); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"ali", "sara"} {
		if err := q.AddTxSplitPerson(ctx, queries.AddTxSplitPersonParams{TransactionID: "tx", CounterpartyID: id, Position: int64(i)}); err != nil {
			t.Fatal(err)
		}
	}

	// Merging sara into ali: ali takes her place, once per item.
	if err := q.ReassignTxSplitPeople(ctx, queries.ReassignTxSplitPeopleParams{ToID: "ali", FromID: "sara"}); err != nil {
		t.Fatal(err)
	}
	to, from := sql.NullString{String: "ali", Valid: true}, sql.NullString{String: "sara", Valid: true}
	if err := q.MergeTxItemShareUnits(ctx, queries.MergeTxItemShareUnitsParams{ToID: to, FromID: from}); err != nil {
		t.Fatal(err)
	}
	if err := q.ReassignTxItemShares(ctx, queries.ReassignTxItemSharesParams{ToID: to, FromID: from}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DELETE FROM counterparties WHERE id = 'sara'"); err != nil {
		t.Fatal(err)
	}

	people, err := q.GetTxSplitPeople(ctx, "tx")
	if err != nil {
		t.Fatal(err)
	}
	if len(people) != 1 || people[0].ID != "ali" {
		t.Errorf("people = %+v, want ali alone", people)
	}
	shares, err := itemSharesOf(ctx, q, "tx")
	if err != nil {
		t.Fatal(err)
	}
	// Ali takes Sara's units of a too.
	if !slices.Equal(shares["a"], []models.ItemShare{{Person: "ali", Units: 3}}) || !slices.Equal(shares["b"], []models.ItemShare{{Person: "ali", Units: 1}}) {
		t.Errorf("shares = %v, want ali's 3 units of a and b", shares)
	}
}
