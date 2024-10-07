package main

import (
	"context"
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"

	"github.com/joho/godotenv"
	"github.com/omareloui/tyche/internal/config"
	"github.com/omareloui/tyche/internal/db"
	"github.com/omareloui/tyche/internal/dbconn"
	"github.com/omareloui/tyche/internal/fxrate"
	"github.com/omareloui/tyche/internal/idgen"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
)

func main() {
	l := logger.Get()

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	_, q, err := dbconn.New(config.GetDataSource())
	if err != nil {
		log.Fatalln("error connecting to the database", err)
		return
	}

	currencies, err := q.GetCurrencies(context.Background())
	if err != nil {
		log.Fatalln("error while getting the currencies", err)
	}

	usdToEgp, err := fxrate.UsdToEgp()
	if err != nil {
		log.Fatalln("error while fetching usd fx rate", err)
	}
	currenciesSeed := []db.Currency{
		{ID: idgen.New(), Enum: int64(models.CurrencyEgp), EgpFxRate: 1},
		{ID: idgen.New(), Enum: int64(models.CurrencyUsd), EgpFxRate: usdToEgp},
	}

	if len(currencies) != len(currenciesSeed) {
		for _, cur := range currenciesSeed {
			if err := q.CreateCurrency(context.Background(), db.CreateCurrencyParams(cur)); err != nil {
				log.Fatalln("error while seeding the currencies", err)
			}
		}
	}

	txKinds, err := q.GetTransactionKinds(context.Background())
	if err != nil {
		l.Error("error while getting the transaction kinds", zap.Error(err))
	}

	kindsSeed := []db.TransactionKind{
		{ID: idgen.New(), Kind: models.ExpenseTxKind, IncreaseBalance: sql.NullBool{Bool: false, Valid: true}},
		{ID: idgen.New(), Kind: models.IncomeTxKind, IncreaseBalance: sql.NullBool{Bool: true, Valid: true}},
		{ID: idgen.New(), Kind: models.TransferTxKind, IncreaseBalance: sql.NullBool{Valid: false}},
		{ID: idgen.New(), Kind: models.DebtTxKind, IncreaseBalance: sql.NullBool{Valid: false}},
	}

	if len(txKinds) != len(kindsSeed) {
		for _, kind := range kindsSeed {
			if err := q.CreateTransactionKind(context.Background(), db.CreateTransactionKindParams(kind)); err != nil {
				l.Error("error while seeding the transaction kinds", zap.Error(err))
			}
		}
	}

	accKinds, err := q.GetAccountKinds(context.Background())
	if err != nil {
		l.Error("error while getting the account kinds", zap.Error(err))
	}

	accKindsSeed := []db.CreateAccountKindParams{
		{ID: idgen.New(), Enum: int64(models.AccountKindCreditCard)},
		{ID: idgen.New(), Enum: int64(models.AccountKindBankAccount)},
		{ID: idgen.New(), Enum: int64(models.AccountKindCash)},
		{ID: idgen.New(), Enum: int64(models.AccountKindDebt)},
	}

	if len(accKinds) != len(accKindsSeed) {
		for _, k := range accKindsSeed {
			if err := q.CreateAccountKind(context.Background(), k); err != nil {
				l.Error("error while seeding the account kinds", zap.Error(err))
			}
		}
	}
}
