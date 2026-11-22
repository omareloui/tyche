package shareddb

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/omareloui/tyche/internal/sqlite"
	"github.com/pressly/goose/v3"
)

// The receipts' limits set by an admin become the receipt kind's AI limits.
func TestAILimitsMigration(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "shared.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	dir := "../../sql/shared/schema"
	if err := goose.UpTo(db, dir, 20261128000000); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO receipt_limits (tenant_id, monthly_limit) VALUES ('GL_1', 7)`); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(db, dir); err != nil {
		t.Fatal(err)
	}
	var kind string
	var limit int
	if err := db.QueryRow(`SELECT kind, monthly_limit FROM ai_limits WHERE tenant_id = 'GL_1'`).Scan(&kind, &limit); err != nil {
		t.Fatal(err)
	}
	if kind != "receipt" || limit != 7 {
		t.Errorf("limit = %s %d", kind, limit)
	}
	if _, err := db.Exec(`SELECT search_queries FROM receipt_scans`); err != nil {
		t.Error(err)
	}
}
