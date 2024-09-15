package main

import (
	"context"
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"

	"github.com/joho/godotenv"
	"github.com/omareloui/financial-planner/internal/config"
	"github.com/omareloui/financial-planner/internal/db"
	"github.com/omareloui/financial-planner/internal/dbconn"
	"github.com/omareloui/financial-planner/internal/fxrate"
	"github.com/omareloui/financial-planner/internal/idgen"
	"github.com/omareloui/financial-planner/internal/models"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	queries, err := dbconn.New(config.GetDataSource())
	if err != nil {
		log.Fatalln("error connecting to the database", err)
		return
	}

	currencies, err := queries.GetCurrencies(context.Background())
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
			if err := queries.CreateCurrency(context.Background(), db.CreateCurrencyParams(cur)); err != nil {
				log.Fatalln("error while seeding the currencies", err)
			}
		}
	}

	kinds, err := queries.GetTransactionKinds(context.Background())
	if err != nil {
		log.Fatalln("error while getting the kinds", err)
	}

	kindsSeed := []db.TransactionKind{
		{ID: idgen.New(), Kind: "EXPENSES", IncreaseBalance: sql.NullBool{Bool: false, Valid: true}},
		{ID: idgen.New(), Kind: "INCOME", IncreaseBalance: sql.NullBool{Bool: true, Valid: true}},
	}

	if len(kinds) != len(kindsSeed) {
		for _, kind := range kindsSeed {
			if err := queries.CreateTransactionKind(context.Background(), db.CreateTransactionKindParams(kind)); err != nil {
				log.Fatalln("error while seeding the transaction kinds", err)
			}
		}
	}

	balances, err := queries.GetBalances(context.Background())
	if err != nil {
		log.Fatalln("error while getting the balances", err)
	}

	if len(balances) != len(currenciesSeed) {
		currencies, err := queries.GetCurrencies(context.Background())
		if err != nil {
			log.Fatalln("error while getting the currencies", err)
		}

		for _, cur := range currencies {
			if err := queries.CreateBalance(context.Background(), db.CreateBalanceParams{
				ID:         idgen.New(),
				CurrencyID: cur.ID,
				Amount:     0,
			}); err != nil {
				log.Fatalln("error while seeding the balance", err)
			}
		}
	}
}
