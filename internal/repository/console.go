package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mattn/go-sqlite3"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"go.uber.org/zap"
)

// The SQL console (/console) reads the user's own database as is: its tables
// and the rows of their SELECTs. Every console query runs on the tenant's
// connection with an authorizer allowing reads only (consoleAuthorizer), so no
// write, ATTACH (another tenant's file), transaction or key pragma gets
// through whatever the SQL says, within consoleTimeout and with strings and
// blobs capped at consoleMaxLength.
const (
	consoleTimeout   = 5 * time.Second
	consoleMaxLength = 32 << 20
	// consoleMaxText is how much of a text value a cell keeps.
	consoleMaxText = 2000
)

// sqliteRecursive is SQLITE_RECURSIVE, which the driver doesn't export.
const sqliteRecursive = 33

// ErrConsoleDenied is a console query trying more than reading.
var ErrConsoleDenied = errors.New("the console only reads: this query isn't allowed")

// consolePragmas are the pragmas the console may run, all read only.
var consolePragmas = map[string]bool{
	"table_info":       true,
	"table_xinfo":      true,
	"table_list":       true,
	"index_list":       true,
	"index_info":       true,
	"index_xinfo":      true,
	"foreign_key_list": true,
}

// consoleAuthorizer allows reading: SELECTs, their tables and functions, and
// the schema's pragmas. Anything else is denied, so the statement fails to
// prepare.
func consoleAuthorizer(op int, arg1, arg2, _ string) int {
	switch op {
	case sqlite3.SQLITE_SELECT, sqlite3.SQLITE_READ, sqliteRecursive:
		return sqlite3.SQLITE_OK
	case sqlite3.SQLITE_UPDATE:
		// Asked when a pragma's table-valued function (pragma_table_info)
		// reads the schema. Writing it needs writable_schema (denied) and
		// query_only stops it anyway.
		if arg1 == "sqlite_master" {
			return sqlite3.SQLITE_OK
		}
	case sqlite3.SQLITE_FUNCTION:
		// arg2 is the function's name.
		name := strings.ToLower(arg2)
		if name == "load_extension" || strings.HasPrefix(name, "sqlcipher") {
			return sqlite3.SQLITE_DENY
		}
		return sqlite3.SQLITE_OK
	case sqlite3.SQLITE_PRAGMA:
		if consolePragmas[strings.ToLower(arg1)] {
			return sqlite3.SQLITE_OK
		}
	}
	return sqlite3.SQLITE_DENY
}

// consoleQuery runs query on db with the console's authorizer and limits,
// reading at most maxRows rows.
func consoleQuery(ctx context.Context, db *sql.DB, maxRows int, query string, args ...any) (models.ConsoleResult, error) {
	ctx, cancel := context.WithTimeout(ctx, consoleTimeout)
	defer cancel()

	conn, err := db.Conn(ctx)
	if err != nil {
		return models.ConsoleResult{}, consoleErr(ctx, err)
	}
	defer conn.Close()

	var oldLength int
	if err := conn.Raw(func(dc any) error {
		c, ok := dc.(*sqlite3.SQLiteConn)
		if !ok {
			return fmt.Errorf("unexpected driver connection %T", dc)
		}
		if _, err := c.Exec("PRAGMA query_only = ON", nil); err != nil {
			return err
		}
		// Each registration keeps a small handle until the connection closes:
		// fine for a console's queries.
		c.RegisterAuthorizer(consoleAuthorizer)
		oldLength = c.SetLimit(sqlite3.SQLITE_LIMIT_LENGTH, consoleMaxLength)
		return nil
	}); err != nil {
		return models.ConsoleResult{}, err
	}
	defer func() {
		err := conn.Raw(func(dc any) error {
			c := dc.(*sqlite3.SQLiteConn)
			c.RegisterAuthorizer(nil)
			c.SetLimit(sqlite3.SQLITE_LIMIT_LENGTH, oldLength)
			_, err := c.Exec("PRAGMA query_only = OFF", nil)
			return err
		})
		if err != nil {
			logger.FromCtx(ctx).Error("restoring the connection after a console query", zap.Error(err))
		}
	}()

	start := time.Now()
	rows, err := conn.QueryContext(ctx, query, args...)
	if err != nil {
		return models.ConsoleResult{}, consoleErr(ctx, err)
	}
	defer rows.Close()

	res := models.ConsoleResult{}
	if res.Columns, err = rows.Columns(); err != nil {
		return models.ConsoleResult{}, consoleErr(ctx, err)
	}
	for rows.Next() {
		if len(res.Rows) == maxRows {
			res.Truncated = true
			break
		}
		vals := make([]any, len(res.Columns))
		ptrs := make([]any, len(vals))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return models.ConsoleResult{}, consoleErr(ctx, err)
		}
		row := make([]models.ConsoleValue, len(vals))
		for i, v := range vals {
			row[i] = consoleValue(v)
		}
		res.Rows = append(res.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return models.ConsoleResult{}, consoleErr(ctx, err)
	}
	res.Took = time.Since(start)
	return res, nil
}

// consoleErr words the errors of a console query for the user.
func consoleErr(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("the query took over %s and was stopped", consoleTimeout)
	}
	var se sqlite3.Error
	if errors.As(err, &se) && se.Code == sqlite3.ErrAuth {
		return ErrConsoleDenied
	}
	return err
}

func consoleValue(v any) models.ConsoleValue {
	val := models.ConsoleValue{Blob: -1}
	switch v := v.(type) {
	case nil:
		val.Null = true
		val.Text = "NULL"
		return val
	case []byte:
		val.Blob = len(v)
		val.Text = consoleBlobText(len(v))
		return val
	case int64:
		val.Raw, val.Number = strconv.FormatInt(v, 10), true
	case float64:
		val.Raw, val.Number = strconv.FormatFloat(v, 'g', -1, 64), true
	case bool:
		val.Raw, val.Number = "0", true
		if v {
			val.Raw = "1"
		}
	case time.Time:
		// The driver parses the columns declared as a time, write them back
		// as it stores them.
		val.Raw = v.Format(sqlite3.SQLiteTimestampFormats[0])
	case string:
		val.Raw = v
	default:
		val.Raw = fmt.Sprint(v)
	}
	val.Text = val.Raw
	if utf8.RuneCountInString(val.Text) > consoleMaxText {
		val.Text = string([]rune(val.Text)[:consoleMaxText])
		val.Cut = true
	}
	return val
}

func consoleBlobText(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("blob · %.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("blob · %.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("blob · %d B", n)
}

// ConsoleTables lists the tables and views of the user's database, the
// internal ones (sqlite_*, goose's) last.
func (r *Repository) ConsoleTables(ctx context.Context) ([]models.ConsoleTable, error) {
	db, _, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	return consoleTables(ctx, db)
}

func consoleTables(ctx context.Context, db *sql.DB) ([]models.ConsoleTable, error) {
	rows, err := db.QueryContext(ctx, `SELECT name, type FROM sqlite_schema WHERE type IN ('table', 'view')`)
	if err != nil {
		return nil, err
	}
	var tables []models.ConsoleTable
	for rows.Next() {
		var t models.ConsoleTable
		if err := rows.Scan(&t.Name, &t.Type); err != nil {
			rows.Close()
			return nil, err
		}
		t.Internal = strings.HasPrefix(t.Name, "sqlite_") || t.Name == "goose_db_version"
		tables = append(tables, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	tables = append(tables, models.ConsoleTable{Name: "sqlite_schema", Type: "table", Internal: true})

	for i := range tables {
		tables[i].Rows = -1
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+models.QuoteIdent(tables[i].Name)).Scan(&tables[i].Rows); err != nil {
			tables[i].Rows = -1
		}
	}
	sort.SliceStable(tables, func(i, j int) bool {
		if tables[i].Internal != tables[j].Internal {
			return !tables[i].Internal
		}
		return tables[i].Name < tables[j].Name
	})
	return tables, nil
}

// ErrConsoleNoTable is a console table that doesn't exist.
var ErrConsoleNoTable = errors.New("no such table")

// ConsoleTableInfo is the schema of the table named name, ErrConsoleNoTable
// when it's none of tables.
func (r *Repository) ConsoleTableInfo(ctx context.Context, tables []models.ConsoleTable, name string) (models.ConsoleTableInfo, error) {
	db, _, err := r.dbFromCtx(ctx)
	if err != nil {
		return models.ConsoleTableInfo{}, err
	}
	return consoleTableInfo(ctx, db, tables, name)
}

func consoleTableInfo(ctx context.Context, db *sql.DB, tables []models.ConsoleTable, name string) (models.ConsoleTableInfo, error) {
	info := models.ConsoleTableInfo{}
	found := false
	for _, t := range tables {
		if t.Name == name {
			info.ConsoleTable, found = t, true
			break
		}
	}
	if !found {
		return info, ErrConsoleNoTable
	}

	if name != "sqlite_schema" {
		if err := db.QueryRowContext(ctx, "SELECT COALESCE(sql, '') FROM sqlite_schema WHERE name = ?", name).Scan(&info.SQL); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return info, err
		}
	}

	rows, err := db.QueryContext(ctx, "SELECT cid, name, type, \"notnull\", COALESCE(dflt_value, ''), pk, hidden FROM pragma_table_xinfo(?)", name)
	if err != nil {
		return info, err
	}
	for rows.Next() {
		var c models.ConsoleColumn
		var hidden int
		if err := rows.Scan(&c.Position, &c.Name, &c.Type, &c.NotNull, &c.Default, &c.PK, &hidden); err != nil {
			rows.Close()
			return info, err
		}
		c.Hidden = hidden != 0
		info.Columns = append(info.Columns, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return info, err
	}

	for _, t := range tables {
		if t.Type != "table" {
			continue
		}
		fks, err := consoleFKs(ctx, db, t.Name)
		if err != nil {
			return info, err
		}
		for _, fk := range fks {
			if t.Name == name {
				info.FKs = append(info.FKs, fk)
			}
			if fk.RefTable == name {
				info.RefBy = append(info.RefBy, fk)
			}
		}
	}

	if info.Type == "table" {
		if info.Indexes, err = consoleIndexes(ctx, db, name); err != nil {
			return info, err
		}
	}
	return info, nil
}

func consoleFKs(ctx context.Context, db *sql.DB, table string) ([]models.ConsoleFK, error) {
	rows, err := db.QueryContext(ctx, `SELECT "table", "from", COALESCE("to", '') FROM pragma_foreign_key_list(?) ORDER BY id, seq`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var fks []models.ConsoleFK
	for rows.Next() {
		fk := models.ConsoleFK{Table: table}
		if err := rows.Scan(&fk.RefTable, &fk.From, &fk.To); err != nil {
			return nil, err
		}
		if fk.To == "" {
			// A key to the referenced table's primary key.
			fk.To = "rowid"
			pkRow := db.QueryRowContext(ctx, "SELECT name FROM pragma_table_info(?) WHERE pk = 1", fk.RefTable)
			_ = pkRow.Scan(&fk.To)
		}
		fks = append(fks, fk)
	}
	return fks, rows.Err()
}

func consoleIndexes(ctx context.Context, db *sql.DB, table string) ([]models.ConsoleIndex, error) {
	rows, err := db.QueryContext(ctx, `SELECT il.name, il."unique", COALESCE(s.sql, '') FROM pragma_index_list(?) AS il LEFT JOIN sqlite_schema AS s ON s.name = il.name ORDER BY il.name`, table)
	if err != nil {
		return nil, err
	}
	var idx []models.ConsoleIndex
	for rows.Next() {
		var i models.ConsoleIndex
		if err := rows.Scan(&i.Name, &i.Unique, &i.SQL); err != nil {
			rows.Close()
			return nil, err
		}
		idx = append(idx, i)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for k := range idx {
		cols, err := db.QueryContext(ctx, "SELECT COALESCE(name, '<expr>') FROM pragma_index_info(?) ORDER BY seqno", idx[k].Name)
		if err != nil {
			return nil, err
		}
		for cols.Next() {
			var c string
			if err := cols.Scan(&c); err != nil {
				cols.Close()
				return nil, err
			}
			idx[k].Columns = append(idx[k].Columns, c)
		}
		cols.Close()
	}
	return idx, nil
}

// ConsoleBrowseTable reads a page of the table's rows (Total: how many there
// are), the browse's columns checked against info's.
func (r *Repository) ConsoleBrowseTable(ctx context.Context, info models.ConsoleTableInfo, b models.ConsoleBrowse) (models.ConsoleResult, int64, error) {
	db, _, err := r.dbFromCtx(ctx)
	if err != nil {
		return models.ConsoleResult{}, 0, err
	}
	return consoleBrowseTable(ctx, db, info, b)
}

func consoleBrowseTable(ctx context.Context, db *sql.DB, info models.ConsoleTableInfo, b models.ConsoleBrowse) (models.ConsoleResult, int64, error) {
	if (b.Sort != "" && !info.HasColumn(b.Sort)) || (b.Col != "" && !info.HasColumn(b.Col) && b.Col != "rowid") {
		return models.ConsoleResult{}, 0, ErrConsoleNoTable
	}
	q, args := b.SQL(info.Name)

	var total int64
	count := "SELECT COUNT(*) FROM (" + q + ")"
	countRes, err := consoleQuery(ctx, db, 1, count, args...)
	if err != nil {
		return models.ConsoleResult{}, 0, err
	}
	if len(countRes.Rows) == 1 {
		total, _ = strconv.ParseInt(countRes.Rows[0][0].Raw, 10, 64)
	}

	q += fmt.Sprintf(" LIMIT %d OFFSET %d", models.ConsolePageSize, max(b.Page, 0)*models.ConsolePageSize)
	res, err := consoleQuery(ctx, db, models.ConsolePageSize, q, args...)
	return res, total, err
}

// ConsoleRun runs the user's SQL, reading its rows (the last statement's
// when there are many).
func (r *Repository) ConsoleRun(ctx context.Context, query string) (models.ConsoleResult, error) {
	db, _, err := r.dbFromCtx(ctx)
	if err != nil {
		return models.ConsoleResult{}, err
	}
	res, err := consoleQuery(ctx, db, models.ConsoleMaxRows, query)
	l := logger.FromCtx(ctx)
	if err != nil {
		l.Info("console query failed", zap.Bool("denied", errors.Is(err, ErrConsoleDenied)))
	} else {
		l.Info("console query", zap.Int("rows", len(res.Rows)), zap.Duration("took", res.Took))
	}
	return res, err
}
