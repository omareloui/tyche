package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// TestRemovedGoMigration checks a DB that ran a Go migration since deleted
// (the items backfill, 20261122000001) still migrates.
func TestRemovedGoMigration(t *testing.T) {
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
	if _, err := p.UpTo(ctx, 20261122000000); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO goose_db_version (version_id, is_applied) VALUES (20261122000001, 1)"); err != nil {
		t.Fatal(err)
	}
	if err := migrateTenantDB(ctx, db, dir); err != nil {
		t.Fatalf("migrating past the removed backfill: %v", err)
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
	if _, err := p.UpTo(ctx, 20261122000000); err != nil {
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
