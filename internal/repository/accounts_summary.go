package repository

import (
	"context"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"go.uber.org/zap"
)

// GetAccountsSummary is the net worth of the accounts in the balance (and
// the ones with asks for), what they hold per kind and currency, its last
// months and the cards' and debts' state, in def.
func (r *Repository) GetAccountsSummary(ctx context.Context, def *models.Currency, with models.AccountsSummaryWith) (*models.AccountsSummary, error) {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	accs, err := r.GetAccounts(ctx)
	if err != nil {
		return nil, err
	}
	if err := r.AttachCardStatements(ctx, accs); err != nil {
		return nil, err
	}
	// The goals' accounts, hidden from GetAccounts.
	goals, err := r.GetGoals(ctx)
	if err != nil {
		return nil, err
	}
	for _, gl := range goals {
		if gl.Account != nil {
			accs = append(accs, *gl.Account)
		}
	}
	loc, err := r.GetTimezone(ctx)
	if err != nil {
		return nil, err
	}
	today := time.Now().In(loc)
	start := monthStartFromCtx(ctx)

	rows, err := q.GetAccountsDailyNet(ctx, models.AccountHistoryFrom(start, today))
	if err != nil {
		logger.FromCtx(ctx).Error("querying accounts daily net", zap.Error(err))
		return nil, err
	}
	daily := map[string][]models.DayFlow{}
	for _, row := range rows {
		if d, ok := dayFlow(row.Day, row.MoneyIn, row.MoneyOut); ok {
			daily[row.AccountID] = append(daily[row.AccountID], d)
		}
	}
	return models.NewAccountsSummary(accs, *def, with, start, today, daily), nil
}
