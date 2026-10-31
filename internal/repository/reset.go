package repository

import (
	"context"

	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
	"github.com/omareloui/tyche/internal/seeder"
	"go.uber.org/zap"
)

// Reset deletes the selected entities of the tenant in one transaction. A
// full reset seeds the defaults again, as for a new user.
func (r *Repository) Reset(ctx context.Context, opts models.ResetOptions) error {
	l := logger.FromCtx(ctx)
	opts.Normalize()

	err := r.transact(ctx, func(qtx *queries.Queries) error {
		steps := []struct {
			do  bool
			run func(context.Context) error
		}{
			{opts.Has(models.ResetSMS), qtx.ResetDeleteSMSMessages},
			{opts.Has(models.ResetSMSTemplates), qtx.ResetDeleteSMSTemplates},
			{opts.Has(models.ResetWishlist), qtx.ResetDeleteWishlistItems},
			{opts.Has(models.ResetTransactions), qtx.ResetDeleteTransactions},
			// Balances are the sum of the transactions, so kept accounts
			// start again from 0.
			{opts.Has(models.ResetTransactions) && !opts.Has(models.ResetAccounts), qtx.ResetZeroAccountBalances},
			{opts.Has(models.ResetRecurrent), qtx.ResetDeleteRecurrentTxs},
			{opts.Has(models.ResetGoals), qtx.ResetDeleteGoals},
			{opts.Has(models.ResetCategories), qtx.ResetClearTxCategories},
			{opts.Has(models.ResetCategories), qtx.ResetClearRecurrentTxCategories},
			{opts.Has(models.ResetCategories), qtx.ResetDeleteCategories},
			{opts.Has(models.ResetAccounts), qtx.ResetDeleteAccounts},
			{opts.Has(models.ResetCounterparties) && !opts.Has(models.ResetAccounts), qtx.ResetClearAccountCounterparties},
			{opts.Has(models.ResetCounterparties), qtx.ResetDeleteCounterparties},
			{opts.Has(models.ResetDevices), qtx.ResetDeleteDevices},
		}
		for _, s := range steps {
			if !s.do {
				continue
			}
			if err := s.run(ctx); err != nil {
				return err
			}
		}

		if opts.Has(models.ResetSettings) {
			if err := qtx.ResetCurrenciesShown(ctx, seeder.ShownCurrencies()); err != nil {
				return err
			}
			if err := qtx.ResetUserSettings(ctx, models.GetDefaultCurrency().String); err != nil {
				return err
			}
		}

		if opts.IsAll() {
			return qtx.ResetSeededAt(ctx)
		}
		return nil
	})
	if err != nil {
		l.Error("resetting account", zap.Any("options", opts), zap.Error(err))
		return err
	}
	l.Info("reset account", zap.Any("options", opts))

	if !opts.IsAll() {
		return nil
	}

	provider, oauthID, err := tenantFromCtx(ctx)
	if err != nil {
		return err
	}
	if err := r.tm.Reseed(ctx, provider, oauthID); err != nil {
		l.Error("reseeding after reset", zap.Error(err))
		return err
	}
	return nil
}
