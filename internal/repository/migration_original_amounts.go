package repository

// One-off: moves the "originally <CUR> <amount>" the SMS ingest used to put in
// the description of a converted transaction into its original_amount /
// original_currency (20261206000000_foreign_amounts.sql). Delete this file
// once every tenant DB has been opened since it shipped: a DB it never ran on
// only keeps the note in its descriptions.

import (
	"context"
	"database/sql"
	"regexp"
	"strconv"
	"strings"

	"github.com/pressly/goose/v3"
)

const originalAmountsVersion = 20261206000001

func init() {
	tenantGoMigrations = append(tenantGoMigrations,
		goose.NewGoMigration(originalAmountsVersion, &goose.GoFunc{RunTx: backfillOriginalAmounts}, nil))
}

// originallyRe is the note smsingest.Convert wrote (formatters.FormatAmount:
// "USD 1,234.5"), with the " · " joining it to the other notes.
var originallyRe = regexp.MustCompile(`(?:^| · )originally ([A-Za-z]{2,5}) (-?[\d,]+(?:\.\d+)?)`)

// parseOriginally finds the first "originally <CUR> <amount>" note of a
// description: the currency's code, the amount and the description without
// it; ok false when it has none.
func parseOriginally(desc string) (cur string, amount float64, rest string, ok bool) {
	m := originallyRe.FindStringSubmatchIndex(desc)
	if m == nil {
		return "", 0, desc, false
	}
	amount, err := strconv.ParseFloat(strings.ReplaceAll(desc[m[4]:m[5]], ",", ""), 64)
	if err != nil {
		return "", 0, desc, false
	}
	cur = strings.ToUpper(desc[m[2]:m[3]])
	rest = desc[:m[0]] + desc[m[1]:]
	if m[0] == 0 {
		rest = strings.TrimPrefix(rest, " · ")
	}
	return cur, amount, strings.TrimSpace(rest), true
}

// backfillOriginalAmounts parses the notes of the transactions without an
// original amount yet. Raw SQL only, so later query changes never touch it.
func backfillOriginalAmounts(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, description FROM transactions
		WHERE original_amount IS NULL AND INSTR(description, 'originally ') > 0`)
	if err != nil {
		return err
	}
	type found struct {
		id, cur, desc string
		amount        float64
	}
	var all []found
	for rows.Next() {
		var id, desc string
		if err := rows.Scan(&id, &desc); err != nil {
			rows.Close()
			return err
		}
		if cur, amount, rest, ok := parseOriginally(desc); ok {
			all = append(all, found{id, cur, rest, amount})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, f := range all {
		var desc sql.NullString
		if f.desc != "" {
			desc = sql.NullString{String: f.desc, Valid: true}
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE transactions SET original_amount = ?, original_currency = ?, description = ?
			WHERE id = ?`, f.amount, f.cur, desc, f.id); err != nil {
			return err
		}
	}
	return nil
}
