package repository

import (
	"cmp"
	"context"
	"database/sql"
	"math"
	"slices"

	"go.uber.org/zap"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
)

// importMergeCandidatesMax is how many transactions the merge picker offers.
const importMergeCandidatesMax = 12

// GetImportMergeCandidates are the fulfilled transactions of the imported
// one's account, within models.ImportMergeWindow, it can be merged into: the
// likely ones first, then the closest in amount and day.
func (r *Repository) GetImportMergeCandidates(ctx context.Context, imported *models.Transaction) ([]models.Transaction, error) {
	rows, err := r.GetCSVDupCandidates(ctx, imported.Account.ID,
		imported.ScheduledTo.Add(-models.ImportMergeWindow), imported.ScheduledTo.Add(models.ImportMergeWindow))
	if err != nil {
		return nil, err
	}
	var res []models.Transaction
	for _, row := range rows {
		if row.ID == imported.ID || row.Out == imported.Kind.IsIncoming() {
			continue
		}
		tx, err := r.GetTransaction(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		if models.ValidateImportMerge(imported, tx) == nil {
			res = append(res, *tx)
		}
	}
	score := func(t *models.Transaction) float64 {
		off := math.Abs(math.Abs(t.Amount)-math.Abs(imported.Amount)) / max(math.Abs(imported.Amount), 1)
		return off + math.Abs(t.ScheduledTo.Sub(imported.ScheduledTo).Hours())/24/100
	}
	slices.SortStableFunc(res, func(a, b models.Transaction) int {
		la, lb := models.ImportMergeLikely(imported, &a), models.ImportMergeLikely(imported, &b)
		if la != lb {
			if la {
				return -1
			}
			return 1
		}
		return cmp.Compare(score(&a), score(&b))
	})
	if len(res) > importMergeCandidatesMax {
		res = res[:importMergeCandidatesMax]
	}
	return res, nil
}

// MergeImportedInto merges the imported transaction into target (see
// models.MergeImported) and deletes it: target takes its amount, fee, what
// it was paid in and its import (so the row reads as imported before).
func (r *Repository) MergeImportedInto(ctx context.Context, imported, target *models.Transaction) error {
	if err := models.ValidateImportMerge(imported, target); err != nil {
		return err
	}
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if err := r.deleteTransactionWithTxn(ctx, qtx, imported); err != nil {
			return err
		}
		return r.applyImportedWithTxn(ctx, qtx, target, imported, imported.ImportID, imported.ImportHash)
	})
}

// applyImportedWithTxn saves target with what the imported row (a saved
// transaction or one built from a row) knows best, linked to its import.
func (r *Repository) applyImportedWithTxn(ctx context.Context, qtx *queries.Queries, target, imported *models.Transaction, importID, hash string) error {
	merged := models.MergeImported(imported, target)
	if err := r.updateTransactionWithTxn(ctx, qtx, target, merged); err != nil {
		return err
	}
	if err := qtx.SetTransactionImport(ctx, queries.SetTransactionImportParams{
		ID:         target.ID,
		ImportID:   sql.NullString{String: importID, Valid: importID != ""},
		ImportHash: sql.NullString{String: hash, Valid: hash != ""},
	}); err != nil {
		logger.FromCtx(ctx).Error("linking a merged transaction to its import", zap.Error(err), zap.String("id", target.ID))
		return err
	}
	logger.FromCtx(ctx).Info("merged an imported row into a transaction", zap.String("id", target.ID))
	return nil
}
