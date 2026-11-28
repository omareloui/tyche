package repository

import (
	"context"
	"slices"
	"testing"
	"time"
)

// TestPurgeKeepsLatestPDFRead checks the daily purge of pending imports
// keeps the latest read of each PDF (the cache reading it again reuses)
// unless another import holds its rows, and still drops pending CSVs.
func TestPurgeKeepsLatestPDFRead(t *testing.T) {
	ctx := context.Background()
	db, q := splitTestDB(t)
	old := "2026-01-01 10:00:00"
	for _, stmt := range []string{
		"PRAGMA foreign_keys = OFF",
		// a CSV never imported
		"INSERT INTO csv_imports (id, account_id, file_name, content, mapping, row_count, created_at) VALUES ('01A', 'acc', 'a.csv', x'00', '{}', 1, '" + old + "')",
		// a PDF read twice, never imported: the latest kept
		"INSERT INTO csv_imports (id, account_id, file_name, content, mapping, row_count, source_kind, source_hash, created_at) VALUES ('01B', 'acc', 'b.pdf', x'00', '{}', 1, 'pdf', 'hb', '" + old + "')",
		"INSERT INTO csv_imports (id, account_id, file_name, content, mapping, row_count, source_kind, source_hash, created_at) VALUES ('01C', 'acc', 'b.pdf', x'00', '{}', 1, 'pdf', 'hb', '" + old + "')",
		// a PDF imported, read again later: the imported one holds its rows
		"INSERT INTO csv_imports (id, account_id, file_name, content, mapping, row_count, source_kind, source_hash, status, created_at) VALUES ('01D', 'acc', 'd.pdf', x'00', '{}', 1, 'pdf', 'hd', 'imported', '" + old + "')",
		"INSERT INTO csv_imports (id, account_id, file_name, content, mapping, row_count, source_kind, source_hash, created_at) VALUES ('01E', 'acc', 'd.pdf', x'00', '{}', 1, 'pdf', 'hd', '" + old + "')",
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	purgePendingCSVImports(ctx, q, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))

	rows, err := db.Query("SELECT id FROM csv_imports ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if want := []string{"01C", "01D"}; !slices.Equal(ids, want) {
		t.Errorf("left %v, want %v", ids, want)
	}
}
