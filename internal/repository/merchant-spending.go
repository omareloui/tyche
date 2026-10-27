package repository

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/omareloui/tyche/internal/jwt"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/middleware"
	"github.com/omareloui/tyche/internal/models"
	"go.uber.org/zap"
)

const monthLayout = "2006-01"

// GetMerchantSpending returns the merchant expenses, net of refunds, of month
// (YYYY-MM), or of the latest month with spending when month is empty.
func (r *Repository) GetMerchantSpending(ctx context.Context, month string) (*models.MerchantSpending, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	months, err := q.GetMerchantSpendingMonths(ctx)
	if err != nil {
		l.Error("querying merchant spending months", zap.Error(err))
		return nil, err
	}

	if month == "" {
		month = time.Now().Format(monthLayout)
		if len(months) > 0 {
			month = months[0]
		}
	}
	monthTime, err := time.Parse(monthLayout, month)
	if err != nil {
		return nil, err
	}

	rows, err := q.GetMerchantSpending(ctx, month)
	if err != nil {
		l.Error("querying merchant spending", zap.Error(err))
		return nil, err
	}

	ms := &models.MerchantSpending{Month: monthTime}
	if v := ctx.Value(middleware.AccessClaimsCtxKey{}); v != nil {
		ms.Currency = v.(*jwt.AccessClaims).DefaultCurrency
	}

	// Months are sorted descending, so newer ones come before.
	for _, m := range months {
		if m > month {
			ms.Next = m
		} else if m < month && ms.Prev == "" {
			ms.Prev = m
		}
	}

	idx := map[string]int{}
	for _, row := range rows {
		cur := dbCurrencyToCurrency(&row.Currency, false)
		amount := row.Total
		if ms.Currency.String != "" && cur.String != ms.Currency.String {
			amount *= cur.FxRate(ms.Currency.String)
		}

		if i, ok := idx[row.CounterpartyID]; ok {
			ms.Items[i].Amount += amount
			continue
		}
		idx[row.CounterpartyID] = len(ms.Items)
		ms.Items = append(ms.Items, models.MerchantSpend{
			Counterparty: models.Counterparty{
				ID:         row.CounterpartyID,
				Name:       row.CounterpartyName,
				Icon:       row.CounterpartyIcon.String,
				Color:      row.CounterpartyColor.String,
				IsMerchant: true,
			},
			Amount: amount,
		})
	}

	// Drop merchants whose refunds cover their expenses.
	ms.Items = slices.DeleteFunc(ms.Items, func(it models.MerchantSpend) bool {
		return it.Amount <= 0
	})
	slices.SortStableFunc(ms.Items, func(a, b models.MerchantSpend) int {
		return cmp.Compare(b.Amount, a.Amount)
	})

	return ms, nil
}
