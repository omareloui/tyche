package repository

import (
	"context"
	"time"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/queries"
	"go.uber.org/zap"
)

// SetRegion sets the user's default currency (shown in pickers) and time zone,
// from the onboarding. The accounts in the previous default currency nothing
// uses yet (e.g. the seeded Cash) move to the new one.
func (r *Repository) SetRegion(ctx context.Context, currID string, loc *time.Location) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		prev, err := qtx.GetDefaultCurrency(ctx)
		if err != nil {
			return err
		}
		if err := qtx.SetCurrencyShown(ctx, queries.SetCurrencyShownParams{ID: currID, IsShown: true}); err != nil {
			return err
		}
		if err := qtx.SetDefaultCurrency(ctx, currID); err != nil {
			return err
		}
		if prev.Currency.ID != currID {
			if err := qtx.SetCurrencyOfUnusedAccounts(ctx, queries.SetCurrencyOfUnusedAccountsParams{
				CurrencyID:     currID,
				FromCurrencyID: prev.Currency.ID,
			}); err != nil {
				return err
			}
		}
		if err := qtx.SetTimezone(ctx, loc.String()); err != nil {
			return err
		}
		logger.FromCtx(ctx).Info("set the region", zap.String("currency_id", currID), zap.String("timezone", loc.String()))
		return nil
	})
}

// SetCurrencyAndTimezone sets the user's default currency (shown in pickers)
// and time zone, from the settings. Unlike SetRegion, no account changes
// currency.
func (r *Repository) SetCurrencyAndTimezone(ctx context.Context, currID string, loc *time.Location) error {
	return r.transact(ctx, func(qtx *queries.Queries) error {
		if err := qtx.SetCurrencyShown(ctx, queries.SetCurrencyShownParams{ID: currID, IsShown: true}); err != nil {
			return err
		}
		if err := qtx.SetDefaultCurrency(ctx, currID); err != nil {
			return err
		}
		if err := qtx.SetTimezone(ctx, loc.String()); err != nil {
			return err
		}
		logger.FromCtx(ctx).Info("set the currency and time zone", zap.String("currency_id", currID), zap.String("timezone", loc.String()))
		return nil
	})
}
