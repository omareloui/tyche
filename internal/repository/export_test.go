package repository

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/omareloui/tyche/internal/vault"
)

// TestExportToCompacts checks an export (plain and encrypted) reads back
// with every row and no free pages left by sqlcipher_export.
func TestExportToCompacts(t *testing.T) {
	dir := t.TempDir()
	src, err := openDB(filepath.Join(dir, "src.sqlite"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	if _, err := src.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, body TEXT); CREATE INDEX t_body ON t (body)"); err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("x", 500)
	for i := range 2000 {
		if _, err := src.Exec("INSERT INTO t (id, body) VALUES (?, ?)", i, body); err != nil {
			t.Fatal(err)
		}
	}

	key, err := vault.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	for name, k := range map[string][]byte{"plain": nil, "encrypted": key} {
		t.Run(name, func(t *testing.T) {
			dst := filepath.Join(dir, name+".sqlite")
			if err := exportTo(context.Background(), src, dst, k); err != nil {
				t.Fatal(err)
			}
			db, err := openDB(dst, k)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			var rows, free int
			if err := db.QueryRow("SELECT COUNT(*) FROM t").Scan(&rows); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow("PRAGMA freelist_count").Scan(&free); err != nil {
				t.Fatal(err)
			}
			if rows != 2000 || free != 0 {
				t.Errorf("rows = %d, free pages = %d, want 2000 and 0", rows, free)
			}
		})
	}
}
