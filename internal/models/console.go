package models

import (
	"strings"
	"time"
)

// The SQL console (/console) types: the user's database as is, read by
// repository's Console* methods.

const (
	// ConsoleMaxRows is the most rows a console query reads.
	ConsoleMaxRows = 500
	// ConsolePageSize is how many rows browsing a table shows a page.
	ConsolePageSize = 100
	// ConsoleMaxSQL is the longest SQL the console runs.
	ConsoleMaxSQL = 10_000
)

// QuoteIdent quotes a name for SQL.
func QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// ConsoleValue is one cell of a console result.
type ConsoleValue struct {
	Null bool
	// Text is the value as shown, cut at consoleMaxText characters.
	Text string
	// Raw is the value as a query argument would match it (e.g. to follow a
	// foreign key), empty for NULLs and blobs.
	Raw string
	// Blob is the size of a blob value, -1 when it's not one.
	Blob int
	Cut  bool
	// Number is whether it's an integer or a real.
	Number bool
}

// ConsoleResult is the rows a console query read.
type ConsoleResult struct {
	Columns []string
	Rows    [][]ConsoleValue
	// Truncated is whether there were more rows than read.
	Truncated bool
	Took      time.Duration
}

// ConsoleTable is a table or view of the user's database.
type ConsoleTable struct {
	Name string
	// Type is "table" or "view".
	Type string
	// Rows is its row count, -1 when it couldn't be counted.
	Rows int64
	// Internal is whether it's SQLite's or the migrations' own.
	Internal bool
}

// ConsoleColumn is a column of a table.
type ConsoleColumn struct {
	Name     string
	Type     string
	NotNull  bool
	Default  string
	PK       int
	Hidden   bool
	Position int
}

// ConsoleFK is a foreign key from Table.From to RefTable.To.
type ConsoleFK struct {
	Table    string
	From     string
	RefTable string
	To       string
}

// ConsoleIndex is an index of a table.
type ConsoleIndex struct {
	Name    string
	Unique  bool
	Columns []string
	SQL     string
}

// ConsoleTableInfo is the schema of a table: its columns, foreign keys both
// ways and indexes.
type ConsoleTableInfo struct {
	ConsoleTable
	Columns []ConsoleColumn
	// FKs are its foreign keys, RefBy the other tables' pointing at it.
	FKs     []ConsoleFK
	RefBy   []ConsoleFK
	Indexes []ConsoleIndex
	SQL     string
}

// FK is the foreign key of the column, nil when it has none.
func (t ConsoleTableInfo) FK(column string) *ConsoleFK {
	for i := range t.FKs {
		if t.FKs[i].From == column {
			return &t.FKs[i]
		}
	}
	return nil
}

// HasColumn reports whether the table has the column.
func (t ConsoleTableInfo) HasColumn(name string) bool {
	for _, c := range t.Columns {
		if c.Name == name {
			return true
		}
	}
	return false
}

// ConsoleBrowse is a page of a table's rows: sorted by a column, filtered on
// one equal to a value (following a foreign key).
type ConsoleBrowse struct {
	Sort string
	Desc bool
	// Col and Val filter the rows on Col = Val when Col is set.
	Col string
	Val string
	// Page is 0-based.
	Page int
}

// SQL is the query reading the page of table, with its arguments.
func (b ConsoleBrowse) SQL(table string) (string, []any) {
	q := "SELECT * FROM " + QuoteIdent(table)
	var args []any
	if b.Col != "" {
		q += " WHERE " + QuoteIdent(b.Col) + " = ?"
		args = append(args, b.Val)
	}
	if b.Sort != "" {
		q += " ORDER BY " + QuoteIdent(b.Sort)
		if b.Desc {
			q += " DESC"
		}
	}
	return q, args
}

// Inline is SQL with its value written in, to run in the console's editor.
func (b ConsoleBrowse) Inline(table string) string {
	q, args := b.SQL(table)
	if len(args) > 0 {
		q = strings.Replace(q, "?", "'"+strings.ReplaceAll(b.Val, "'", "''")+"'", 1)
	}
	return q + ";"
}

// ConsoleRanks places the tables of a schema diagram in columns, left to
// right: a table one right of the furthest table it references (itself
// aside), the ones referencing nothing first. Cycles stop growing after as
// many rounds as there are tables.
func ConsoleRanks(tables []ConsoleTableInfo) map[string]int {
	rank := make(map[string]int, len(tables))
	known := make(map[string]bool, len(tables))
	for _, t := range tables {
		known[t.Name] = true
	}
	for range tables {
		changed := false
		for _, t := range tables {
			for _, fk := range t.FKs {
				if fk.RefTable == t.Name || !known[fk.RefTable] {
					continue
				}
				if r := rank[fk.RefTable] + 1; r > rank[t.Name] && r < len(tables) {
					rank[t.Name] = r
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}
	return rank
}

// IsKey reports whether the column is in the table's primary key or a
// foreign key.
func (t ConsoleTableInfo) IsKey(c ConsoleColumn) bool {
	return c.PK > 0 || t.FK(c.Name) != nil
}
