package sqlguess

import (
	"strings"
	"testing"
	"time"

	"github.com/omareloui/tyche/internal/models"
)

func TestClean(t *testing.T) {
	o := Clean(Out{SQL: "```sql\nSELECT 1;\n```", Explain: " counts\n  things ", Chart: "BAR"})
	if o.SQL != "SELECT 1;" || o.Explain != "counts things" || o.Chart != ChartBar {
		t.Errorf("got %+v", o)
	}
	if o := Clean(Out{SQL: " SELECT 2 ", Chart: "pie"}); o.SQL != "SELECT 2" || o.Chart != ChartTable {
		t.Errorf("got %+v", o)
	}
	if o := Clean(Out{Explain: strings.Repeat("a", MaxExplainLen+10)}); len([]rune(o.Explain)) != MaxExplainLen {
		t.Errorf("explain of %d runes", len([]rune(o.Explain)))
	}
}

func TestTooLong(t *testing.T) {
	if TooLong(strings.Repeat("a ", MaxQuestionLen/2)) {
		t.Error("a question at the limit is too long")
	}
	if !TooLong(strings.Repeat("a", MaxQuestionLen+1)) {
		t.Error("a question over the limit isn't too long")
	}
}

func TestCondense(t *testing.T) {
	tables := []models.ConsoleTableInfo{
		{
			ConsoleTable: models.ConsoleTable{Name: "transactions", Type: "table"},
			Columns: []models.ConsoleColumn{
				{Name: "id", Type: "VARCHAR", PK: 1},
				{Name: "account_id", Type: "VARCHAR"},
				{Name: "amount", Type: "FLOAT"},
				{Name: "scheduled_to", Type: "DATE"},
			},
			FKs: []models.ConsoleFK{{Table: "transactions", From: "account_id", RefTable: "accounts", To: "id"}},
		},
		{ConsoleTable: models.ConsoleTable{Name: "receipts", Type: "table"}, Columns: []models.ConsoleColumn{{Name: "id"}}},
		{ConsoleTable: models.ConsoleTable{Name: "goose_db_version", Type: "table", Internal: true}, Columns: []models.ConsoleColumn{{Name: "id"}}},
		{
			ConsoleTable: models.ConsoleTable{Name: "user", Type: "table"},
			Columns:      []models.ConsoleColumn{{Name: "email", Type: "TEXT"}, {Name: "start_of_period", Type: "INT"}, {Name: "blob", Type: "BLOB"}},
		},
	}
	want := "transactions(id text PK, account_id text → accounts, amount real, scheduled_to date)\nuser(start_of_period int)\n"
	if got := Condense(tables); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestPrompt(t *testing.T) {
	today := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	in := In{Question: "biggest grocery month?", Schema: "transactions(id text PK)\n", Today: today, Month: models.MonthStart(25).Of(today), DefaultCurrency: "EGP"}
	p := Prompt(in)
	for _, s := range []string{
		"Today: 2026-11-27",
		"This month (December 2026): 2026-11-25 to 2026-12-24",
		"Last month (November 2026): 2026-10-25 to 2026-11-24",
		"start on day 25",
		"main currency: EGP",
		"transactions(id text PK)",
		"biggest grocery month?",
	} {
		if !strings.Contains(p, s) {
			t.Errorf("prompt lacks %q", s)
		}
	}
	if strings.Contains(p, "previous query failed") {
		t.Error("a first try tells of a failure")
	}
	in.Retry = &Retry{SQL: "SELECT x", Error: "no such column: x"}
	if p := Prompt(in); !strings.Contains(p, "SELECT x\nError:\nno such column: x") {
		t.Error("a retry doesn't tell the failed query and its error")
	}
}
