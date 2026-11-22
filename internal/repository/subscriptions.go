package repository

import (
	"context"
	"slices"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// GetSubscriptionCandidates is what looks recurring as of today
// (models.DetectSubscriptions) but isn't a recurrent transaction yet: the
// ones a live recurrent transaction of the same counterparty and currency
// already makes (about the same amount), and the dismissed ones (until their
// amount changes), are left out.
func (r *Repository) GetSubscriptionCandidates(ctx context.Context, today time.Time) ([]models.SubscriptionCandidate, error) {
	l := logger.FromCtx(ctx)

	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	from := time.Date(today.Year(), today.Month()-models.SubscriptionLookback, today.Day(), 0, 0, 0, 0, time.UTC)
	rows, err := q.GetSubscriptionTxs(ctx, from)
	if err != nil {
		l.Error("querying subscription transactions", zap.Error(err))
		return nil, err
	}
	txs := make([]models.SubscriptionTx, 0, len(rows))
	for _, row := range rows {
		d, err := time.Parse(dayLayout, row.Day)
		if err != nil {
			continue
		}
		txs = append(txs, models.SubscriptionTx{
			ID: row.ID, AccountID: row.AccountID, CurrencyID: row.CurrencyID,
			CounterpartyID: row.CounterpartyID, CategoryID: row.CategoryID,
			Day: d, Amount: row.Amount,
		})
	}
	cands := models.DetectSubscriptions(txs, today)
	if len(cands) == 0 {
		return nil, nil
	}

	dismissed, err := q.GetDismissedSubscriptions(ctx)
	if err != nil {
		l.Error("querying dismissed subscriptions", zap.Error(err))
		return nil, err
	}
	rtxs, err := r.GetRecurrentTxs(ctx)
	if err != nil {
		return nil, err
	}

	cands = slices.DeleteFunc(cands, func(c models.SubscriptionCandidate) bool {
		for _, d := range dismissed {
			if d.CounterpartyID == c.CounterpartyID && d.CurrencyID == c.CurrencyID && c.SameAs(d.Amount) {
				return true
			}
		}
		for _, rt := range rtxs {
			if rt.Counterparty == nil || rt.Counterparty.ID != c.CounterpartyID ||
				rt.Account.Currency.ID != c.CurrencyID || rt.HasEnded(today) {
				continue
			}
			if c.SameAs(rt.Amount) {
				return true
			}
		}
		return false
	})
	l.Debug("detected subscriptions", zap.Int("count", len(cands)))
	return cands, nil
}

// DismissSubscription hides a counterparty's suggested subscription in a
// currency while its amount stays about the same.
func (r *Repository) DismissSubscription(ctx context.Context, counterpartyID, currencyID string, amount float64) error {
	_, q, err := r.dbFromCtx(ctx)
	if err != nil {
		return err
	}
	if err := q.DismissSubscription(ctx, queries.DismissSubscriptionParams{
		CounterpartyID: counterpartyID, CurrencyID: currencyID, Amount: amount,
	}); err != nil {
		logger.FromCtx(ctx).Error("dismissing a subscription", zap.Error(err))
		return err
	}
	return nil
}
