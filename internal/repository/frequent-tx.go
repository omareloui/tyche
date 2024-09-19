package repository

import (
	"database/sql"
	"slices"
	"time"

	"github.com/omareloui/financial-planner/internal/db"
	"github.com/omareloui/financial-planner/internal/logger"
	"github.com/omareloui/financial-planner/internal/models"
	"go.uber.org/zap"
	"golang.org/x/net/context"
)

func (r *Repository) createFrequentTxWithTxn(qtx *db.Queries, ctx context.Context, ftx *models.FrequentTx) error {
	l := logger.FromCtx(ctx)
	if err := qtx.CreateFrequentTx(ctx, db.CreateFrequentTxParams{
		ID:                       ftx.ID,
		FrequencyBy:              int64(ftx.FrequencyBy),
		Frequency:                int64(ftx.Frequency),
		Amount:                   ftx.Amount,
		CurrencyID:               ftx.Currency.ID,
		KindID:                   ftx.Kind.ID,
		Description:              sql.NullString{String: ftx.Description, Valid: ftx.Description != ""},
		StopsOn:                  sql.NullTime{Time: ftx.StopsOn, Valid: !ftx.StopsOn.IsZero()},
		CreatedAt:                ftx.CreatedAt,
		LastTransactionCreatedAt: sql.NullTime{Time: ftx.LastTransactionCreatedAt, Valid: !ftx.LastTransactionCreatedAt.IsZero()},
	}); err != nil {
		l.Error("creating frequent transaction", zap.Error(err), zap.Any("frequent tx", ftx))
		return err
	}
	l.Debug("created frequent transaction", zap.Any("frequent tx", ftx))
	return nil
}

func (r *Repository) createFrequentTxTransactionWithTxn(qtx *db.Queries, ctx context.Context, ftxId, txId string) error {
	l := logger.FromCtx(ctx)
	if err := qtx.CreateFrequentTxTransaction(ctx, db.CreateFrequentTxTransactionParams{
		TransactionID: txId,
		FrequencyID:   ftxId,
	}); err != nil {
		l.Error("creating frequent transaction transaction", zap.Error(err))
		return err
	}
	l.Debug("created frequent transaction transaction")
	return nil
}

func (r *Repository) FrequentTxs(ctx context.Context) ([]models.FrequentTx, error) {
	l := logger.FromCtx(ctx)
	dbFtx, err := r.q.GetFrequentTxs(ctx)
	if err != nil {
		l.Error("querying frequent transactions", zap.Error(err))
		return nil, err
	}
	l.Debug("queried frequent transactions")

	return dbFrequentTxRowsToFrequentTxs(dbFtx), nil
}

func dbFrequentTxRowsToFrequentTxs(dbFtxs []db.GetFrequentTxsRow) []models.FrequentTx {
	ftxs := []models.FrequentTx{}

	for _, dbFtx := range dbFtxs {
		ftxIdx := slices.IndexFunc(ftxs, func(ftx models.FrequentTx) bool { return ftx.ID == dbFtx.ID })
		if ftxIdx == -1 {
			ftxs = append(ftxs, models.FrequentTx{
				ID:          dbFtx.ID,
				Amount:      dbFtx.Amount,
				Description: dbFtx.Description.String,
				CreatedAt:   dbFtx.CreatedAt.(time.Time),
				Currency: models.Currency{
					ID:        dbFtx.CurrencyID,
					Enum:      models.CurrencyEnum(dbFtx.Enum.Int64),
					EgpFxRate: dbFtx.EgpFxRate.Float64,
				},
				Kind: models.TransactionKind{
					ID:              dbFtx.KindID,
					Kind:            dbFtx.Kind.String,
					IncreaseBalance: dbFtx.IncreaseBalance.Bool,
				},
				Frequency:                uint(dbFtx.Frequency),
				FrequencyBy:              models.FrequencyBy(dbFtx.FrequencyBy),
				StopsOn:                  dbFtx.StopsOn.Time,
				LastTransactionCreatedAt: dbFtx.TransactionFulfilledOn.Time,
				Transactions:             []models.Transaction{},
			})
			ftxIdx = len(ftxs) - 1
		}
		ftxs[ftxIdx].Transactions = append(ftxs[ftxIdx].Transactions, models.Transaction{
			ID:          dbFtx.TransactionID.String,
			Amount:      dbFtx.TransactionAmount.Float64,
			Description: dbFtx.TransactionDescription.String,
			FulfilledOn: dbFtx.TransactionFulfilledOn.Time,
			ScheduledTo: dbFtx.TransactionScheduledTo.Time,
			CreatedAt:   dbFtx.TransactionCreatedAt.(time.Time),
			Currency: models.Currency{
				ID:        dbFtx.TransactionCurrencyID.String,
				Enum:      models.CurrencyEnum(dbFtx.TransactionEnum.Int64),
				EgpFxRate: dbFtx.TransactionEgpFxRate.Float64,
			},
			Kind: models.TransactionKind{
				ID:              dbFtx.TransactionKindID.String,
				Kind:            dbFtx.TransactionKind.String,
				IncreaseBalance: dbFtx.TransactionIncreaseBalance.Bool,
			},
		})
	}

	return ftxs
}
