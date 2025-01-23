package repository

import (
	"context"
	"slices"

	"github.com/omareloui/tyche/internal/db"
	"github.com/omareloui/tyche/internal/logger"
	"github.com/omareloui/tyche/internal/models"
	"go.uber.org/zap"
)

func (r *Repository) GetGoals(ctx context.Context) ([]models.Goal, error) {
	l := logger.FromCtx(ctx)
	dbGoals, err := r.q.GetGoals(ctx)
	if err != nil {
		l.Error("fetching goals", zap.Error(err))
		return nil, err
	}
	l.Debug("fetched goals")
	return dbGoalRowsToGoals(dbGoals), nil
}

func (r *Repository) GetGoal(ctx context.Context, id string) (*models.Goal, error) {
	dbGls, err := r.q.GetGoal(ctx, id)
	if err != nil {
		return nil, err
	}
	return dbGoalRowToGoal(dbGls), nil
}

func (r *Repository) CreateGoal(ctx context.Context, gl *models.Goal) error {
	return r.q.CreateGoal(ctx, db.CreateGoalParams{
		ID:         gl.ID,
		Name:       gl.Name,
		Price:      gl.Price,
		CurrencyID: gl.Currency.ID,
	})
}

func (r *Repository) UpdateGoal(ctx context.Context, gl *models.Goal) error {
	return r.q.UpdateGoal(ctx, db.UpdateGoalParams{
		ID:         gl.ID,
		Name:       gl.Name,
		Price:      gl.Price,
		CurrencyID: gl.Currency.ID,
	})
}

func (r *Repository) FulfillGoal(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx)
	if err := r.q.FulfillGoal(ctx, id); err != nil {
		l.Error("fulfilling goal", zap.Error(err))
		return err
	}
	l.Debug("fulfilled goal", zap.String("id", id))
	return nil
}

func (r *Repository) UnfulfillGoal(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx)
	if err := r.q.UnfulfillGoal(ctx, id); err != nil {
		l.Error("fulfilling goal", zap.Error(err))
		return err
	}
	l.Debug("fulfilled goal", zap.String("id", id))
	return nil
}

func (r *Repository) CreateGoalProgress(ctx context.Context, glId string, tx *models.Transaction) error {
	return r.transact(func(qtx *db.Queries) error {
		if err := r.createTransactionAndApplyToAccountIfNeededWithTxn(ctx, qtx, tx); err != nil {
			return err
		}

		l := logger.FromCtx(ctx)
		if err := qtx.CreateGoalProgress(ctx, db.CreateGoalProgressParams{GoalID: glId, TransactionID: tx.ID}); err != nil {
			l.Error("creating goal progress", zap.Error(err))
			return err
		}
		l.Debug("created goal progress")
		return nil
	})
}

func (r *Repository) DeleteGoal(ctx context.Context, id string) error {
	return r.q.DeleteGoal(ctx, id)
}

func dbGoalRowToGoal(dbGl db.GetGoalRow) *models.Goal {
	return &models.Goal{
		ID:          dbGl.ID,
		Name:        dbGl.Name,
		Price:       dbGl.Price,
		Currency:    models.Currency{ID: dbGl.CurrencyID, Enum: models.CurrencyEnum(dbGl.Currency.Int64)},
		FulfilledAt: dbGl.FulfilledAt.Time,
		CreatedAt:   dbGl.CreatedAt,
	}
}

func dbGoalRowsToGoals(dbGls []db.GetGoalsRow) []models.Goal {
	gls := []models.Goal{}

	for _, dbGl := range dbGls {
		glIdx := slices.IndexFunc(gls, func(gl models.Goal) bool { return gl.ID == dbGl.ID })
		if glIdx == -1 {
			gls = append(gls, models.Goal{
				ID:           dbGl.ID,
				Name:         dbGl.Name,
				Price:        dbGl.Price,
				Currency:     models.Currency{ID: dbGl.CurrencyID, Enum: models.CurrencyEnum(dbGl.Currency.Int64)},
				FulfilledAt:  dbGl.FulfilledAt.Time,
				CreatedAt:    dbGl.CreatedAt,
				Transactions: []models.Transaction{},
			})
			glIdx = len(gls) - 1
		}

		if dbGl.TransactionID.Valid {
			if idx := slices.IndexFunc(gls[glIdx].Transactions, func(tx models.Transaction) bool { return tx.ID == dbGl.TransactionID.String }); idx == -1 {
				gls[glIdx].Transactions = append(gls[glIdx].Transactions, models.Transaction{
					ID:     dbGl.TransactionID.String,
					Amount: dbGl.TransactionAmount.Float64,
					Account: models.Account{
						ID:   dbGl.TransactionAccountID.String,
						Name: dbGl.TransactionAccountName.String,
						Currency: models.Currency{
							ID:   dbGl.TransactionAccountCurrencyID.String,
							Enum: models.CurrencyEnum(dbGl.TransactionAccountCurrencyEnum.Int64),
						},
					},
					ScheduledTo: dbGl.TransactionScheduledTo.Time,
					FulfilledAt: dbGl.TransactionFulfilledAt.Time,
				})
			}
		}
	}

	return gls
}
