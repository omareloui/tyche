package repository

import (
	"context"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/slicesutils"
)

func (r *Repository) Seed(ctx context.Context) {
	wg := new(sync.WaitGroup)

	go r.seedCurrencies(ctx, wg)
	go r.seedAccountKinds(ctx, wg)
	go r.seedTxKinds(ctx, wg)

	wg.Add(3)
	wg.Wait()
}

func (r *Repository) seedCurrencies(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	l := logger.FromCtx(ctx)

	exists, err := r.GetCurrencies(ctx)
	if err != nil {
		l.Error("error while getting the currencies", zap.Error(err))
		return
	}

	seed := []models.Currency{
		{ID: idgen.New(), Enum: models.CurrencyEgp},
		{ID: idgen.New(), Enum: models.CurrencyUsd},
		{ID: idgen.New(), Enum: models.CurrencyAed},
	}

	toSeed := slicesutils.FilterNewItems(exists, seed, func(cur models.Currency) models.CurrencyEnum {
		return cur.Enum
	})

	for _, cur := range toSeed {
		if err := r.createCurrency(ctx, cur); err != nil {
			l.Error("error while seeding currency", zap.Any("currency", cur), zap.Error(err))
		}
	}
}

func (r *Repository) seedTxKinds(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	l := logger.FromCtx(ctx)

	exists, err := r.GetTransactionKinds(ctx)
	if err != nil {
		l.Error("error while getting the transaction kinds", zap.Error(err))
		return
	}

	seed := []models.TransactionKind{
		{ID: idgen.New(), Enum: models.ExpenseTxKind, IncreaseBalance: false},
		{ID: idgen.New(), Enum: models.IncomeTxKind, IncreaseBalance: true},
		{ID: idgen.New(), Enum: models.TransferTxKind, IncreaseBalance: false},
		{ID: idgen.New(), Enum: models.DebtTxKind, IncreaseBalance: false},
	}

	toSeed := slicesutils.FilterNewItems(exists, seed, func(tk models.TransactionKind) models.TxKind {
		return tk.Enum
	})

	for _, tk := range toSeed {
		if err := r.createTransactionKind(ctx, tk); err != nil {
			l.Error("error while seeding transaction kind", zap.Any("transaction_kind", tk), zap.Error(err))
		}
	}
}

func (r *Repository) seedAccountKinds(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	l := logger.FromCtx(ctx)

	exists, err := r.GetAccountKinds(ctx)
	if err != nil {
		l.Error("error while getting the account kinds", zap.Error(err))
		return
	}

	seed := []models.AccountKind{
		{ID: idgen.New(), Enum: models.AccKindCreditCard},
		{ID: idgen.New(), Enum: models.AccKindBankAcc},
		{ID: idgen.New(), Enum: models.AccKindCash},
		{ID: idgen.New(), Enum: models.AccKindDebt},
	}

	toSeed := slicesutils.FilterNewItems(exists, seed, func(tk models.AccountKind) models.AccKind {
		return tk.Enum
	})

	for _, k := range toSeed {
		if err := r.createAccountKind(ctx, k); err != nil {
			l.Error("error while seeding the account kinds", zap.Error(err))
		}
	}
}
