package repository

import (
	"context"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"go.uber.org/zap"
)

// HoldingAccountInfo is a holding account and whether the gains are shown
// for it (live: not archived nor deleted, and holding money rather than
// owing it: no card, debt or gam'yaa).
type HoldingAccountInfo struct {
	models.HoldingAccount
	Shown bool
	// GoalID is the goal it's an account of, "" for none.
	GoalID string
	// Excluded tells it's excluded from the balance.
	Excluded bool
}

// GetHoldingData is every account in another currency than def, and the
// fulfilled transactions moving them in order, to walk their cost
// (models.ComputeHoldingGains).
func (r *Repository) GetHoldingData(ctx context.Context, def *models.Currency) ([]HoldingAccountInfo, []models.HoldingMove, error) {
	l := logger.FromCtx(ctx)
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, nil, err
	}
	rows, err := q.GetHoldingAccounts(ctx, def.ID)
	if err != nil {
		l.Error("querying holding accounts", zap.Error(err))
		return nil, nil, err
	}
	accs := make([]HoldingAccountInfo, 0, len(rows))
	for _, row := range rows {
		kind := models.AccKind(row.Kind)
		accs = append(accs, HoldingAccountInfo{
			HoldingAccount: models.HoldingAccount{ID: row.ID, Code: row.Code, Balance: row.Amount, Opened: row.CreatedAt},
			Shown: !row.IsArchived && !row.IsDeleted &&
				kind != models.AccKindCreditCard && kind != models.AccKindDebt && kind != models.AccKindGamyaa,
			GoalID:   row.GoalID.String,
			Excluded: row.ExcludeFromBalance,
		})
	}
	if len(accs) == 0 {
		return nil, nil, nil
	}

	mrows, err := q.GetHoldingMoves(ctx, def.ID)
	if err != nil {
		l.Error("querying holding moves", zap.Error(err))
		return nil, nil, err
	}
	moves := make([]models.HoldingMove, 0, len(mrows))
	for _, row := range mrows {
		day, err := time.Parse(dayLayout, row.Day)
		if err != nil {
			continue
		}
		moves = append(moves, models.HoldingMove{
			Day:         day,
			Kind:        models.TxKind(row.Kind),
			AccountID:   row.AccountID,
			ToAccountID: row.ToAccountID.String,
			Amount:      row.Amount,
			ToAmount:    row.ToAmount.Float64,
			FromCode:    row.FromCode,
			ToCode:      row.ToCode,
			PairAmount:  row.PairAmount,
			PairCode:    row.PairCode,
		})
	}
	return accs, moves, nil
}
