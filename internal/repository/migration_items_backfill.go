package repository

// One-off: links the receipt lines written before the items catalog
// (20261122000000_items.sql) to items. Delete this file once every tenant DB
// has been opened since it shipped (see CLAUDE.md, Items catalog): goose
// leaves its recorded version alone, and a DB it never ran on only keeps its
// old lines unlinked (saving one of their transactions links them).

import (
	"context"
	"database/sql"
	"slices"
	"strings"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
)

const itemsBackfillVersion = 20261122000001

func init() {
	tenantGoMigrations = append(tenantGoMigrations,
		goose.NewGoMigration(itemsBackfillVersion, &goose.GoFunc{RunTx: backfillItems}, nil))
}

// backfillItems groups the item lines without an item by their name's
// models.ItemKey into new items (named after the latest line's spelling) and
// links the merchants they were bought at. Raw SQL only, so later query
// changes never touch it.
func backfillItems(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT ti.id, ti.name, COALESCE(t.counterparty_id, r.counterparty_id, '')
		FROM transaction_items AS ti
		LEFT JOIN transactions AS t ON t.id = ti.transaction_id
		LEFT JOIN recurrent_txs AS r ON r.id = ti.recurrent_id
		WHERE ti.kind = 0 AND ti.item_id IS NULL
		ORDER BY COALESCE(t.scheduled_to, r.created_at, ti.created_at), ti.created_at`)
	if err != nil {
		return err
	}

	type group struct {
		name  string // the latest line's spelling
		lines []string
		cps   []string
	}
	groups := map[string]*group{}
	var keys []string
	for rows.Next() {
		var id, name, cp string
		if err := rows.Scan(&id, &name, &cp); err != nil {
			rows.Close()
			return err
		}
		key := models.ItemKey(name)
		if key == "" {
			continue
		}
		g := groups[key]
		if g == nil {
			g = &group{}
			groups[key] = g
			keys = append(keys, key)
		}
		g.name = strings.TrimSpace(name)
		g.lines = append(g.lines, id)
		if cp != "" && !slices.Contains(g.cps, cp) {
			g.cps = append(g.cps, cp)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	linked := 0
	for _, key := range keys {
		g := groups[key]
		id := idgen.New()
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO items (id, name, name_key) VALUES (?, ?, ?)",
			id, g.name, key); err != nil {
			return err
		}
		for chunk := range slices.Chunk(g.lines, 500) {
			args := make([]any, 0, len(chunk)+1)
			args = append(args, id)
			for _, l := range chunk {
				args = append(args, l)
			}
			q := "UPDATE transaction_items SET item_id = ? WHERE id IN (?" + strings.Repeat(", ?", len(chunk)-1) + ")"
			if _, err := tx.ExecContext(ctx, q, args...); err != nil {
				return err
			}
		}
		for _, cp := range g.cps {
			if _, err := tx.ExecContext(ctx,
				"INSERT OR IGNORE INTO counterparty_items (counterparty_id, item_id) VALUES (?, ?)",
				cp, id); err != nil {
				return err
			}
		}
		linked += len(g.lines)
	}
	if len(keys) > 0 {
		logger.FromCtx(ctx).Info("backfilled items", zap.Int("items", len(keys)), zap.Int("lines", linked))
	}
	return nil
}
