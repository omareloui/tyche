package repository

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/pressly/goose/v3"
)

// TestItemsBackfill migrates a DB with receipt lines written before the items
// catalog: lines spelled alike become one item, charges stay unlinked, the
// merchants are linked.
func TestItemsBackfill(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join("..", "..", migrationsDir)
	db, err := openDB(filepath.Join(t.TempDir(), "t.sqlite"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	p, err := newMigrationProvider(db, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, itemsBackfillVersion-1); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"PRAGMA foreign_keys = OFF",
		"INSERT INTO counterparties (id, name) VALUES ('shop', 'Shop'), ('mart', 'Mart')",
		"INSERT INTO transactions (id, account_id, amount, kind, scheduled_to, counterparty_id) VALUES ('t1', 'acc', 30, 1, '2026-01-01 10:00:00', 'shop'), ('t2', 'acc', 20, 1, '2026-02-01 10:00:00', NULL)",
		"INSERT INTO recurrent_txs (id, every, unit, account_id, amount, kind, counterparty_id, starts_at) VALUES ('r1', 1, 2, 'acc', 10, 1, 'mart', '2025-12-01 10:00:00')",
		`INSERT INTO transaction_items (id, transaction_id, recurrent_id, kind, name, total) VALUES
			('l1', 't1', NULL, 0, 'MILK 1L', 10),
			('l2', 't2', NULL, 0, 'Milk  1l.', 10),
			('l3', NULL, 'r1', 0, 'milk 1l', 10),
			('l4', 't1', NULL, 0, 'Bread', 5),
			('l5', 't1', NULL, 1, '', 2)`,
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	if _, err := p.Up(ctx); err != nil {
		t.Fatal(err)
	}

	var items int
	if err := db.QueryRow("SELECT COUNT(*) FROM items").Scan(&items); err != nil {
		t.Fatal(err)
	}
	if items != 2 {
		t.Fatalf("items = %d, want 2", items)
	}
	var milk, name string
	if err := db.QueryRow("SELECT id, name FROM items WHERE name_key = 'milk 1l'").Scan(&milk, &name); err != nil {
		t.Fatal(err)
	}
	if name != "milk 1l" { // the recurrent tx, created last
		t.Errorf("name = %q, want the latest line's spelling", name)
	}
	for line, want := range map[string]sql.NullString{
		"l1": {String: milk, Valid: true}, "l2": {String: milk, Valid: true}, "l3": {String: milk, Valid: true},
		"l5": {},
	} {
		var got sql.NullString
		if err := db.QueryRow("SELECT item_id FROM transaction_items WHERE id = ?", line).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s item_id = %v, want %v", line, got, want)
		}
	}
	rows, err := db.Query("SELECT counterparty_id FROM counterparty_items WHERE item_id = ? ORDER BY 1", milk)
	if err != nil {
		t.Fatal(err)
	}
	var cps []string
	for rows.Next() {
		var cp string
		if err := rows.Scan(&cp); err != nil {
			t.Fatal(err)
		}
		cps = append(cps, cp)
	}
	rows.Close()
	if !slices.Equal(cps, []string{"mart", "shop"}) {
		t.Errorf("merchants = %v", cps)
	}
}

// TestItemsBackfillRemovable checks deleting the backfill's file is safe: a
// DB it ran on still migrates without it.
func TestItemsBackfillRemovable(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join("..", "..", migrationsDir)
	db, err := openDB(filepath.Join(t.TempDir(), "t.sqlite"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := migrateTenantDB(ctx, db, dir); err != nil {
		t.Fatal(err)
	}

	without := slices.DeleteFunc(slices.Clone(tenantGoMigrations), func(m *goose.Migration) bool {
		return m.Version == itemsBackfillVersion
	})
	p, err := goose.NewProvider(goose.DialectSQLite3, db, os.DirFS(dir),
		goose.WithGoMigrations(without...), goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("migrating without the backfill: %v", err)
	}
}

// TestItemCategoriesMigration moves the items' budget categories to item
// categories, keeping the lines' links through the items rebuild.
func TestItemCategoriesMigration(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join("..", "..", migrationsDir)
	db, err := openDB(filepath.Join(t.TempDir(), "t.sqlite"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	p, err := newMigrationProvider(db, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, itemsBackfillVersion); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"PRAGMA foreign_keys = OFF",
		`INSERT INTO categories (id, name, color, kinds) VALUES ('food', 'Food', 'green', '[1]'), ('fun', 'Fun', 'red', '[1]')`,
		"INSERT INTO items (id, name, name_key, category_id) VALUES ('milk', 'Milk', 'milk', 'food'), ('salt', 'Salt', 'salt', NULL)",
		"INSERT INTO transactions (id, account_id, amount, kind, scheduled_to) VALUES ('t1', 'acc', 10, 1, CURRENT_TIMESTAMP)",
		"INSERT INTO transaction_items (id, transaction_id, kind, name, total, item_id) VALUES ('l1', 't1', 0, 'Milk', 10, 'milk')",
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatal(err)
	}

	var name, color string
	if err := db.QueryRow("SELECT name, color FROM item_categories WHERE id = 'food'").Scan(&name, &color); err != nil {
		t.Fatal(err)
	}
	if name != "Food" || color != "green" {
		t.Errorf("item category = %q %q", name, color)
	}
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM item_categories").Scan(&n); err != nil || n != 1 {
		t.Errorf("item categories = %d (%v), only the ones items had", n, err)
	}
	var cat sql.NullString
	if err := db.QueryRow("SELECT category_id FROM items WHERE id = 'milk'").Scan(&cat); err != nil || cat.String != "food" {
		t.Errorf("milk category = %v (%v)", cat, err)
	}
	var item sql.NullString
	if err := db.QueryRow("SELECT item_id FROM transaction_items WHERE id = 'l1'").Scan(&item); err != nil || item.String != "milk" {
		t.Errorf("line item = %v (%v)", item, err)
	}
	// Deleting the item category clears the item's, not the budget one's.
	if _, err := db.Exec("DELETE FROM item_categories WHERE id = 'food'"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT category_id FROM items WHERE id = 'milk'").Scan(&cat); err != nil || cat.Valid {
		t.Errorf("milk category after deleting it = %v (%v)", cat, err)
	}
	for _, table := range []string{"items", "transaction_items", "counterparty_items"} {
		if err := db.QueryRow("PRAGMA foreign_key_check(" + table + ")").Scan(new(any), new(any), new(any), new(any)); err != sql.ErrNoRows {
			t.Errorf("foreign key check of %s: %v", table, err)
		}
	}
}
