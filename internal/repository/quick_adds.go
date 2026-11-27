package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// QuickAdded is a transaction a quick add saved: ID the one it is, Merged
// when it paid a recurrent occurrence (ID then the occurrence's) instead of
// being created.
type QuickAdded struct {
	ID     string
	Merged bool
}

// QuickAdd saves the transactions read from text in one database
// transaction, each like an SMS's (paying the recurrent occurrence it likely
// is, auto tagged, paired with the other leg of a transfer), linked to a
// quick_adds row keeping the text and the model that read it ('' for none).
func (r *Repository) QuickAdd(ctx context.Context, text, model string, txs []*models.Transaction) ([]QuickAdded, error) {
	l := logger.FromCtx(ctx)
	var out []QuickAdded
	err := r.transact(ctx, func(qtx *queries.Queries) error {
		out = nil
		id := idgen.New()
		if err := qtx.CreateQuickAdd(ctx, queries.CreateQuickAddParams{ID: id, Text: text, Model: model}); err != nil {
			l.Error("creating a quick add", zap.Error(err))
			return err
		}
		for _, tx := range txs {
			created, tags := tx.ID, tx.Tags
			if err := r.createIngestedTransactionWithTxn(ctx, qtx, tx); err != nil {
				return err
			}
			// paying an occurrence keeps its fields: the #tags are added
			if tx.ID != created {
				for _, t := range tags {
					tag, err := findOrCreateTagWithTxn(ctx, qtx, t.Name)
					if err != nil {
						return err
					}
					if err := qtx.AddTransactionTag(ctx, queries.AddTransactionTagParams{TransactionID: tx.ID, TagID: tag.ID}); err != nil {
						return err
					}
				}
			}
			if err := qtx.SetTransactionQuickAdd(ctx, queries.SetTransactionQuickAddParams{
				ID: tx.ID, QuickAddID: sql.NullString{String: id, Valid: true},
			}); err != nil {
				l.Error("linking a transaction to its quick add", zap.Error(err), zap.String("id", tx.ID))
				return err
			}
			tx.QuickAddID = id
			out = append(out, QuickAdded{ID: tx.ID, Merged: tx.ID != created})
		}
		l.Info("quick added", zap.String("id", id), zap.Int("count", len(txs)))
		return nil
	})
	return out, err
}

// GetTxQuickAddText is the text a transaction was quick added from, "" when
// it wasn't.
func (r *Repository) GetTxQuickAddText(ctx context.Context, txID string) (string, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return "", err
	}
	text, err := q.GetTransactionQuickAddText(ctx, txID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return text, err
}

// GetAIQuickAdd is whether the user reads quick add's texts with AI.
func (r *Repository) GetAIQuickAdd(ctx context.Context) (bool, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return false, err
	}
	return q.GetAIQuickAdd(ctx)
}

// SetAIQuickAdd turns reading quick add's texts with AI on or off.
func (r *Repository) SetAIQuickAdd(ctx context.Context, on bool) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	return q.SetAIQuickAdd(ctx, on)
}

// GetCounterpartyUses is how many transactions each counterparty is in (by
// name or its debt account), by id.
func (r *Repository) GetCounterpartyUses(ctx context.Context) (map[string]int, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.GetCounterpartyUses(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.ID] = int(row.Uses)
	}
	return out, nil
}
