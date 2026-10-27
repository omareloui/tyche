package dbtypes_test

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/omareloui/tyche/internal/dbtypes"
)

func TestJSONScansTextAndBlob(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE t (id INTEGER, v JSON NOT NULL DEFAULT '[]')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO t (id) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO t (id, v) VALUES (2, ?)`, []byte(`["a"]`)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO t (id, v) VALUES (3, ?)`, dbtypes.JSON(`["b"]`)); err != nil {
		t.Fatal(err)
	}

	want := map[int]string{1: `[]`, 2: `["a"]`, 3: `["b"]`}
	rows, err := db.Query(`SELECT id, v FROM t`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var v dbtypes.JSON
		if err := rows.Scan(&id, &v); err != nil {
			t.Fatalf("row %d: %v", id, err)
		}
		if string(v) != want[id] {
			t.Errorf("row %d = %q, want %q", id, v, want[id])
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
