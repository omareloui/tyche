package seeder

import (
	"context"
	"database/sql"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"

	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"github.com/omareloui/tyche/internal/queries"
)

type Seeder struct{}

func NewSeeder() *Seeder {
	return &Seeder{}
}

func (s *Seeder) Seed(ctx context.Context, q *queries.Queries) {
	wg := new(sync.WaitGroup)

	go s.seedCurrencies(ctx, wg, q)
	go s.seedAccountKinds(ctx, wg, q)
	go s.seedTxKinds(ctx, wg, q)

	wg.Add(3)
	wg.Wait()
}

func (s *Seeder) seedCurrencies(ctx context.Context, wg *sync.WaitGroup, q *queries.Queries) {
	defer wg.Done()

	l := logger.FromCtx(ctx)

	currencies, err := q.GetCurrencies(ctx)
	if err != nil {
		l.Error("error while getting the currencies", zap.Error(err))
		return
	}

	currenciesSeed := []queries.Currency{
		{ID: idgen.New(), Enum: int64(models.CurrencyEgp)},
		{ID: idgen.New(), Enum: int64(models.CurrencyUsd)},
		{ID: idgen.New(), Enum: int64(models.CurrencyAed)},
	}

	if len(currencies) != len(currenciesSeed) {
		for _, cur := range currenciesSeed {
			if err := q.CreateCurrency(ctx, queries.CreateCurrencyParams(cur)); err != nil {
				l.Error("error while seeding the currencies", zap.Error(err))
			}
		}
	}
}

func (s *Seeder) seedTxKinds(ctx context.Context, wg *sync.WaitGroup, q *queries.Queries) {
	defer wg.Done()

	l := logger.FromCtx(ctx)

	txKinds, err := q.GetTransactionKinds(ctx)
	if err != nil {
		l.Error("error while getting the transaction kinds", zap.Error(err))
		return
	}

	kindsSeed := []queries.TransactionKind{
		{ID: idgen.New(), Enum: int64(models.ExpenseTxKind), IncreaseBalance: sql.NullBool{Bool: false, Valid: true}},
		{ID: idgen.New(), Enum: int64(models.IncomeTxKind), IncreaseBalance: sql.NullBool{Bool: true, Valid: true}},
		{ID: idgen.New(), Enum: int64(models.TransferTxKind), IncreaseBalance: sql.NullBool{Valid: false}},
		{ID: idgen.New(), Enum: int64(models.DebtTxKind), IncreaseBalance: sql.NullBool{Valid: false}},
	}

	if len(txKinds) != len(kindsSeed) {
		for _, kind := range kindsSeed {
			if err := q.CreateTransactionKind(ctx, queries.CreateTransactionKindParams(kind)); err != nil {
				l.Error("error while seeding the transaction kinds", zap.Error(err))
			}
		}
	}
}

func (s *Seeder) seedAccountKinds(ctx context.Context, wg *sync.WaitGroup, q *queries.Queries) {
	defer wg.Done()

	l := logger.FromCtx(ctx)

	accKinds, err := q.GetAccountKinds(ctx)
	if err != nil {
		l.Error("error while getting the account kinds", zap.Error(err))
		return
	}

	accKindsSeed := []queries.CreateAccountKindParams{
		{ID: idgen.New(), Enum: int64(models.AccKindCreditCard)},
		{ID: idgen.New(), Enum: int64(models.AccKindBankAcc)},
		{ID: idgen.New(), Enum: int64(models.AccKindCash)},
		{ID: idgen.New(), Enum: int64(models.AccKindDebt)},
	}

	if len(accKinds) != len(accKindsSeed) {
		for _, k := range accKindsSeed {
			if err := q.CreateAccountKind(ctx, k); err != nil {
				l.Error("error while seeding the account kinds", zap.Error(err))
			}
		}
	}
}
